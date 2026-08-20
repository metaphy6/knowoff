// Package workbench provides a minimal, server-rendered media curation UI for
// development and staging. It is mounted on the admin port and must not be
// reachable from the public ingress.
package workbench

import (
	"fmt"
	"html/template"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/knowoff/knowoff/server/pkg/media"
)

// Workbench is the dev-only media curation surface.
type Workbench struct {
	manager     *media.Manager
	ingestPath  string
	ingestFiles []string
	ingestMu    sync.RWMutex
	ingestTick  *time.Ticker
	stopCh      chan struct{}

	keptMu sync.RWMutex
	kept   map[string]bool // item id -> keep decision (true=keep, false=kill)
}

// New returns a Workbench backed by the given media manager. ingestPath is the
// directory to scan for newly generated assets (e.g. ComfyUI / Ollama output).
func New(manager *media.Manager, ingestPath string) *Workbench {
	wb := &Workbench{
		manager:    manager,
		ingestPath: ingestPath,
		stopCh:     make(chan struct{}),
		kept:       make(map[string]bool),
	}
	wb.scanIngest()
	wb.ingestTick = time.NewTicker(5 * time.Second)
	go wb.watchLoop()
	return wb
}

// Close stops the background ingest scanner.
func (wb *Workbench) Close() {
	if wb.ingestTick != nil {
		wb.ingestTick.Stop()
	}
	close(wb.stopCh)
}

func (wb *Workbench) watchLoop() {
	for {
		select {
		case <-wb.ingestTick.C:
			wb.scanIngest()
		case <-wb.stopCh:
			return
		}
	}
}

func (wb *Workbench) scanIngest() {
	var files []string
	_ = filepath.Walk(wb.ingestPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	wb.ingestMu.Lock()
	wb.ingestFiles = files
	wb.ingestMu.Unlock()
}

// Register mounts workbench routes on mux under /workbench/.
func (wb *Workbench) Register(mux *http.ServeMux) {
	mux.HandleFunc("/workbench/", wb.index)
	mux.HandleFunc("/workbench/simulate", wb.simulate)
	mux.HandleFunc("/workbench/nearest", wb.nearest)
	mux.HandleFunc("/workbench/grid", wb.grid)
	mux.HandleFunc("/workbench/ingest", wb.ingest)
}

func (wb *Workbench) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/workbench/" {
		http.NotFound(w, r)
		return
	}
	fmt.Fprint(w, `<!doctype html>
<html><head><title>Knowoff Media Workbench</title></head><body>
<h1>Media Workbench</h1>
<ul>
<li><a href="/workbench/simulate">Deal Simulator</a></li>
<li><a href="/workbench/nearest">Nearest Neighbor</a></li>
<li><a href="/workbench/grid">Bulk Grid</a></li>
<li><a href="/workbench/ingest">Ingest Watch</a></li>
</ul>
</body></html>`)
}

func (wb *Workbench) simulate(w http.ResponseWriter, r *http.Request) {
	pack := wb.manager.Active()
	if pack == nil {
		http.Error(w, "no pack loaded", http.StatusServiceUnavailable)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		tableSize, _ := strconv.Atoi(r.FormValue("table_size"))
		if tableSize != 4 && tableSize != 6 {
			tableSize = 6
		}
		nownID := r.FormValue("nown_id")
		seed, _ := strconv.ParseInt(r.FormValue("seed"), 10, 64)

		dealer := media.NewDealer(pack, media.DefaultDealingTuning())
		rng := rand.New(rand.NewSource(seed))
		var nownIDs []string
		if nownID != "" {
			nownIDs = []string{nownID}
		} else {
			nownIDs = sampleNownIDs(pack, 1, rng)
		}
		result, err := dealer.Deal(tableSize, nownIDs, rng)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, "<h2>Simulated %d-player deal for Nown %s</h2>", tableSize, nownIDs[0])
		for i, hand := range result.Hands {
			fmt.Fprintf(w, "<h3>Player %d</h3><p>Cards: %s</p><p>Draw pile: %s</p>", i+1, strings.Join(hand.Cards, ", "), strings.Join(hand.DrawPile, ", "))
		}
		return
	}

	fmt.Fprint(w, `<form method="post">
<label>Table size: <select name="table_size"><option value="4">4</option><option value="6" selected>6</option></select></label><br>
<label>Nown ID (leave blank for random): <input name="nown_id"></label><br>
<label>Seed: <input name="seed" value="42"></label><br>
<button>Simulate</button>
</form>`)
}

