# ┊┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃┃
#  Model-agnostic agent framework Makefile
# ──────────────────────────────────────────────────────────────
#  Targets are thin dispatchers. All real logic lives in
#  xops/makefile/<module>.py (stdlib-only, cross-platform).
#
#  Convention:
#    • daily verbs are short  : help, git
#    • everything else uses   : domain.action  (track.add, git.dry)
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

.DEFAULT_GOAL := help

.PHONY: help git git.dry track.add track.list codeg \
	server.build server.rebuild web.rebuild web.run web.stop \
	up down bots \
	label.version label.list localhostfile.add localhostfile.remove localhostfile.status

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

## codeg             Initialize or update the CodeGraph index
codeg:
	@$(XOPS)/codegraph_ops.py update

## server.rebuild    Stop the current server, then rebuild and recreate it
server.rebuild:
	@$(XOPS)/dev_ops.py rebuild

## web.rebuild       Force-rebuild + recreate the client-web (Flutter web) dev container, then open the dev URL in a fresh private browser window
web.rebuild:
	@$(XOPS)/client_web_ops.py rebuild

## web.run           Run the Flutter web client natively via the Flutter CLI (no Docker) at http://localhost:8000 or http://0.0.0.0:8000 for integrated browser
web.run:
	@$(XOPS)/flutter_web_ops.py run

## web.stop          Stop every locally owned Flutter web-server process, regardless of port
web.stop:
	@$(XOPS)/flutter_web_ops.py stop

## up                Start the manual debug stack with private five-mode prototype enabled
up:
	@$(XOPS)/dev_ops.py up

## down              Stop the local Docker Compose stack
down:
	@$(XOPS)/dev_ops.py down

export ROOM COUNT
COUNT ?= 3
## bots              Join your private room with explicit bot companions (ROOM=code COUNT=3 or 5)
bots:
	@$(XOPS)/dev_ops.py bots

## label.version     Bump a Dockerfile's org.opencontainers.image.version (vars: SERVICE VERSION)
label.version:
	@$(XOPS)/labels_ops.py version

## label.list        List every service Dockerfile's current image label version
label.list:
	@$(XOPS)/labels_ops.py list

## localhostfile.add Resolve *.knowoff.local to 127.0.0.1 in the OS hosts file (needs elevated privileges)
localhostfile.add:
	@$(XOPS)/hosts_ops.py add

## localhostfile.remove Remove the *.knowoff.local block from the OS hosts file (needs elevated privileges)
localhostfile.remove:
	@$(XOPS)/hosts_ops.py remove

## localhostfile.status Show whether the *.knowoff.local hosts block is present
localhostfile.status:
	@$(XOPS)/hosts_ops.py status

.PHONY: playtest.up playtest.down playtest.reset playtest.web playtest.android
## playtest.up       Build Web and start the private five-mode prototype with six player origins
playtest.up:
	@$(XOPS)/playtest_ops.py up

## playtest.down     Stop the private prototype, preserving its disposable accounts and history
playtest.down:
	@$(XOPS)/playtest_ops.py down

## playtest.reset    Stop and delete only the private prototype database (then clear player site data)
playtest.reset:
	@$(XOPS)/playtest_ops.py reset

## playtest.web      Rebuild the ordinary Web client served by the private prototype
playtest.web:
	@$(XOPS)/playtest_ops.py web

## playtest.android  Build a debug APK for Android emulator playtesting
playtest.android:
	@$(XOPS)/playtest_ops.py android
