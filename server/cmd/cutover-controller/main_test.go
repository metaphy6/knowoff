package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCutoverCommandHelpDoesNotReadInputOrCredentials(t *testing.T) {
	var out, diagnostics bytes.Buffer
	if status := run([]string{"--help"}, func(string) string { t.Fatal("credentials read for help"); return "" }, nil, &out, &diagnostics); status != 0 || !strings.Contains(diagnostics.String(), "usage:") || out.Len() != 0 {
		t.Fatal("help must succeed without database access")
	}
}

func TestCutoverCommandRefusesAmbiguousInputWithoutEcho(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		body string
	}{
		{"operation required", nil, `{}`},
		{"unknown argument", []string{"--private-password=secret"}, `{}`},
		{"positional", []string{"--operation=identity", "private-password"}, `{}`},
		{"zero timeout", []string{"--operation=identity", "--timeout=0s"}, `{}`},
		{"unbounded timeout", []string{"--operation=identity", "--timeout=6m"}, `{}`},
		{"unknown operation", []string{"--operation=private-password"}, `{}`},
		{"unknown field", []string{"--operation=identity"}, `{"private-password":"secret"}`},
		{"duplicate field", []string{"--operation=identity"}, `{"roles":{},"roles":{}}`},
		{"case folded duplicate", []string{"--operation=identity"}, `{"roles":{},"Roles":{}}`},
		{"nested duplicate", []string{"--operation=identity"}, `{"roles":{"Control":"one","Control":"two"}}`},
		{"unicode folded duplicate", []string{"--operation=handoff"}, `{"roles":{},"handoff":{"watermark_id":"one","watermar\u212a_id":"two"}}`},
		{"extra document", []string{"--operation=identity"}, `{} {}`},
		{"oversize", []string{"--operation=identity"}, strings.Repeat(" ", 65537)},
		{"deep nesting", []string{"--operation=identity"}, strings.Repeat("[", 40) + "0" + strings.Repeat("]", 40)},
		{"wrong operation body", []string{"--operation=identity"}, `{"roles":{},"begin":{}}`},
		{"missing operation body", []string{"--operation=begin"}, `{"roles":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			status := run(tc.args, func(string) string { t.Fatal("invalid input reached credential lookup"); return "" }, strings.NewReader(tc.body), &out, &diagnostics)
			if status != 2 || out.Len() != 0 || diagnostics.String() != "cutover.command_refused\n" {
				t.Fatal("invalid command produced success or partial receipt")
			}
			for _, value := range []string{"private-user", "private-password", "private-database", "secret", tc.body} {
				if value != "" && strings.Contains(diagnostics.String(), value) {
					t.Fatal("private input escaped diagnostics")
				}
			}
		})
	}
}

func TestCutoverCommandMissingCredentialsAndNetworkErrorArePrivate(t *testing.T) {
	for _, dsn := range []string{"", "postgres://private-user:private-password@127.0.0.1:1/private-database?sslmode=disable"} {
		var out, diagnostics bytes.Buffer
		body := `{"roles":{"Roles":{"Database":"private_database","Owner":"owner","Runtime":"runtime","Capture":"capture","Migrator":"migrator","PrivacyOwner":"privacy_owner","PrivacyExecutor":"privacy_executor"},"Control":"control","WritersEnabled":true}}`
		if status := run([]string{"--operation=identity", "--timeout=1ms"}, func(string) string { return dsn }, strings.NewReader(body), &out, &diagnostics); status == 0 || out.Len() != 0 {
			t.Fatal("missing or unavailable connection must refuse")
		}
		for _, secret := range []string{"private-user", "private-password", "private-database", "private_database"} {
			if strings.Contains(diagnostics.String(), secret) {
				t.Fatal("database diagnostic escaped")
			}
		}
	}
}

func TestCutoverCommandTimeoutIncludesIncompleteInput(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	defer reader.Close()
	finished := make(chan int, 1)
	go func() {
		finished <- run([]string{"--operation=identity", "--timeout=10ms"}, func(string) string { return "" }, reader, io.Discard, io.Discard)
	}()
	select {
	case status := <-finished:
		if status == 0 {
			t.Fatal("incomplete input returned a successful receipt")
		}
	case <-time.After(time.Second):
		reader.Close()
		<-finished
		t.Fatal("input read ignored the operation deadline")
	}
}
