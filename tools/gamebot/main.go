// Command gamebot runs zero-effect text simulations and authenticated local
// prototype network proofs. It cannot join ordinary rooms or use protocol v1.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func main() { os.Exit(runGamebot(os.Args[1:])) }

func runGamebot(args []string) int {
	flags := flag.NewFlagSet("gamebot", flag.ContinueOnError)
	textNetwork := flags.String("text-network", "", "authenticated local prototype network proof (stable mode ID)")
	textOut := flags.String("text-out", "", "new private 0600 simulation or network trace file")
	textReplay := flags.String("text-replay", "", "replay a recorded zero-effect text simulation JSON file")
	textMode := flags.String("text-simulate", "", "zero-effect text simulation (stable mode ID)")
	textPack := flags.String("text-pack", "", "private text bundle directory for simulation or replay")
	textTuning := flags.String("text-tuning", "configs/gameplay/tuning.yaml", "pinned gameplay tuning YAML")
	textSize := flags.Int("text-size", 4, "original table size (4 or 6)")
	server := flags.String("server", "ws://127.0.0.1:8080/ws/v2", "local prototype WebSocket endpoint")
	seed := flags.Int64("seed", time.Now().UnixNano(), "simulation or network random seed")
	// Retain names only to provide an explicit refusal, including default values.
	flags.String("room", "", "retired; ordinary room joining is unsupported")
	flags.String("queue", "", "retired; ordinary queue joining is unsupported")
	flags.String("count", "", "retired; use the explicit text table size")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	visited := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	refuse := func(message string) int { logger.Error(message); return 2 }
	for _, name := range []string{"room", "queue", "count"} {
		if visited[name] {
			return refuse("retired gamebot flag: -" + name)
		}
	}
	if flags.NArg() != 0 {
		return refuse("positional arguments are unsupported")
	}
	commands := 0
	for _, name := range []string{"text-network", "text-simulate", "text-replay"} {
		if visited[name] {
			commands++
		}
	}
	if commands != 1 || (*textNetwork == "" && *textMode == "" && *textReplay == "") {
		return refuse("choose exactly one of -text-network, -text-simulate or -text-replay")
	}
	if *textNetwork != "" {
		if visited["text-pack"] || visited["text-tuning"] {
			return refuse("-text-pack and -text-tuning are incompatible with network execution")
		}
	} else if visited["server"] {
		return refuse("-server is only valid with -text-network")
	}
	if *textReplay != "" {
		for _, name := range []string{"text-out", "text-size", "seed"} {
			if visited[name] {
				return refuse("-" + name + " is incompatible with replay")
			}
		}
	} else {
		mode := gamecontract.ModeID(*textMode)
		if *textNetwork != "" {
			mode = gamecontract.ModeID(*textNetwork)
		}
		if !mode.Valid() || (*textSize != 4 && *textSize != 6) {
			return refuse("a valid text mode and -text-size 4/6 are required")
		}
		if *textOut == "" {
			return refuse("-text-out is required for privileged replay storage")
		}
	}
	if *textNetwork == "" && *textPack == "" {
		return refuse("-text-pack is required for simulation and replay")
	}
	if *textNetwork != "" {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		ctx, timeout := context.WithTimeout(ctx, 20*time.Minute)
		defer timeout()
		if err := runTextNetwork(ctx, *server, gamecontract.ModeID(*textNetwork), *textSize, *seed, *textOut, os.Getenv("KNOWOFF_DEV_BOT_KEY")); err != nil {
			logger.Error("prototype network proof failed", "error", err)
			return 1
		}
		logger.Info("prototype network proof complete", "mode", *textNetwork, "size", *textSize)
		return 0
	}
	if *textReplay != "" {
		script, err := readTextScript(*textReplay)
		if err == nil {
			_, err = replayText(*textPack, *textTuning, script)
		}
		if err != nil {
			logger.Error("text replay failed", "error", err)
			return 1
		}
		logger.Info("text replay verified", "evidence_sha256", script.EvidenceSHA256)
		return 0
	}
	result, err := simulateText(*textPack, *textTuning, gamecontract.ModeID(*textMode), *textSize, *seed)
	if err != nil {
		logger.Error("text simulation failed", "error", err)
		return 1
	}
	if err := writeTextScript(*textOut, result); err != nil {
		logger.Error("text simulation output failed", "error", err)
		return 1
	}
	logger.Info("text simulation complete", "outcome", result.Outcome, "rounds", result.Rounds, "actions", len(result.Steps), "evidence_sha256", result.EvidenceSHA256)
	return 0
}
