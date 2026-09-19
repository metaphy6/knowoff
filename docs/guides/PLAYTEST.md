# Private prototype playtesting

This is the Phase 4 test setup using the existing real server's private
prototype path. All five modes support four or six players; normal Flutter
clients join through their ordinary anonymous accounts. Matches have no earned
Noin, XP, progression or leaderboard credit. Production availability remains
closed, and this does not complete Phase 6 cutover.

## Manual debugging with bot companions

Use your usual two terminals:

```bash
make up
make web.run
```

Open **http://localhost:8000**, choose **Host a Room**, check available modes,
select any of the five modes and four or six seats, then create the room.
In a third terminal, substitute the displayed room code:

```bash
make bots ROOM=ABC123 COUNT=3   # you plus three bots: four seats
# or: make bots ROOM=ABC123 COUNT=5   # you plus five bots: six seats
```

Bots occupy their own numbered seats, automatically Ready after lobby changes,
and take legal actions and votes from their own role-scoped state. Ready your
own seat and start the match as host. Use the normal results/rematch controls;
bots follow the lobby back to Ready while you choose when to start again.
They never replace your disconnected seat or grant earned Noin/progression.
The private debug header also exposes Freeze, next-match role and specialty
grants; see [developer controls](CLIENT_DEV_TOOLS.md).
The current v2 lobby shows seat numbers, not a Bot badge: you explicitly add
these companions and know which seats they occupy.
Stop the bot command with Ctrl+C when finished; lobby/results seats are released.
Stopping bots during an active match leaves disconnected seats, so create a
fresh room before restarting companions. A backend/Air restart loses the
in-memory match: restart the bot command and create a fresh room afterward.
These companions are for
manual functional debugging, not human humor/balance evidence.

Prefer hot reload (`r`) while playing. Hot restart (`R`) resets client state;
if Flutter reports a browser-engine assertion, reload the browser and rejoin
using the same origin and room code. A disconnected lobby host can transfer
ownership to a companion. After rejoining, stop the bot command, click Refresh
to recover host controls, and add companions again. This recovery was checked
in the six-seat lobby; it is not proof of seamless active-match reconnect.

`make web.run` is an interactive Flutter debug process using `.tools/flutter`
when present, otherwise the SDK on PATH. Renderer resources are served locally
so a blocked CDN cannot leave the debug page blank. Its ordinary terminal controls remain
available. Server source is mounted into the Air development container;
`make server.rebuild` retains the private prototype overlay. `make down`
stops this stack without deleting its database. The generated bot key lives
in ignored, mode-0600 `docs/tracking/state/dev.env`; the launcher passes it only
through the process environment, never a command-line argument. Do not share it.
Base Compose and production configuration do not activate synthetic content.

The manual `knowoff` stack and isolated `knowoff-playtest` stack both use API
port 8080, so run one at a time. To switch to agent verification, stop bots,
run `make down`, then `make playtest.up`. To return, run `make playtest.down`,
then `make up`. Their databases and credentials remain separate; clear browser
site data if you reused an origin with accounts from a different database.

## Agent playtest startup

Prerequisites: Docker with Compose v2, Python 3 and a compatible Flutter SDK
(the workspace `.tools/flutter` is preferred; otherwise `flutter` on PATH).
Android builds also require the project's configured Android SDK/JDK. The
launcher installs no operating-system packages and makes no hosts-file changes.
Run from the repository root:

```bash
make playtest.up
```

This builds the ordinary release Web client with local Web renderer resources,
builds the real server, migrates a separate disposable PostgreSQL database, and
waits for server readiness. First startup needs network access for Docker images,
Go dependencies and Flutter dependencies. It creates private random credentials
in the ignored `docs/tracking/state/playtest.env`; keep that file while retaining
the database. It never seeds an admin account or enables public ingress.

The dedicated Compose project is `knowoff-playtest`; it does not reuse the
normal `knowoff` stack's data. Port 8080 and ports 8001–8006 must be free.
Stop another local stack using its own documented stop command if necessary;
the playtest launcher does not stop unrelated services. Only the API and player
Web ports are published, all bound to host loopback. PostgreSQL, Redis, admin
and metrics listeners are not published.

Open a different origin for each player:

| Player | URL |
|---|---|
| 1 | http://localhost:8001 |
| 2 | http://localhost:8002 |
| 3 | http://localhost:8003 |
| 4 | http://localhost:8004 |
| 5 | http://localhost:8005 |
| 6 | http://localhost:8006 |

Ports separate browser local storage, so six origins can hold six distinct
anonymous accounts. Two tabs at the **same** origin share account credentials
and do not represent separate players. Keep the same origin for reconnect.
Use ports 8001–8004 for a four-player table, adding 8005–8006 for six players.

