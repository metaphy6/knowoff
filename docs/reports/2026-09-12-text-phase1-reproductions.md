# Phase 1: isolated text-transition regression evidence

Recorded 2026-09-12 at 09:08 UTC. This report preserves six **expected failures**
of the adopted text-transition rules against the existing v1 implementation.
It is engineering evidence for [Roadmap Phase 1](../planning/ROADMAP.md), not a
passing runtime gate or a claim that the five modes are implemented. The
[transition design](../design/DESIGN-text-transition.md#phase-1-executed-reproductions--2026-09-12)
records scope, root causes and the repair phases.

The probe uses only synthetic fixture data, anonymous temporary seats, an
in-process HTTP/WebSocket test server and a disposable Docker container. The
repository is mounted read-only; virtual Go test files are added with `-overlay`.
No application source/test file is edited and no existing database is contacted.
Go 1.25.14 with CGO enabled ran inside `knowoff-test-go:local`; avatar WebP remains
supported. Five production sources (match, payload, handler, room and active-pack
manager) matched HEAD `1862316`. Concurrent baseline changes to unrelated test
cases did not change the reused game setup helpers.

| Assertion | Actual result |
|---|---|
| Out-of-turn draw must reject without mutation | Accepted; current seat 1, actor 2, hand 5→6 |
| Draw identities are private to their owner | All three other recipients received drawn cards |
| Existing match renders its pinned content | Same-ID activation changed rendered wording despite old Pack pointer |
| Outbound gameplay events have sequence identity | Six startup/round events had sequence zero |
| Active-match reclaim restores authorized state | Only `joined_joined`, sequence zero; no hand/board/history/deadline/cursor or next event in 250 ms |
| Real room current-turn disconnect completes | Blocked beyond 250 ms; stack shows Room→Match→Room lock reentry |

The Go subprocess exited **1** with all six target assertions failing. The
collection driver exited **0 only after verifying those six expected failures**;
compile/setup errors, missing failures or a changed target result fail collection.
These intentional probe failures are kept outside the ordinary green test suite.
The eventual fixes must land their executable regressions in the owning phases,
including 4/6 seats, role privacy and the race detector; these four-seat probes
are not a substitute for that coverage.

## Recreate and run

From the repository root, prepare the existing test image if it is unavailable:

```bash
pwd
docker build -f server/Dockerfile.dev -t knowoff-test-go:local server
```

This uses the repository Dockerfile's container toolchain; it does not install
host OS packages. The command may require the same Docker/network access as the
normal validation runner. Extract the following three frozen artifacts to the
allowed temporary directory, generate the virtual-file map, and run the driver:

```bash
python3 - <<'PY_REPRO'
from pathlib import Path
import json, re
report = Path('docs/reports/2026-09-12-text-phase1-reproductions.md').read_text()
out = Path('/tmp/agent-runs/text-phase1-repros')
out.mkdir(parents=True, exist_ok=True)
for name, body in re.findall(r'<!-- artifact: ([a-z_]+\.\w+) -->\n```\w+\n(.*?)\n```', report, re.S):
    (out / name).write_text(body + '\n')
(out / 'overlay.json').write_text(json.dumps({'Replace': {
    '/workspace/server/internal/game/phase1_probe_test.go': str(out / 'game_probe_test.go'),
    '/workspace/server/internal/handler/phase1_probe_test.go': str(out / 'handler_probe_test.go'),
}}))
PY_REPRO
pwd
bash xops/agent/safe-run.sh text-phase1-repros -- \
  python3 /tmp/agent-runs/text-phase1-repros/run.py
```

A host `/tmp` bind was unavailable to this workstation's Docker daemon; the
driver streams a tar archive through stdin instead. The container extracts it
under its own `/tmp/agent-runs/` and uses the same overlay map. Each Go test
process has a 20-second bound, each network assertion has a read deadline, and
the parent subprocess has a 150-second outer bound. Unfixed blocked goroutines
are confined to the disposable test process.

The original successful collection log is
`/tmp/agent-runs/text-phase1-repros--20260912T090823Z-167633.log`.
Raw log SHA-256:
`8626b38abdf1dc3ca9fef40bda53f2d726a263b0d17faaeff436f10d0d6e6620`.
Reruns create `command.json`, `go-test.jsonl`, `go-test.exit`, and `summary.json`
beside the driver. Runtime timestamps, connection identities and synthetic match
seeds change between runs, so the old raw-log hash identifies the original
observation, not a deterministic replay requirement.

## Frozen temporary artifacts

<!-- artifact: run.py -->
```python
from pathlib import Path
import io, json, subprocess, sys, tarfile
p=Path(__file__).resolve().parent
cmd=['docker','run','--rm','-i','-v',f'{Path.cwd()}:/workspace:ro','-v','knowoff-tests-go-build:/root/.cache/go-build','-w','/workspace/server','-e','CGO_ENABLED=1','knowoff-test-go:local','sh','-c','mkdir -p /tmp/agent-runs/text-phase1-repros && tar -xf - -C /tmp/agent-runs/text-phase1-repros && go test -json -overlay /tmp/agent-runs/text-phase1-repros/overlay.json -count=1 -p=1 -timeout=20s -run \'^TestPhase1.*Probe$\' ./internal/game ./internal/handler']
(p/'command.json').write_text(json.dumps(cmd,indent=2)+'\n')
bundle=io.BytesIO()
with tarfile.open(fileobj=bundle,mode='w') as archive:
 for name in ['overlay.json','game_probe_test.go','handler_probe_test.go']: archive.add(p/name,arcname=name)
result=subprocess.run(cmd,input=bundle.getvalue(),stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=150)
result.stdout=result.stdout.decode()
(p/'go-test.jsonl').write_text(result.stdout)
(p/'go-test.exit').write_text(str(result.returncode)+'\n')
failures=[]
for line in result.stdout.splitlines():
 try:event=json.loads(line)
 except json.JSONDecodeError:
  print(line);continue
 if event.get('Action')=='fail' and event.get('Test'):failures.append(event['Test'])
 if 'TARGET FAILURE' in event.get('Output','') or event.get('Action')=='build-output':print(event.get('Output','').rstrip())
expected={'TestPhase1OutOfTurnDrawProbe','TestPhase1PublicDrawProbe','TestPhase1PackDriftProbe','TestPhase1SequenceProbe','TestPhase1ReconnectProbe','TestPhase1RoomDeadlockProbe'}
summary={'test_process_exit':result.returncode,'observed_failed_target_probes':sorted(failures),'expected_target_failures':sorted(expected),'runtime_fixed':False,'normal_suite_modified':False}
(p/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps(summary,indent=2))
if result.returncode!=1 or set(failures)!=expected or result.stdout.count('TARGET FAILURE')!=6:
 print('UNEXPECTED PROBE RESULT: inspect go-test.jsonl; evidence collection did not complete.')
 sys.exit(2)
print('Evidence collection complete: six expected target-contract failures reproduced; this is not a passing runtime gate.')
```

<!-- artifact: game_probe_test.go -->
```go
package game

import (
 "testing"
 "time"
 "github.com/knowoff/knowoff/server/internal/transport"
 "github.com/knowoff/knowoff/server/pkg/media"
)

func TestPhase1OutOfTurnDrawProbe(t *testing.T) {
 m, _ := newTestMatch(t,4,WithSeed(1),WithReplay(true))
 if err:=m.Start();err!=nil {t.Fatal(err)}
 m.beginRound()
 current:=m.turnOrder[m.currentTurn]; other:=(current+1)%4
 before:=len(m.players[other].Hand.Cards)
 err:=m.HandleIntent(other,transport.NewIntent(transport.IntentDrawCards,map[string]any{"count":float64(1)}))
 if err==nil {t.Errorf("TARGET FAILURE: out-of-turn draw accepted; current=%d actor=%d hand=%d->%d",current,other,before,len(m.players[other].Hand.Cards))}
}
func TestPhase1PublicDrawProbe(t *testing.T) {
 m,b:=newTestMatch(t,4,WithSeed(1),WithReplay(true))
 if err:=m.Start();err!=nil {t.Fatal(err)}
 m.beginRound(); b.clear()
 current:=m.turnOrder[m.currentTurn]
 if err:=m.HandleIntent(current,transport.NewIntent(transport.IntentDrawCards,map[string]any{"count":float64(1)}));err!=nil {t.Fatal(err)}
 leaked:=0
 for seat:=0;seat<4;seat++ {if seat==current {continue}; for _,e:=range b.messages[seat] {if cards,ok:=e.Payload["cards"].([]map[string]any);ok&&len(cards)>0 {leaked++}}}
 if leaked>0 {t.Errorf("TARGET FAILURE: drawn card identities delivered to %d non-owner recipients",leaked)}
}
func TestPhase1PackDriftProbe(t *testing.T) {
 pack:=loadGoldenPack(t)
 for _,n:=range pack.Media {n.Type=media.MediaTypeText;n.Content="synthetic original wording"}
 mgr:=media.NewManager(pack)
 r:=NewPayloadRenderer(mgr,media.NewSignedURLIssuer([]byte("synthetic-probe"),time.Minute),"")
 m:=NewMatch(4,Dependencies{Config:testConfig(4),Pack:pack,Renderer:r},newFakeBcast(4),WithSeed(1),WithReplay(true))
 if err:=m.Start();err!=nil {t.Fatal(err)}
 m.beginRound()
 replacement:=*pack; replacement.Media=make([]*media.MediaItem,len(pack.Media))
 for i,n:=range pack.Media {copy:=*n;copy.Content="synthetic replacement wording";replacement.Media[i]=&copy}
 mgr.Load(&replacement)
 payload,err:=m.deps.Renderer.NownPayload("round-1",m.nownSchedule[0],ViewNower)
 if err!=nil {t.Fatal(err)}
 if m.deps.Pack!=pack {t.Fatal("setup: original match pack pointer changed")}
 if payload["nown"].(map[string]any)["content"]!="synthetic original wording" {t.Error("TARGET FAILURE: unchanged match pack pointer rendered replacement pack wording")}
}
func TestPhase1SequenceProbe(t *testing.T) {
 m,b:=newTestMatch(t,4,WithSeed(1),WithReplay(true))
 if err:=m.Start();err!=nil {t.Fatal(err)}
 m.beginRound(); zero:=0
 for _,e:=range b.messages[0] {if e.Seq==0 {zero++}}
 if zero>0 {t.Errorf("TARGET FAILURE: %d startup/round events have zero sequence",zero)}
}
```

<!-- artifact: handler_probe_test.go -->
```go
package handler

import (
 "encoding/json"
 "fmt"
 "io"
 "log/slog"
 "net/http/httptest"
 "runtime"
 "strings"
 "testing"
 "time"
 "github.com/gorilla/websocket"
 "github.com/knowoff/knowoff/server/internal/config"
 "github.com/knowoff/knowoff/server/internal/game"
 "github.com/knowoff/knowoff/server/internal/lobby"
 "github.com/knowoff/knowoff/server/internal/transport"
 "github.com/knowoff/knowoff/server/pkg/media"
)

func phase1ProbeRoom(t *testing.T)(*lobby.Room,string,[]*websocket.Conn,[]string,int) {
 t.Helper()
 cfg:=&config.Config{WebSocket:config.WebSocketConfig{PongWaitS:30,PingPeriodS:30,WriteWaitS:1},RateLimit:config.RateLimitConfig{MaxBytesPerFrame:65536},Tuning:config.TuningConfig{
  Game:config.GameTuning{RoomSizes:[]int{4,6},DonowersBySize:map[int]int{4:1,6:2},VotesBySize:map[int]int{4:2,6:3},MinConnected:3,ReconnectGraceS:20},
  Timers:config.TimersTuning{PlayTurn:20,DiscussionPerPlayer:5,KnowoffBallot:20,KnowoffRunoff:15,VoteResultWindow:8,PrefetchCountdown:0},
  Hand:config.HandTuning{Size:5,DrawPile:3},
  Dealing:config.DealingTuning{BandHigh:0.55,BandLow:0.30,MinHighPerNown:2,MinDistantPerNown:2},
  Points:config.PointsTuning{DrawPenalty:5},
 }}
 pack,err:=media.LoadPack("../../pkg/media/testdata/golden-pack",media.DealingTuning{BandHigh:0.55,BandLow:0.30,MinHighPerNown:2,MinDistantPerNown:2})
 if err!=nil {t.Fatal(err)}
 logger:=slog.New(slog.NewTextHandler(io.Discard,nil)); manager:=media.NewManager(pack)
 lm:=lobby.NewManager(lobby.Deps{Config:cfg,Logger:logger,Pack:pack,Manager:manager,NodeID:"synthetic-probe"})
 room,err:=lm.CreateRoom(4);if err!=nil {t.Fatal(err)}
 room.SetOnStart(nil)
 srv:=httptest.NewServer(RealtimeHandler(HandlerDeps{Config:cfg,Logger:logger,Lobby:lm}));t.Cleanup(srv.Close)
 url:="ws"+strings.TrimPrefix(srv.URL,"http")
 clients:=make([]*websocket.Conn,4);tokens:=make([]string,4)
 for i:=0;i<4;i++ {
  c,_,err:=websocket.DefaultDialer.Dial(url,nil);if err!=nil {t.Fatal(err)};clients[i]=c;t.Cleanup(func(){c.Close()})
  if err:=c.WriteJSON(transport.NewIntent(transport.IntentJoinRoom,map[string]any{"code":room.Code}));err!=nil {t.Fatal(err)}
  c.SetReadDeadline(time.Now().Add(time.Second));var env transport.Envelope
  if err:=c.ReadJSON(&env);err!=nil {t.Fatal(err)}
  if env.Kind!="joined_joined" {t.Fatalf("setup expected joined, got %q",env.Kind)}
  tokens[i],_=env.Payload["session_token"].(string)
 }
 renderer:=game.NewPayloadRenderer(manager,media.NewSignedURLIssuer([]byte("synthetic-probe"),time.Minute),"")
 if err:=room.StartMatch(game.Dependencies{Config:cfg,Pack:pack,Renderer:renderer});err!=nil {t.Fatal(err)}
 clients[0].SetReadDeadline(time.Now().Add(2*time.Second))
 for {
  var env transport.Envelope;if err:=clients[0].ReadJSON(&env);err!=nil {t.Fatal(err)}
  if env.Kind==transport.EventTurnStarted {return room,url,clients,tokens,int(env.Payload["turn_seat"].(float64))}
 }
}
func TestPhase1ReconnectProbe(t *testing.T) {
 room,url,clients,tokens,current:=phase1ProbeRoom(t);seat:=(current+1)%4
 clients[seat].Close()
 // The handler's ordinary disconnect cleanup must finish before rebinding.
 deadline:=time.Now().Add(time.Second)
 for room.Connection(seat)!=nil {if time.Now().After(deadline){t.Fatal("setup non-current disconnect did not finish")};time.Sleep(time.Millisecond)}
 c,_,err:=websocket.DefaultDialer.Dial(url,nil);if err!=nil {t.Fatal(err)};defer c.Close()
 if err:=c.WriteJSON(transport.NewIntent(transport.IntentJoinRoom,map[string]any{"code":room.Code,"session_token":tokens[seat]}));err!=nil {t.Fatal(err)}
 c.SetReadDeadline(time.Now().Add(time.Second));var joined transport.Envelope
 if err:=c.ReadJSON(&joined);err!=nil {t.Fatal(err)}
 if joined.Kind!="joined_joined" {t.Fatalf("setup reclaim returned %q",joined.Kind)}
 c.SetReadDeadline(time.Now().Add(250*time.Millisecond));_,data,readErr:=c.ReadMessage()
 if readErr==nil {var extra transport.Envelope;if err:=json.Unmarshal(data,&extra);err!=nil {t.Fatal(err)};t.Fatalf("unexpected follow-up %s",extra.Kind)}
 if !websocket.IsUnexpectedCloseError(readErr) && !strings.Contains(readErr.Error(),"timeout") {t.Fatalf("unexpected read failure: %v",readErr)}
 missing:=0;for _,field:=range []string{"hand","board","history","deadline","through_sequence"} {if _,ok:=joined.Payload[field];!ok {missing++}}
 if missing==5 {t.Errorf("TARGET FAILURE: active-match reclaim returned joined only; 5 snapshot fields absent; no next event within 250ms; joined seq=%d",joined.Seq)}
}
func TestPhase1RoomDeadlockProbe(t *testing.T) {
 room,_,_,_,current:=phase1ProbeRoom(t)
 done:=make(chan struct{});go func(){room.SetConnection(current,nil);close(done)}()
 select {case <-done:return;case <-time.After(250*time.Millisecond):}
 buf:=make([]byte,128<<10);n:=runtime.Stack(buf,true);stack:=string(buf[:n])
 for _,needle:=range []string{"(*Room).Broadcast","(*Match).autoPass","(*Room).SetConnection"} {if !strings.Contains(stack,needle){t.Fatalf("blocked but missing expected stack %q",needle)}}
 for _,line:=range strings.Split(stack,"\n") {if strings.Contains(line,"/internal/lobby/room.go:")||strings.Contains(line,"/internal/game/match.go:")||strings.Contains(line,"(*Room).Broadcast")||strings.Contains(line,"(*Match).autoPass")||strings.Contains(line,"(*Room).SetConnection"){fmt.Println(line)}}
 t.Error("TARGET FAILURE: actual websocket room current-turn disconnect blocked beyond 250ms on Room->Match->Room broadcast lock reentry")
}
```
