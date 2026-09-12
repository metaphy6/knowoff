package profile

import (
	"context"
	"database/sql"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

func testProgression() config.ProgressionTuning {
	return config.ProgressionTuning{
		XPBase:           10,
		XPPerCorrectVote: 5,
		XPWinBonus:       20,
		LevelThresholds:  []int{0, 50, 120},
	}
}

func TestProfileCurrentWeekWinnerIsSeparateFromLifetimeCount(t *testing.T) {
	if os.Getenv("KNOWOFF_TEST_DSN") == "" {
		t.Fatal("real PostgreSQL required")
	}
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	m := NewManager(db)
	first, next := uuid.NewString(), uuid.NewString()
	for _, id := range []string{first, next} {
		if err := ensureTestProfile(ctx, db, id, "winner-"+id[:8]); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE profiles SET week_winner_titles=4,non_converted_points=200 WHERE account_id=$1`, id); err != nil {
			t.Fatal(err)
		}
		p, err := m.Get(ctx, id, false)
		if err != nil || p.CurrentWeekWinner || p.WeekWinnerTitles != 4 || p.NonConvertedPoints != 0 {
			t.Fatal("lifetime count became current title", p, err)
		}
	}
	terms := "title-" + uuid.NewString()
	if _, err := db.Exec(`INSERT INTO portal_terms(version,title,body,active_from) VALUES($1,'Test only title','Test only',now())`, terms); err != nil {
		t.Fatal(err)
	}
	topic, source := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at) VALUES($1,$2,'text','Test title source','approved',$3,now())`, source, first, terms); err != nil {
		t.Fatal(err)
	}
	week := time.Date(2091, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO challenge_topics(id,week_start,week_end,nown_media_id) VALUES($1,$2,$3,$4)`, topic, week, week.AddDate(0, 0, 7), source); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first, next} {
		entry := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO challenge_entries(id,account_id,topic_id,entry_type,content,terms_version,terms_accepted_at) VALUES($1,$2,$3,'text','Test winner',$4,now())`, entry, id, topic, terms); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO challenge_current_winner(singleton,topic_id,entry_id,account_id,week_start,crowned_at) VALUES(true,$1,$2,$3,$4,now()) ON CONFLICT(singleton) DO UPDATE SET topic_id=EXCLUDED.topic_id,entry_id=EXCLUDED.entry_id,account_id=EXCLUDED.account_id,week_start=EXCLUDED.week_start,crowned_at=EXCLUDED.crowned_at`, topic, entry, id, week); err != nil {
			t.Fatal(err)
		}
		p, err := m.Get(ctx, id, false)
		if err != nil || !p.CurrentWeekWinner || p.WeekWinnerTitles != 4 {
			t.Fatal("current title missing", p, err)
		}
	}
	p, err := m.Get(ctx, first, false)
	if err != nil || p.CurrentWeekWinner || p.WeekWinnerTitles != 4 {
		t.Fatal("old title persisted or historical wins lost", p, err)
	}
	if _, err := db.Exec(`UPDATE accounts SET deleted_at=now() WHERE id=$1`, next); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get(ctx, next, false); err == nil {
		t.Fatal("deleted winner public")
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn, token := os.Getenv("KNOWOFF_TEST_DSN"), os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, err := url.Parse(dsn)
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(token) || u == nil || u.Path != "/knowoff_test_"+token || (u.Hostname() != "postgres" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Scheme != "postgres" {
		t.Fatal("explicit disposable PostgreSQL test target required")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			t.Fatal("unexpected test database query")
		}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var actual string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&actual); err != nil || actual != "knowoff_test_"+token {
		db.Close()
		t.Fatal("disposable database identity mismatch", err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestEnsureAndGetProfile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	ctx := context.Background()

	id := uuid.NewString()
	nickname := "test-" + id[:8]
	if err := ensureTestProfile(ctx, db, id, nickname); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	p, err := m.Get(ctx, id, true)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.Nickname != nickname {
		t.Fatalf("nickname mismatch")
	}
	if p.Level != 1 {
		t.Fatalf("expected level 1, got %d", p.Level)
	}
}

func TestApplyMatchResult(t *testing.T) {
	db := setupTestDB(t)
	t.Cleanup(func() { db.Close() })
	m := NewManager(db)
	ctx := t.Context()
	cfg := profileValueConfig(t)
	cfg.Tuning.Progression = testProgression()
	owner, err := store.AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := owner.Release(cleanup); err != nil {
			t.Error(err)
		}
	})
	values, err := store.NewTextValueStore(db, cfg.Tuning).WithOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	recovery, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for {
		result, err := owner.RecoverLostOwners(recovery, values, 100)
		if err != nil {
			t.Fatal(err)
		}
		if result.Done {
			break
		}
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	accounts, admissions := make([]string, 4), make([]string, 4)
	for i := range accounts {
		accounts[i], admissions[i] = uuid.NewString(), uuid.NewString()
		if err := ensureTestProfile(ctx, db, accounts[i], "result-"+accounts[i][:8]); err != nil {
			t.Fatal(err)
		}
		if err := values.Reserve(ctx, store.TextReservation{ID: admissions[i], AccountID: accounts[i], EntryPath: "local", At: at}); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := cfg.Tuning.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	contract := v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(), ModeID: gamecontract.ModeMissedTheBriefing, OriginalSize: 4, RulesVersion: "text-v1", ContentLanguage: "en", PackReleaseID: "profile-storage-fixture", PackSHA256: strings.Repeat("a", 64), Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "local", Rewards: true}}
	record := store.TextMatchRecord{Contract: contract, Owner: owner.Token().IncarnationID, Epoch: 1, AdmissionIDs: admissions}
	if err := values.Prepare(ctx, record, at); err != nil {
		t.Fatal(err)
	}
	if err := values.Start(ctx, contract.MatchID, record.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	id := accounts[0]
	for ordinal := 1; ordinal <= 2; ordinal++ {
		award := store.TextAward{MatchID: contract.MatchID, Owner: record.Owner, Epoch: 1, AccountID: id, Kind: "correct_vote", Ordinal: ordinal, Amount: cfg.Tuning.Noin.CorrectVote, At: at.Add(time.Duration(ordinal) * time.Second)}
		for replay := 0; replay < 2; replay++ {
			if credited, err := values.Award(ctx, award); err != nil || credited != award.Amount {
				t.Fatal("correct vote receipt", credited, err)
			}
		}
	}
	out := store.TextOutcome{MatchID: contract.MatchID, Owner: record.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: at.Add(time.Minute)}
	for seat, account := range accounts {
		role := "nower"
		if seat == 3 {
			role = "donower"
		}
		player := store.TextPlayerResult{AccountID: account, Seat: seat, Role: role}
		if seat == 0 {
			player.Points = 45
			player.CorrectVotes = 2
			player.VotesCast = 2
			player.Pokes = 1
		}
		out.Players = append(out.Players, player)
	}
	var before *Profile
	var beforeReceipt profileReceipt
	for replay := 0; replay < 2; replay++ {
		if err := values.Finish(ctx, out); err != nil {
			t.Fatal(err)
		}
		if err := values.SettlePending(ctx, out.MatchID); err != nil {
			t.Fatal(err)
		}
		p, err := m.Get(ctx, id, true)
		if err != nil {
			t.Fatal(err)
		}
		if p.MatchesPlayed != 1 || p.MatchesWonNower != 1 || p.CorrectVotes != 2 || p.VotesCast != 2 || p.PokesSent != 1 || p.OverallPoints != 45 || p.NonConvertedPoints != 45 || p.XP != 40 || p.Level != 1 {
			t.Fatalf("settled profile invariant: %+v", p)
		}
		receipt := readProfileReceipt(t, db, id)
		if receipt.Deliveries != 1 || receipt.LedgerCount != 5 {
			t.Fatalf("settlement receipt invariant: %+v", receipt)
		}
		if replay == 0 {
			before = p
			beforeReceipt = receipt
		} else if !reflect.DeepEqual(before, p) || beforeReceipt != receipt {
			t.Fatal("terminal replay changed profile or value receipts")
		}
	}
	public, err := m.Get(ctx, id, false)
	if err != nil || public.NonConvertedPoints != 0 || public.OverallPoints != 45 {
		t.Fatal("public profile privacy", public, err)
	}
}

func TestValidateNickname(t *testing.T) {
	cases := []struct {
		name    string
		nick    string
		wantErr bool
	}{
		{"short", "a", true},
		{"ok", "PlayerOne", false},
		{"profane", "shithead", true},
		{"long", "averylongnicknamethatwontfit", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNickname(tc.nick)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestConvertPoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	ctx := t.Context()
	id := uuid.NewString()
	if err := ensureTestProfile(ctx, db, id, "convert-"+id[:8]); err != nil {
		t.Fatal(err)
	}
	// Retained convertible points are historical value; conversion must not
	// require fabricating a new match or rewriting their lifetime total.
	if _, err := db.ExecContext(ctx, `UPDATE profiles SET overall_points=250,non_converted_points=250 WHERE account_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	cfg := profileValueConfig(t)
	econ := economy.NewManager(db, cfg)
	noin, err := econ.ConvertPoints(ctx, id, 200)
	if err != nil || noin != 2 {
		t.Fatal("200 points must credit 2 Noin", noin, err)
	}
	p, err := m.Get(ctx, id, true)
	if err != nil || p.NonConvertedPoints != 50 || p.OverallPoints != 250 {
		t.Fatal("conversion changed lifetime or wrong balance", p, err)
	}
	receipt := readProfileReceipt(t, db, id)
	if receipt.Wallet != 2 || receipt.LedgerAmount != 2 || receipt.LedgerCount != 1 || receipt.DailyEarned != 2 {
		t.Fatalf("conversion accounting: %+v", receipt)
	}
	for _, points := range []int64{100, 51, 0, -100} {
		if _, err := econ.ConvertPoints(ctx, id, points); err == nil {
			t.Fatalf("invalid/insufficient %d accepted", points)
		}
		after, err := m.Get(ctx, id, true)
		if err != nil || !reflect.DeepEqual(p, after) || receipt != readProfileReceipt(t, db, id) {
			t.Fatal("failed conversion changed persisted values", err)
		}
	}
	capped := uuid.NewString()
	if err := ensureTestProfile(ctx, db, capped, "cap-"+capped[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE profiles SET overall_points=250,non_converted_points=250 WHERE account_id=$1`, capped); err != nil {
		t.Fatal(err)
	}
	if credited, err := econ.Wallet.Grant(ctx, capped, economy.LedgerCorrectVote, cfg.Tuning.Noin.DailyEarnCap, "profile cap fixture", int64(cfg.Tuning.Noin.DailyEarnCap)); err != nil || credited != cfg.Tuning.Noin.DailyEarnCap {
		t.Fatal(credited, err)
	}
	capBefore := readProfileReceipt(t, db, capped)
	capProfile, err := m.Get(ctx, capped, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := econ.ConvertPoints(ctx, capped, 100); err == nil {
		t.Fatal("conversion bypassed daily cap")
	}
	capAfter, err := m.Get(ctx, capped, true)
	if err != nil || !reflect.DeepEqual(capProfile, capAfter) || capBefore != readProfileReceipt(t, db, capped) {
		t.Fatal("capped conversion changed persisted values", err)
	}
}

func TestAvatarPresetSelectionRejectsUnapprovedReferencesAndFencesUploads(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	account := uuid.NewString()
	if err := ensureTestProfile(t.Context(), db, account, "avatar-"+account[:8]); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{"custom", "https://example.test/avatar", "../private", "unknown"} {
		if err := m.UpdateAvatar(t.Context(), account, candidate); err == nil {
			t.Errorf("unapproved reference accepted: %s", candidate)
		}
	}
	for i := 0; i < 2; i++ {
		if err := m.UpdateAvatar(t.Context(), account, "party"); err != nil {
			t.Fatal(err)
		}
	}
	var revision int64
	var selected string
	if err := db.QueryRow(`SELECT avatar,avatar_revision FROM accounts WHERE id=$1`, account).Scan(&selected, &revision); err != nil {
		t.Fatal(err)
	}
	if selected != "party" || revision != 2 {
		t.Fatalf("preset does not fence pending uploads: %s/%d", selected, revision)
	}
	if _, err := db.Exec(`UPDATE accounts SET banned_at=clock_timestamp() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateAvatar(t.Context(), account, "default"); err == nil {
		t.Fatal("banned account changed avatar")
	}
}
func TestProfileHistoricalCustomAvatarDoesNotBecomeDisplayable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	account := uuid.NewString()
	if err := ensureTestProfile(t.Context(), db, account, "legacy-"+account[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET avatar='custom' WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO custom_avatars(account_id,blob,content_type,moderated) VALUES($1,$2,'image/webp',true)`, account, []byte("legacy unproved blob")); err != nil {
		t.Fatal(err)
	}
	p, err := m.Get(t.Context(), account, true)
	if err != nil {
		t.Fatal(err)
	}
	if p.Avatar != "default" {
		t.Fatal("legacy blob became approved display")
	}
}

// ensureTestProfile is fixture-only account creation, never a runtime backfill.
func ensureTestProfile(ctx context.Context, db *sql.DB, id, nickname string) error {
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id,nickname) VALUES($1,$2)`, id, nickname); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO profiles(account_id) VALUES($1)`, id)
	return err
}
func profileValueConfig(t *testing.T) *config.Config {
	t.Helper()
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	if err := yaml.Unmarshal(raw, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	return cfg
}

type profileReceipt struct{ Wallet, LedgerAmount, LedgerCount, DailyEarned, Deliveries int64 }

func readProfileReceipt(t *testing.T, db *sql.DB, id string) profileReceipt {
	t.Helper()
	var result profileReceipt
	err := db.QueryRowContext(t.Context(), `SELECT COALESCE((SELECT balance FROM noin_wallets WHERE account_id=$1),0),(SELECT COALESCE(sum(amount),0) FROM noin_ledger WHERE account_id=$1),(SELECT count(*) FROM noin_ledger WHERE account_id=$1),(SELECT COALESCE(sum(earned),0) FROM daily_noin_earned WHERE account_id=$1),(SELECT count(*) FROM text_outbox WHERE account_id=$1)`, id).Scan(&result.Wallet, &result.LedgerAmount, &result.LedgerCount, &result.DailyEarned, &result.Deliveries)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProfileHasNoUnfencedLegacyWriters(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	retired := map[string]bool{"ApplyMatchResult": true, "ConvertPoints": true, "boolInt": true, "xpForMatch": true, "levelForXP": true, "AddContributorCredit": true, "AddWeekWinnerTitle": true, "EnsureProfile": true}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && retired[fn.Name.Name] {
					t.Errorf("retired unfenced profile writer/helper remains: %s", fn.Name.Name)
				}
			}
		}
	}
	if reflect.TypeOf(NewManager).NumIn() != 1 {
		t.Error("profile constructor retains obsolete progression writer dependency")
	}
}