Switching browser tabs or focusing another window conceals the old client's
private state. On return, a healthy authenticated socket now requests a fresh
role-scoped snapshot without disconnecting the player. Changed credentials or
a replaced socket still require a new handshake. The first actual browser test
exposed focus-driven disconnect/auto-pass; the repaired lifecycle has regression
coverage for hidden frames, same-seat restoration, deadlines, token changes and
socket replacement. Record observed UI acceptance separately from those tests.

One operator cycling through six origins still has to meet the ordinary turn
and trade deadlines. Use independently operated player clients for a realistic
full-match playtest; separate windows are not equivalent to six human players.
The profile is also useful for account isolation and launch/lobby/reconnect
inspection. Preserve background concealment and normal deadlines; do not extend
timers merely to clear a test matrix.
The client connects to `http://localhost:8080` and `/ws/v2`. The private
`configs/playtest.yaml` overlay allows these six exact WebSocket origins;
using `127.0.0.1` instead of `localhost` is a different, unauthorized origin.

On one client choose **Host a Room**, then open its hosting screen and choose
**Check available modes**. Select the mode and four/six seats, choose **Create
room**, and share the displayed code. Other clients use **Join with Code**; if
the Local Room screen still asks for discovery, choose **Check available modes**
then **Join room**. Mark every
player Ready after the latest membership/settings change. Keep all seats
connected; this setup supplies no hidden bots. Quick Play can also assemble
clients that select the same mode, size and content language.

## Test content and boundaries

The server loads the checked-in
[`text-en`](../../server/pkg/media/testdata/text-en/manifest.json) fixture using
`KNOWOFF_TEXT_PROTOTYPE_PACK=/prototype`. Its release ID is
`synthetic-text-en`; schema 2, `text-v1`, language `en`, and all five modes are
pinned by its manifest and verified member hashes. It is explicitly synthetic
engineering content, not screened, human-approved or production-certified
content. Do not use its wording to assess final humor, content variety or
commercial readiness. It is mounted only in the server container; the Web
build does not contain the pack or private Nown catalog.

Gameplay tuning is unchanged. Use the normal actions, Ready shortcuts, voting,
results and rematch controls. A process restart interrupts live in-memory
matches; it cannot restore the lost board. A single client reconnect uses the
same account/session and must obey the original server deadlines.

Billing/provider acceptance, complete deletion, certified content, public
hosting, human cultural pilots and physical low-end-device performance remain
open roadmap gates. Wallet/account buttons elsewhere in the ordinary client do
not make this isolated test database a production account system. External
provider credentials, user terms and contribution screening are not configured;
free text or provider-backed actions can therefore refuse as designed.

## Rebuild and Android

```bash
make playtest.web       # rebuild the Web files served on all six origins
make playtest.android   # ordinary debug APK; no production billing credentials
```

The Web output is `client/build/web`. The APK is
`client/build/app/outputs/flutter-apk/app-debug.apk`. The Android emulator's
existing client configuration maps localhost to `10.0.2.2`, reaching host port
8080; install the APK on separate emulator instances for separate players, or
mix emulator and Web seats. The debug manifest permits local cleartext traffic.
A successful APK build is not evidence that a native UI journey was played.

Physical phones cannot reach a workstation's loopback through `10.0.2.2`.
This guide's launch profile deliberately remains loopback-only. Physical-device
networking and acceptance must be separately arranged and recorded; do not
publish this prototype through a tunnel merely to make a phone connect.

After rebuilding, hard-refresh each Web tab. If an old installed PWA persists,
unregister its service worker and clear that origin's site data before testing
again; doing so also removes that player's anonymous identity.

## Stop, reconnect and reset

```bash
make playtest.down     # stop containers; retain disposable DB and credentials
make playtest.up       # rebuild/start again; accounts survive, live matches do not
make playtest.reset    # stop and delete only knowoff-playtest database volumes
```

For a full reset, close the six player tabs, run `make playtest.reset`, clear
site data for localhost ports 8001–8006, then run `make playtest.up`. Clear app
data on any participating Android emulator too. Old browser/app credentials
refer to deleted accounts after the database reset. Keep the generated env file;
there is no need to rotate it for a playtest reset. Never reset the ordinary
stack or a retained database to recover a failed prototype test.

For diagnostics (commands keep secrets out of printed Compose configuration):

```bash
docker compose --project-name knowoff-playtest \
  --env-file docs/tracking/state/playtest.env \
  -f infra/compose/playtest.yaml ps
docker compose --project-name knowoff-playtest \
  --env-file docs/tracking/state/playtest.env \
  -f infra/compose/playtest.yaml logs --tail=100 server migrate
```

Record mode, size, client platform/build, observed actions and failures in the
playtest evidence report. Automated engine/socket tests, builds and actual UI
journeys are different evidence; mark journeys not observed as **not run**.
