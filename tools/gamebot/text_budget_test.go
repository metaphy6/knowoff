package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/lobby"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

type textBudgetRoom struct {
	peers      []*textNetworkBot
	mode       gamecontract.ModeID
	size       int
	done       bool
	latencies  []time.Duration
	worstFrame int
	pages      int
}

// The script accepts at most one action per room on each of 500 steps.
// Refuse overflow explicitly instead of silently sampling the fastest requests.
type textBudgetObservations struct {
	mu       sync.Mutex
	samples  map[gamecontract.ModeID][]time.Duration
	count    int
	overflow bool
}

func (o *textBudgetObservations) observe(mode gamecontract.ModeID, elapsed time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.count == 100*500 {
		o.overflow = true
		return
	}
	if o.samples == nil {
		o.samples = make(map[gamecontract.ModeID][]time.Duration)
	}
	o.samples[mode] = append(o.samples[mode], elapsed)
	o.count++
}

func (o *textBudgetObservations) take() (map[gamecontract.ModeID][]time.Duration, int, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	samples, count, overflow := o.samples, o.count, o.overflow
	o.samples, o.count, o.overflow = nil, 0, false
	return samples, count, overflow
}

func TestTextBudgetObservationsPreserveAllSamplesAndExposeOverflow(t *testing.T) {
	o := &textBudgetObservations{}
	var workers sync.WaitGroup
	for _, mode := range gamecontract.AllModes() {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := 0; i < 10000; i++ {
				o.observe(mode, time.Duration(i+1))
			}
		}()
	}
	workers.Wait()
	o.observe(gamecontract.ModeMissedTheBriefing, time.Hour)
	samples, count, overflow := o.take()
	if count != 50000 || !overflow || len(samples) != 5 {
		t.Fatal("observation bound changed", count, overflow)
	}
	for _, mode := range gamecontract.AllModes() {
		if len(samples[mode]) != 10000 || samples[mode][0] != 1 || samples[mode][9999] != 10000 {
			t.Fatal("samples lost/reordered", mode)
		}
	}
	o.observe(gamecontract.ModeMissedTheBriefing, time.Minute)
	samples, count, overflow = o.take()
	if count != 1 || overflow || len(samples) != 1 || samples[gamecontract.ModeMissedTheBriefing][0] != time.Minute {
		t.Fatal("warm observations retained")
	}
}

func textBudgetPercentile(samples []time.Duration, fraction float64) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	copy := append([]time.Duration(nil), samples...)
	sort.Slice(copy, func(i, j int) bool { return copy[i] < copy[j] })
	return copy[max(0, min(len(copy)-1, int(math.Ceil(fraction*float64(len(copy))))-1))]
}

func TestTextBudgetPercentilesPreserveTailAndInput(t *testing.T) {
	samples := []time.Duration{5, 1, 3, 2, 4}
	if textBudgetPercentile(samples, .95) != 5 || textBudgetPercentile(samples, .5) != 3 || samples[0] != 5 {
		t.Fatal("tail or input changed")
	}
	if textBudgetPercentile(nil, .99) != 0 {
		t.Fatal("empty measurements fabricated")
	}
}

func (r *textBudgetRoom) collectFrames() error {
	for _, peer := range r.peers {
		for _, frame := range peer.trace {
			if frame.Direction != "server" {
				continue
			}
			raw, err := json.Marshal(frame.Frame)
			if err != nil {
				return err
			}
			if len(raw) > peer.limits.MaxFrameBytes {
				return fmt.Errorf("frame budget exceeded: %d", len(raw))
			}
			r.worstFrame = max(r.worstFrame, len(raw))
		}
		r.pages = max(r.pages, peer.pageCount)
		// The normal tool retains private traces for replay. A sustained load
		// fixture measures each validated frame then releases its private bytes.
		peer.trace = nil
		peer.traceBytes = 0
	}
	return nil
}

