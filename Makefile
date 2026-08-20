# ┊┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃
#  Model-agnostic agent framework Makefile
# ──────────────────────────────────────────────────────────────
#  Targets are thin dispatchers. All real logic lives in
#  xops/makefile/<module>.py (stdlib-only, cross-platform).
#
#  Convention:
#    • daily verbs are short  : help, git, doctor, scaffold
#    • everything else uses   : domain.action  (track.add, git.dry, roadmap.status)
# ┊┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃

PYTHON ?= python3
XOPS   := $(PYTHON) xops/makefile

# Tracking append defaults (override on CLI: make track.add ACTION=note SUMMARY="...")
ACTION  ?= note
STATUS  ?= completed
SCOPE   ?= general
AGENT   ?= human
SUMMARY ?=
REFS    ?=
RUN_ID  ?=

# Skills targets
TAG ?=

.DEFAULT_GOAL := help

.PHONY: help git git.dry track.add track.list roadmap.status doctor scaffold skills.status skills.find test verify \
  server.build server.test server.lint client.build client.test client.lint \
  compose.up compose.down compose.snap.create compose.snap.restore

## help              List all available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  make /' | sort

## git               Commit pending tracking rows as conventional commits + push
git:
	@$(XOPS)/git_ops.py push

## git.dry           Preview what `make git` would commit and push (read-only)
git.dry:
	@$(XOPS)/git_ops.py dry

## track.add         Append a row to docs/tracking/tracking.csv (vars: ACTION STATUS SCOPE AGENT SUMMARY REFS RUN_ID)
track.add:
	@$(XOPS)/track_ops.py add \
		--action="$(ACTION)" --status="$(STATUS)" --scope="$(SCOPE)" \
		--agent="$(AGENT)"   --summary="$(SUMMARY)" --refs="$(REFS)" \
		$(if $(RUN_ID),--run-id="$(RUN_ID)",)

## track.list        Show recent tracking rows (last 20)
track.list:
	@$(XOPS)/track_ops.py list

## roadmap.status    Summarize ROADMAP.md checkbox progress
roadmap.status:
	@$(XOPS)/roadmap_ops.py status

## doctor            Sanity-check the framework is wired correctly
doctor:
	@$(XOPS)/doctor.py

## scaffold          Print bootstrapper usage (run xops/init/scaffold.sh --help for real)
scaffold:
	@xops/init/scaffold.sh --help

## skills.status     List all skills with line count, last-modified, and AGENTS.md refs
skills.status:
	@$(XOPS)/skills_ops.py status

## skills.find       Search skills by tag or name keyword (TAG=<tag>)
skills.find:
	@TAG="$(TAG)" $(XOPS)/skills_ops.py find

## test              Run server and client unit tests
test:
	@$(MAKE) --no-print-directory server.test
	@$(MAKE) --no-print-directory client.test

## verify            Verifier gate: full test suite + make doctor (run cold)
verify:
	@$(MAKE) --no-print-directory test
	@$(MAKE) --no-print-directory doctor

## server.build      Build the Go server binary
server.build:
	@cd server && go build ./cmd/knowoffd

## server.test       Run all Go unit tests
server.test:
	@cd server && go test ./...

## server.lint       Lint and format-check Go code (golangci-lint, fallback gofmt + go vet)
server.lint:
	@cd server && if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		test -z "$$(gofmt -l .)" || (echo "gofmt issues:"; gofmt -l .; exit 1); \
		go vet ./...; \
	fi

## client.build      Build Flutter for Android and Web (CI also builds iOS)
client.build:
	@cd client && flutter build apk --debug
	@cd client && flutter build web

## client.test       Run Flutter unit/widget tests
client.test:
	@cd client && flutter test

## client.lint       Run Flutter static analysis and format check
client.lint:
	@cd client && flutter analyze
	@cd client && dart format --output=none --set-exit-if-changed .

## compose.up        Start the local Docker Compose stack
compose.up:
	@cd deploy/compose && docker compose --profile core up --build -d

## compose.down      Stop the local Docker Compose stack
compose.down:
	@cd deploy/compose && docker compose --profile core down

## compose.snap.create <dir>  Snapshot running Compose volumes
compose.snap.create:
	@cd deploy/compose && ./snapshot.sh create "$(dir)"

## compose.snap.restore <dir> Restore snapshot onto fresh Compose volumes
compose.snap.restore:
	@cd deploy/compose && ./snapshot.sh restore "$(dir)"