func (wb *Workbench) nearest(w http.ResponseWriter, r *http.Request) {
	pack := wb.manager.Active()
	if pack == nil {
		http.Error(w, "no pack loaded", http.StatusServiceUnavailable)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		nownID := r.FormValue("nown_id")
		topK, _ := strconv.Atoi(r.FormValue("top_k"))
		if topK <= 0 {
			topK = 10
		}

		var nown *media.MediaItem
		for _, n := range pack.Media {
			if n.ID == nownID {
				nown = n
				break
			}
		}
		if nown == nil {
			http.Error(w, "nown not found", http.StatusNotFound)
			return
		}

		type scored struct {
			card  *media.CardItem
			score float64
		}
		var scoredCards []scored
		for _, c := range pack.Cards {
			scoredCards = append(scoredCards, scored{card: c, score: media.Cosine(nown.Embedding, c.Embedding)})
		}
		sort.Slice(scoredCards, func(i, j int) bool { return scoredCards[i].score > scoredCards[j].score })

		fmt.Fprintf(w, "<h2>Nearest cards to %s</h2><table border=1><tr><th>ID</th><th>Score</th><th>Band</th><th>Tone</th></tr>", nownID)
		tuning := media.DefaultDealingTuning()
		for i := 0; i < topK && i < len(scoredCards); i++ {
			cs := scoredCards[i]
			band := media.BandFor(cs.score, tuning)
			fmt.Fprintf(w, "<tr><td>%s</td><td>%.3f</td><td>%s</td><td>%s</td></tr>", cs.card.ID, cs.score, band, cs.card.ToneBucket)
		}
		fmt.Fprint(w, "</table>")
		return
	}

	fmt.Fprint(w, `<form method="post">
<label>Nown ID: <input name="nown_id"></label><br>
<label>Top K: <input name="top_k" value="10"></label><br>
<button>Show nearest</button>
</form>`)
}

func (wb *Workbench) grid(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		id := r.FormValue("id")
		action := r.FormValue("action")
		if id != "" && (action == "keep" || action == "kill") {
			wb.keptMu.Lock()
			wb.kept[id] = action == "keep"
			wb.keptMu.Unlock()
		}
	}

	pack := wb.manager.Active()
	if pack == nil {
		http.Error(w, "no pack loaded", http.StatusServiceUnavailable)
		return
	}

	wb.keptMu.RLock()
	kept := make(map[string]bool, len(wb.kept))
	for k, v := range wb.kept {
		kept[k] = v
	}
	wb.keptMu.RUnlock()

	keptCount, killedCount := 0, 0
	for _, v := range kept {
		if v {
			keptCount++
		} else {
			killedCount++
		}
	}
	total := keptCount + killedCount
	keepRate := 0.0
	if total > 0 {
		keepRate = float64(keptCount) / float64(total) * 100
	}

	fmt.Fprintf(w, "<h2>Bulk Grid — Keep rate: %.1f%% (%d kept, %d killed, %d total)</h2>", keepRate, keptCount, killedCount, total)
	fmt.Fprint(w, "<table border=1><tr><th>ID</th><th>Kind</th><th>Type</th><th>Tone</th><th>Rating</th><th>Decision</th><th>Actions</th></tr>")
	for _, n := range pack.Media {
		decision := "—"
		if v, ok := kept[n.ID]; ok {
			if v {
				decision = "KEEP"
			} else {
				decision = "KILL"
			}
		}
		fmt.Fprintf(w, "<tr><td>%s</td><td>Nown</td><td>%s</td><td>%s</td><td>%s</td><td>%s</td><td>"+
			"<form method=post style=display:inline><input type=hidden name=id value=%s><input type=hidden name=action value=keep><button>Keep</button></form> "+
			"<form method=post style=display:inline><input type=hidden name=id value=%s><input type=hidden name=action value=kill><button>Kill</button></form></td></tr>",
			n.ID, n.Type, n.ToneBucket, n.Rating, decision, n.ID, n.ID)
	}
	for i, c := range pack.Cards {
		if i >= 100 {
			break
		}
		decision := "—"
		if v, ok := kept[c.ID]; ok {
			if v {
				decision = "KEEP"
			} else {
				decision = "KILL"
			}
		}
		fmt.Fprintf(w, "<tr><td>%s</td><td>Card</td><td>%s</td><td>%s</td><td>—</td><td>%s</td><td>"+
			"<form method=post style=display:inline><input type=hidden name=id value=%s><input type=hidden name=action value=keep><button>Keep</button></form> "+
			"<form method=post style=display:inline><input type=hidden name=id value=%s><input type=hidden name=action value=kill><button>Kill</button></form></td></tr>",
			c.ID, c.Type, c.ToneBucket, decision, c.ID, c.ID)
	}
	fmt.Fprint(w, "</table>")
}

func (wb *Workbench) ingest(w http.ResponseWriter, r *http.Request) {
	wb.ingestMu.RLock()
	files := append([]string(nil), wb.ingestFiles...)
	wb.ingestMu.RUnlock()

	fmt.Fprintf(w, "<h2>Ingest watch: %s</h2><p>%d files found</p><ul>", template.HTMLEscapeString(wb.ingestPath), len(files))
	for _, f := range files {
		fmt.Fprintf(w, "<li>%s</li>", template.HTMLEscapeString(f))
	}
	fmt.Fprint(w, "</ul>")
}

func sampleNownIDs(pack *media.Pack, n int, rng *rand.Rand) []string {
	if n >= len(pack.Media) {
		var ids []string
		for _, m := range pack.Media {
			ids = append(ids, m.ID)
		}
		return ids
	}
	perm := rng.Perm(len(pack.Media))[:n]
	var ids []string
	for _, i := range perm {
		ids = append(ids, pack.Media[i].ID)
	}
	return ids
}

// Silence unused imports if the package grows; math is used above.
var _ = math.Pi