func textBudgetParallel(rooms []*textBudgetRoom, operation func(*textBudgetRoom) error) error {
	var group sync.WaitGroup
	errors := make(chan error, len(rooms))
	for _, room := range rooms {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := operation(room); err != nil {
				errors <- fmt.Errorf("%s/%d: %w", room.mode, len(room.peers), err)
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		return err
	}
	return nil
}

func textBudgetStart(ctx context.Context, f *networkFixture, mode gamecontract.ModeID, size, ordinal int) (*textBudgetRoom, error) {
	r := &textBudgetRoom{mode: mode, size: size}
	endpoint := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/ws/v2"
	complete := false
	defer func() {
		if !complete {
			for _, peer := range r.peers {
				peer.close()
			}
		}
	}()
	for i := 0; i < size; i++ {
		token, err := networkDevelopmentAuth(ctx, endpoint, "disposable-network-fixture-key-for-local-tests")
		if err != nil {
			return nil, err
		}
		peer, err := connectTextNetwork(ctx, endpoint, token, int64(10000+ordinal*10+i))
		if err != nil {
			return nil, err
		}
		r.peers = append(r.peers, peer)
	}
	settings := v2.LobbySettings{ModeID: mode, Size: size, ContentLanguage: "en", PackReleaseID: "synthetic-text-en", RulesVersion: "text-v1"}
	if err := r.peers[0].control(ctx, "room_create", settings); err != nil {
		return nil, err
	}
	code := r.peers[0].lobby.Code
	for _, peer := range r.peers[1:] {
		if err := peer.control(ctx, "room_join", map[string]string{"code": code}); err != nil {
			return nil, err
		}
	}
	for _, peer := range r.peers {
		if err := peer.control(ctx, "resync", struct{}{}); err != nil {
			return nil, err
		}
		state := peer.lobby.Lobby
		if err := peer.control(ctx, "room_ready", v2.ReadyAcknowledgement{SettingsRevision: state.SettingsRevision, MembershipRevision: state.MembershipRevision}); err != nil {
			return nil, err
		}
	}
	if err := r.peers[0].control(ctx, "room_start", struct{}{}); err != nil {
		return nil, err
	}
	complete = true
	return r, nil
}

func textBudgetPlay(t *testing.T, ctx context.Context, f *networkFixture, rooms []*textBudgetRoom) error {
	for step := 0; step < 500; step++ {
		refreshStart := time.Now()
		// All rooms finish snapshot refresh before concurrent measured intents.
		if err := textBudgetParallel(rooms, func(r *textBudgetRoom) error {
			// Completed spectators keep reading, including websocket ping frames,
			// until every concurrent room has reached its settlement barrier.
			for seat, peer := range r.peers {
				started := time.Now()
				if err := peer.control(ctx, "resync", struct{}{}); err != nil {
					return fmt.Errorf("step %d refresh seat %d phase %s round %d turn %d history %d elapsed %s: %w", step, seat, peer.snapshot.Phase, peer.snapshot.Round, peer.snapshot.Turn, len(peer.snapshot.History), time.Since(started), err)
				}
			}
			first := r.peers[0].snapshot
			if first.Phase == v2.PhaseVerdict {
				if first.Verdict == nil || first.Verdict.Outcome != "completed" || first.PendingOffer != nil {
					return fmt.Errorf("nonordinary verdict: %v", first.Verdict)
				}
				r.done = true
			}
			return r.collectFrames()
		}); err != nil {
			return err
		}
		remaining, actions := 0, 0
		next := int64(math.MaxInt64)
		type choice struct {
			room   *textBudgetRoom
			peer   *textNetworkBot
			action v2.Action
		}
		choices := []choice{}
		for _, room := range rooms {
			if room.done {
				continue
			}
			remaining++
			next = min(next, room.peers[0].snapshot.DeadlineMS)
			for _, peer := range room.peers {
				if action := textPolicy(peer.snapshot, peer.rng); action != nil {
					choices = append(choices, choice{room, peer, *action})
					break
				}
			}
		}
		if step%10 == 0 {
			fmt.Printf("load_step=%d rooms=%d remaining=%d choices=%d refresh=%s\n", step, len(rooms), remaining, len(choices), time.Since(refreshStart))
		}
		if remaining == 0 {
			return nil
		}
		var group sync.WaitGroup
		errors := make(chan error, len(choices))
		for _, chosen := range choices {
			actions++
			group.Add(1)
			go func() {
				defer group.Done()
				start := time.Now()
				err := chosen.peer.act(ctx, chosen.action)
				elapsed := time.Since(start)
				if err != nil {
					errors <- fmt.Errorf("step %d %s/%d action %s phase %s round %d turn %d elapsed %s: %w", step, chosen.room.mode, len(chosen.room.peers), chosen.action.Kind, chosen.peer.snapshot.Phase, chosen.peer.snapshot.Round, chosen.peer.snapshot.Turn, elapsed, err)
					return
				}
				chosen.room.latencies = append(chosen.room.latencies, elapsed)
			}()
		}
		group.Wait()
		close(errors)
		for err := range errors {
			return err
		}
		if actions == 0 {
			if next <= f.now.Load() || next == math.MaxInt64 {
				return fmt.Errorf("stalled fixture clock: %d", next)
			}
			if err := f.advance(ctx, next); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("budget match exceeded bounded 500-step script")
}

func textBudgetCleanup(ctx context.Context, f *networkFixture, rooms []*textBudgetRoom) error {
	var available time.Time
	if err := f.db.QueryRowContext(ctx, `SELECT max(available_at) FROM text_outbox`).Scan(&available); err != nil {
		return err
	}
	if available.UnixMilli()+1 > f.now.Load() {
		if err := f.advance(ctx, available.UnixMilli()+1); err != nil {
			return err
		}
	}
	// The production worker claims at most 100 recipients per pass. Drain a
	// bounded number of passes without advancing leases and duplicating delivery.
	recipients := 0
	for _, room := range rooms {
		recipients += room.size
	}
	for i := 0; i < (recipients+99)/100; i++ {
		if err := f.manager.PumpDeliveries(ctx, f.values); err != nil {
			return err
		}
	}
	if err := textBudgetParallel(rooms, func(r *textBudgetRoom) error {
		for _, peer := range r.peers {
			if err := peer.control(ctx, "resync", struct{}{}); err != nil {
				return err
			}
			if len(peer.deliveries) != 1 {
				return fmt.Errorf("private settlement count %d", len(peer.deliveries))
			}
			for id := range peer.deliveries {
				if err := peer.control(ctx, "settlement_ack", map[string]int64{"id": id}); err != nil {
					return err
				}
			}
			if err := peer.control(ctx, "room_leave", struct{}{}); err != nil {
				return err
			}
			peer.close()
		}
		if err := r.collectFrames(); err != nil {
			return err
		}
		r.peers = nil
		return nil
	}); err != nil {
		return err
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		counts, err := f.manager.ResourceCounts(ctx)
		if err != nil {
			return err
		}
		if counts == (lobby.TextResourceCounts{}) && f.connections.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func TestTextNetworkHundredRoomsAndMatchSoak(t *testing.T) {
	observations := &textBudgetObservations{}
	f := newNetworkFixtureWithObserver(t, observations.observe)
	ctx, cancel := context.WithTimeout(t.Context(), 33*time.Minute)
	defer cancel()
	t.Logf("host go=%s os=%s arch=%s cpus=%d GOMAXPROCS=%d network=loopback prototype=true heap_scope=server_and_clients", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0))
	var pgVersion string
	if err := f.db.QueryRowContext(ctx, "SHOW server_version").Scan(&pgVersion); err != nil {
		t.Fatal(err)
	}
	t.Logf("postgres=%s frame_bounds=%+v source_capture=run_start", pgVersion, f.manager.Limits())
	if build, ok := debug.ReadBuildInfo(); ok {
		t.Logf("build=%s", build.String())
	}
	// Hash checked-in inputs, including the authoritative server and network tool.
	// A final proof additionally freezes this manifest across the whole run.
	for _, dir := range []string{"../../configs", "../../server", "."} {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".go" && ext != ".sql" && ext != ".yaml" && ext != ".json" && ext != ".jsonl" && entry.Name() != "go.mod" && entry.Name() != "go.sum" {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			t.Logf("input path=%s sha256=%x", path, sha256.Sum256(raw))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/proc/cpuinfo", "/proc/meminfo"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("host inventory %s: %v", path, err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "MemTotal:") {
				t.Logf("host %s", line)
				break
			}
		}
	}
	run := func(count int) []*textBudgetRoom {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		setup := time.Now()
		rooms := make([]*textBudgetRoom, 0, count)
		for i := 0; i < count; i++ {
			size := 4
			if i%2 == 1 {
				size = 6
			}
			room, err := textBudgetStart(ctx, f, gamecontract.AllModes()[(i/2)%5], size, i)
			if err != nil {
				t.Fatal(err)
			}
			rooms = append(rooms, room)
			t.Cleanup(func() {
				for _, peer := range room.peers {
					peer.close()
				}
			})
		}
		if got := f.manager.ActiveMatches(); got != count {
			t.Fatalf("start barrier: %d actual active rooms, want %d", got, count)
		}
		counts, err := f.manager.ResourceCounts(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("literal start barrier rooms=%d sockets=%d resources=%+v setup=%s", count, f.connections.Load(), counts, time.Since(setup))
		if count == 100 {
			samples := make([]time.Duration, 0, 100)
			for i := 0; i < 100; i++ {
				start := time.Now()
				if err := rooms[0].peers[0].control(ctx, "resync", struct{}{}); err != nil {
					t.Fatal(err)
				}
				samples = append(samples, time.Since(start))
			}
			fmt.Printf("idle_ws_control_rtt=%v p95=%s p99=%s\n", samples, textBudgetPercentile(samples, .95), textBudgetPercentile(samples, .99))
			if err := rooms[0].collectFrames(); err != nil {
				t.Fatal(err)
			}
		}
		if err := textBudgetPlay(t, ctx, f, rooms); err != nil {
			t.Fatal(err)
		}
		cleanup, finish := context.WithTimeout(ctx, 30*time.Second)
		defer finish()
		if err := textBudgetCleanup(cleanup, f, rooms); err != nil {
			t.Fatal(err)
		}
		return rooms
	}
	// Warm the same 100 rooms/500 sockets, ten mode/size cells and full-match
	// paths as the measured workload. Both samples follow complete cleanup and
	// two collections, so sync.Pool victim generations are treated identically.
	for _, room := range run(100) {
		room.latencies = nil
	}
	if _, _, overflow := observations.take(); overflow {
		t.Fatal("warm server observation overflow")
	}
	runtime.GC()
	runtime.GC()
	var warm runtime.MemStats
	runtime.ReadMemStats(&warm)
	warmGoroutines := runtime.NumGoroutine()
	rooms := run(100)
	serverSamples, serverCount, serverOverflow := observations.take()
	if serverOverflow {
		t.Fatal("server observation overflow")
	}
	var all []time.Duration
	cells := map[string][]time.Duration{}
	worst := 0
	for i, r := range rooms {
		if len(r.latencies) == 0 {
			t.Fatal("completed match had no measured actions")
		}
		all = append(all, r.latencies...)
		cell := fmt.Sprintf("%s/%d", r.mode, r.size)
		cells[cell] = append(cells[cell], r.latencies...)
		worst = max(worst, r.worstFrame)
		// Emit raw bounded evidence directly to the test process output, which
		// safe-run retains outside this heap. testing.T's retained log buffer must
		// not become an apparent application leak between the two measurements.
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Room       int             `json:"room"`
			Cell       string          `json:"cell"`
			Samples    []time.Duration `json:"action_rtt_ns"`
			WorstFrame int             `json:"worst_frame"`
			Pages      int             `json:"pages"`
		}{i, cell, r.latencies, r.worstFrame, r.pages}); err != nil {
			t.Fatal(err)
		}
		r.latencies = nil
	}
	for cell, samples := range cells {
		fmt.Printf("cell=%s samples=%d p95=%s p99=%s\n", cell, len(samples), textBudgetPercentile(samples, .95), textBudgetPercentile(samples, .99))
		if len(samples) < 100 {
			t.Errorf("cell %s has fewer than 100 action samples", cell)
		}
	}
	cells = nil
	if serverCount != len(all) {
		t.Errorf("server accepted requests=%d, client acknowledgements=%d", serverCount, len(all))
	}
	var serverAll []time.Duration
	for _, mode := range gamecontract.AllModes() {
		samples := serverSamples[mode]
		serverAll = append(serverAll, samples...)
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Mode    gamecontract.ModeID `json:"mode"`
			Samples []time.Duration     `json:"server_accepted_action_ns"`
		}{mode, samples}); err != nil {
			t.Fatal(err)
		}
	}
	serverP95, serverP99 := textBudgetPercentile(serverAll, .95), textBudgetPercentile(serverAll, .99)
	serverSamples, serverAll = nil, nil
	p95, p99 := textBudgetPercentile(all, .95), textBudgetPercentile(all, .99)
	all = nil
	rooms = nil
	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	afterGoroutines := runtime.NumGoroutine()
	t.Logf("result action_rtt_p95=%s action_rtt_p99=%s worst_frame=%d warm_heap=%d final_heap=%d warm_goroutines=%d final_goroutines=%d", p95, p99, worst, warm.HeapAlloc, after.HeapAlloc, warmGoroutines, afterGoroutines)
	t.Logf("diagnostic server_accepted_action_p95=%s server_accepted_action_p99=%s samples=%d", serverP95, serverP99, serverCount)
	if p95 > 200*time.Millisecond || p99 > 500*time.Millisecond {
		t.Error("intent-to-ack upper bound exceeded 200ms/500ms budget")
	}
	if after.HeapAlloc > warm.HeapAlloc+warm.HeapAlloc/10 {
		t.Error("process-wide retained heap exceeds warmed baseline by more than 10%")
	}
	if afterGoroutines > warmGoroutines {
		t.Error("goroutines did not return to warmed baseline")
	}
	networkZero(t, f.db, `SELECT count(*) FROM text_matches WHERE state<>'completed'`)
	networkZero(t, f.db, `SELECT count(*) FROM text_settlements WHERE state='pending'`)
	networkZero(t, f.db, `SELECT count(*) FROM text_outbox WHERE acknowledged_at IS NULL`)
	networkZero(t, f.db, `SELECT count(*) FROM noin_ledger`)
}
