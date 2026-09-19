// cutover-controller is an offline operator command. It never loads runtime
// credentials or exposes a listener; database selection is explicit environment.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
)

type commandInput struct {
	Roles       *store.CutoverControlRoleSpec `json:"roles"`
	SourceRoles *store.CutoverControlRoleSpec `json:"source_roles"`
	Begin       *store.CutoverBegin           `json:"begin"`
	Seal        *store.CutoverSeal            `json:"seal"`
	Handoff     *store.CutoverHandoff         `json:"handoff"`
	Bootstrap   *store.CutoverBootstrap       `json:"bootstrap"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, input io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("cutover-controller", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	operation := flags.String("operation", "", "identity, begin, seal, handoff or bootstrap")
	timeout := flags.Duration("timeout", 30*time.Second, "bounded operation duration, at most 5m")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stderr, "usage: cutover-controller --operation=identity|begin|seal|handoff|bootstrap [--timeout=30s]")
			fmt.Fprintln(stderr, "Read a bounded JSON request from stdin; select credentials using KNOWOFF_CUTOVER_CONTROL_DSN and, for bootstrap, KNOWOFF_CUTOVER_SOURCE_CONTROL_DSN.")
			return 0
		}
		fmt.Fprintln(stderr, "cutover.command_refused")
		return 2
	}
	if flags.NArg() != 0 || *timeout <= 0 || *timeout > 5*time.Minute || input == nil {
		fmt.Fprintln(stderr, "cutover.command_refused")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	request, err := readRequestWithin(ctx, input, *operation)
	if err != nil {
		fmt.Fprintln(stderr, "cutover.command_refused")
		return 2
	}
	dsn := getenv("KNOWOFF_CUTOVER_CONTROL_DSN")
	sourceDSN := ""
	if *operation == "bootstrap" {
		sourceDSN = getenv("KNOWOFF_CUTOVER_SOURCE_CONTROL_DSN")
	}
	if dsn == "" || (*operation == "bootstrap" && sourceDSN == "") {
		fmt.Fprintln(stderr, "cutover.credentials_required")
		return 2
	}
	result, err := execute(ctx, *operation, request, dsn, sourceDSN)
	if err != nil {
		// SQL errors, URLs, names and request values are never echoed. A failed
		// command does not imply rollback of an already committed physical fence.
		fmt.Fprintln(stderr, "cutover.operation_refused")
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(result); err != nil {
		fmt.Fprintln(stderr, "cutover.receipt_write_failed")
		return 1
	}
	return 0
}

func readRequestWithin(ctx context.Context, input io.Reader, operation string) (commandInput, error) {
	type decoded struct {
		request commandInput
		err     error
	}
	completed := make(chan decoded, 1)
	go func() {
		request, err := readRequest(input, operation)
		completed <- decoded{request, err}
	}()
	select {
	case result := <-completed:
		return result.request, result.err
	case <-ctx.Done():
		// The executable owns os.Stdin. Closing it unblocks a pipe read, so an
		// incomplete JSON producer cannot outlive the operation's finite deadline.
		if closer, ok := input.(io.Closer); ok {
			closer.Close()
		}
		return commandInput{}, ctx.Err()
	}
}

func readRequest(input io.Reader, operation string) (commandInput, error) {
	var request commandInput
	raw, err := io.ReadAll(io.LimitReader(input, 65537))
	if err != nil || len(raw) > 65536 {
		return request, errors.New("invalid request")
	}
	// DisallowUnknownFields alone accepts duplicate object keys. Check every
	// object before typed decoding so request/lease authority cannot be ambiguous.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := uniqueJSON(decoder, 0); err != nil {
		return request, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return request, errors.New("extra input")
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || request.Roles == nil {
		return request, errors.New("invalid request")
	}
	count := 0
	for _, present := range []bool{request.Begin != nil, request.Seal != nil, request.Handoff != nil, request.Bootstrap != nil} {
		if present {
			count++
		}
	}
	valid := false
	switch operation {
	case "identity":
		valid = count == 0
	case "begin":
		valid = count == 1 && request.Begin != nil
	case "seal":
		valid = count == 1 && request.Seal != nil
	case "handoff":
		valid = count == 1 && request.Handoff != nil
	case "bootstrap":
		valid = count == 1 && request.Bootstrap != nil && request.SourceRoles != nil
	}
	if !valid || (operation != "bootstrap" && request.SourceRoles != nil) {
		return request, errors.New("wrong operation payload")
	}
	return request, nil
}

func uniqueJSON(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("nested input")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	if delim != '{' && delim != '[' {
		return errors.New("invalid delimiter")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			// Every typed field is ASCII. Reject Unicode aliases (for example the
			// Kelvin sign folds to K in encoding/json but not strings.ToUpper).
			for _, r := range name {
				if r > 127 {
					return errors.New("invalid field name")
				}
			}
			name = strings.ToUpper(name)
			if !ok || seen[name] {
				return errors.New("duplicate key")
			}
			seen[name] = true
		}
		if err := uniqueJSON(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func execute(ctx context.Context, operation string, request commandInput, dsn, sourceDSN string) (result any, err error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	controller, err := store.OpenCutoverController(ctx, db, *request.Roles)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, controller.Close()) }()
	switch operation {
	case "identity":
		return controller.Identity(ctx)
	case "begin":
		return controller.Begin(ctx, *request.Begin)
	case "seal":
		return controller.Seal(ctx, *request.Seal)
	case "handoff":
		return controller.Handoff(ctx, *request.Handoff)
	case "bootstrap":
		var sourceDB *sql.DB
		sourceDB, err = sql.Open("postgres", sourceDSN)
		if err != nil {
			return nil, err
		}
		defer func() { err = errors.Join(err, sourceDB.Close()) }()
		var source *store.CutoverController
		source, err = store.OpenCutoverController(ctx, sourceDB, *request.SourceRoles)
		if err != nil {
			return nil, err
		}
		defer func() { err = errors.Join(err, source.Close()) }()
		return controller.Bootstrap(ctx, source, *request.Bootstrap)
	default:
		return nil, errors.New("unsupported operation")
	}
}
