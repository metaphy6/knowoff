package privacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/google/uuid"
)

var ErrJournal = errors.New("privacy.journal_unavailable")

// MinimumStore must persist outside both the journal's replaceable state and
// application backups. Advance must be durable and monotonic before returning.
// Implementations must honor cancellation. A memory implementation proves only
// mechanics, not restart rollback resistance.
type MinimumStore interface {
	Load(context.Context) (SuppressionReceipt, error)
	Advance(context.Context, SuppressionReceipt) error
}

type FixtureJournalConfig struct {
	Directory             string
	RestorableRoots       []string
	InstallationID, KeyID string
	SigningKey            ed25519.PrivateKey
	SelectorKey           []byte
	Minimum               MinimumStore
}

// FixtureJournal is an unpublished local rehearsal adapter, not a configured
// production authority. Signing and selector keys never enter its directory.
type FixtureJournal struct {
	root        *os.Root
	cfg         FixtureJournalConfig
	now         func() time.Time
	afterRename func() error // private fault injection, never configured externally
}

type fixtureRecordBody struct {
	Domain            string   `json:"domain"`
	Installation      string   `json:"installation"`
	KeyID             string   `json:"key_id"`
	Sequence          int64    `json:"sequence"`
	Previous          [32]byte `json:"previous"`
	RequestID         string   `json:"request_id"`
	Selector          string   `json:"selector"`
	SelectorKeyDigest [32]byte `json:"selector_key_digest"`
	VerifiedAt        int64    `json:"verified_at_us"`
	Policy            string   `json:"policy"`
}
type FixtureRecord struct {
	Body      fixtureRecordBody `json:"body"`
	Signature []byte            `json:"signature"`
}
type fixtureState struct {
	Version      int             `json:"version"`
	Installation string          `json:"installation"`
	KeyID        string          `json:"key_id"`
	Records      []FixtureRecord `json:"records"`
}
type fixtureHead struct {
	Domain       string   `json:"domain"`
	Installation string   `json:"installation"`
	KeyID        string   `json:"key_id"`
	Challenge    []byte   `json:"challenge"`
	IssuedAt     int64    `json:"issued_at_us"`
	Sequence     int64    `json:"sequence"`
	Digest       [32]byte `json:"digest"`
}
type FixtureSnapshot struct {
	Records   []FixtureRecord `json:"records"`
	Head      fixtureHead     `json:"head"`
	Signature []byte          `json:"signature"`
}

const journalMaxBytes = 4 << 20
const journalMaxRecords = 4096

func canonical(v any) []byte { b, _ := json.Marshal(v); return b }
func selectorKeyDigest(key []byte) [32]byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte("knowoff:privacy:selector-key:v1"))
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}
func recordReceipt(r FixtureRecord) SuppressionReceipt {
	return SuppressionReceipt{r.Body.Sequence, sha256.Sum256(canonical(r.Body))}
}
func exactUUID(s string) bool {
	v, e := uuid.Parse(s)
	return e == nil && v != uuid.Nil && v.String() == s
}

func journalConfig(c FixtureJournalConfig) (FixtureJournalConfig, error) {
	if !exactUUID(c.InstallationID) || !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(c.KeyID) || len(c.SigningKey) != ed25519.PrivateKeySize || len(c.SelectorKey) != 32 || c.Minimum == nil || len(c.RestorableRoots) == 0 {
		return c, ErrJournal
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(c.SigningKey.Seed()), c.SigningKey) {
		return c, ErrJournal
	}
	path, e := privateLocation(c.Directory, c.RestorableRoots)
	if e != nil {
		return c, e
	}
	c.Directory = path
	c.SigningKey = bytes.Clone(c.SigningKey)
	c.SelectorKey = bytes.Clone(c.SelectorKey)
	return c, nil
}

func privateLocation(directory string, restorableRoots []string) (string, error) {
	if len(restorableRoots) == 0 {
		return "", ErrJournal
	}
	path, e := filepath.Abs(directory)
	if e != nil || path == string(filepath.Separator) {
		return "", ErrJournal
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return "", ErrJournal
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", ErrJournal
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	for _, backup := range restorableRoots {
		b, e := filepath.Abs(backup)
		if e != nil {
			return "", ErrJournal
		}
		// Resolve existing ancestors to reject aliases into the journal directory.
		for current := b; ; current = filepath.Dir(current) {
			if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink != 0 {
				return "", ErrJournal
			}
			if current == filepath.Dir(current) {
				break
			}
		}
		for _, pair := range [][2]string{{path, b}, {b, path}} {
			rel, e := filepath.Rel(pair[0], pair[1])
			if e != nil || rel == "." || rel != ".." && !bytes.HasPrefix([]byte(rel), []byte(".."+string(filepath.Separator))) {
				return "", ErrJournal
			}
		}
	}
	return path, nil
}

func CreateFixtureJournal(c FixtureJournalConfig) (*FixtureJournal, error) {
	c, e := journalConfig(c)
	if e != nil {
		return nil, e
	}
	if e = os.Mkdir(c.Directory, 0700); e != nil {
		return nil, ErrJournal
	}
	r, e := os.OpenRoot(c.Directory)
	if e != nil {
		return nil, ErrJournal
	}
	j := &FixtureJournal{root: r, cfg: c, now: time.Now}
	info, e := r.Stat(".")
	if e != nil || !privateOwned(info, true) {
		j.Close()
		return nil, ErrJournal
	}
	lock, e := r.OpenFile("lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		j.Close()
		return nil, ErrJournal
	}
	e = lock.Sync()
	lock.Close()
	if e != nil {
		j.Close()
		return nil, ErrJournal
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	minimum, e := c.Minimum.Load(ctx)
	if e != nil || minimum != (SuppressionReceipt{}) {
		j.Close()
		return nil, ErrJournal
	}
	if e = j.write(fixtureState{1, c.InstallationID, c.KeyID, []FixtureRecord{}}); e != nil {
		j.Close()
		return nil, e
	}
	parent, e := os.Open(filepath.Dir(c.Directory))
	if e != nil {
		j.Close()
		return nil, ErrJournal
	}
	e = parent.Sync()
	parent.Close()
	if e != nil {
		j.Close()
		return nil, ErrJournal
	}
	return j, nil
}
func OpenFixtureJournal(c FixtureJournalConfig) (*FixtureJournal, error) {
	c, e := journalConfig(c)
	if e != nil {
		return nil, e
	}
	r, e := os.OpenRoot(c.Directory)
	if e != nil {
		return nil, ErrJournal
	}
	j := &FixtureJournal{root: r, cfg: c, now: time.Now}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e = j.locked(ctx, func() error { _, e := j.read(ctx); return e })
	if e != nil {
		j.Close()
		return nil, e
	}
	return j, nil
}
func (j *FixtureJournal) Close() error { return j.root.Close() }

func (j *FixtureJournal) locked(ctx context.Context, fn func() error) error {
	return withPrivateLock(ctx, j.root, fn)
}
func withPrivateLock(ctx context.Context, root *os.Root, fn func() error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	infoRoot, e := root.Stat(".")
	if e != nil || !privateOwned(infoRoot, true) {
		return ErrJournal
	}
	info, e := root.Lstat("lock")
	if e != nil || !privateOwned(info, false) {
		return ErrJournal
	}
	f, e := root.OpenFile("lock", os.O_RDWR, 0)
	if e != nil {
		return ErrJournal
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(info, actual) {
		return ErrJournal
	}
	if e = lockJournal(ctx, f); e != nil {
		return e
	}
	defer unlockJournal(f)
	return fn()
}
func (j *FixtureJournal) read(ctx context.Context) (fixtureState, error) {
	var state fixtureState
	info, e := j.root.Lstat("state.json")
	if e != nil || !privateOwned(info, false) || info.Size() > journalMaxBytes {
		return state, ErrJournal
	}
	f, e := j.root.Open("state.json")
	if e != nil {
		return state, ErrJournal
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(info, actual) {
		return state, ErrJournal
	}
	b, e := io.ReadAll(io.LimitReader(f, journalMaxBytes+1))
	if e != nil || len(b) > journalMaxBytes || json.Unmarshal(b, &state) != nil || !bytes.Equal(b, canonical(state)) {
		return state, ErrJournal
	}
	if state.Version != 1 || state.Installation != j.cfg.InstallationID || state.KeyID != j.cfg.KeyID {
		return state, ErrJournal
	}
	for _, record := range state.Records {
		if record.Body.SelectorKeyDigest != selectorKeyDigest(j.cfg.SelectorKey) {
			return state, ErrJournal
		}
	}
	minimum, e := j.cfg.Minimum.Load(ctx)
	if e != nil {
		return state, ErrJournal
	}
	_, e = verifyRecords(state.Records, j.cfg.SigningKey.Public().(ed25519.PublicKey), j.cfg.InstallationID, j.cfg.KeyID, minimum)
	return state, e
}
func verifyRecords(records []FixtureRecord, key ed25519.PublicKey, installation, keyID string, minimum SuppressionReceipt) (SuppressionReceipt, error) {
	var last SuppressionReceipt
	if len(key) != ed25519.PublicKeySize || len(records) > journalMaxRecords || minimum.Sequence < 0 || minimum.Sequence > int64(len(records)) || minimum.Sequence == 0 && minimum.Digest != ([32]byte{}) {
		return last, ErrJournal
	}
	seen := map[string]bool{}
	for _, r := range records {
		b := r.Body
		selector, e := hex.DecodeString(b.Selector)
		if b.Domain != "knowoff:privacy:record:v1" || b.Installation != installation || b.KeyID != keyID || b.Sequence != last.Sequence+1 || b.Previous != last.Digest || !exactUUID(b.RequestID) || seen[b.RequestID] || e != nil || len(selector) != 32 || hex.EncodeToString(selector) != b.Selector || b.VerifiedAt <= 0 || b.Policy != "deletion-2026-09-19" || !ed25519.Verify(key, canonical(b), r.Signature) {
			return last, ErrJournal
		}
		if b.SelectorKeyDigest == ([32]byte{}) || len(seen) > 0 && b.SelectorKeyDigest != records[0].Body.SelectorKeyDigest {
			return last, ErrJournal
		}
		seen[b.RequestID] = true
		last = recordReceipt(r)
		if last.Sequence == minimum.Sequence && last != minimum {
			return last, ErrJournal
		}
	}
	return last, nil
}

func (j *FixtureJournal) syncState() error { return syncPrivateState(j.root) }
func syncPrivateState(root *os.Root) error {
	f, e := root.Open("state.json")
	if e != nil {
		return ErrJournal
	}
	e = f.Sync()
	f.Close()
	if e != nil {
		return ErrJournal
	}
	d, e := root.Open(".")
	if e != nil {
		return ErrJournal
	}
	defer d.Close()
	if d.Sync() != nil {
		return ErrJournal
	}
	return nil
}
func (j *FixtureJournal) write(state fixtureState) error {
	return writePrivateState(j.root, canonical(state), j.afterRename)
}
func writePrivateState(root *os.Root, raw []byte, afterRename func() error) error {
	if len(raw) > journalMaxBytes {
		return ErrJournal
	}
	name := "pending-" + uuid.NewString()
	f, e := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return ErrJournal
	}
	defer root.Remove(name)
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return ErrJournal
	}
	if root.Rename(name, "state.json") != nil {
		return ErrJournal
	}
	if afterRename != nil {
		if e = afterRename(); e != nil {
			return ErrJournal
		}
	}
	return syncPrivateState(root)
}
func (j *FixtureJournal) Publish(ctx context.Context, request SuppressionRequest) (SuppressionReceipt, error) {
	var receipt SuppressionReceipt
	if !exactUUID(request.RequestID) || !exactUUID(request.AccountID) || request.PolicyVersion != "deletion-2026-09-19" || request.VerifiedAt.IsZero() || !request.VerifiedAt.Equal(request.VerifiedAt.Truncate(time.Microsecond)) || request.VerifiedAt.After(j.now()) || request.VerifiedAt.UnixMicro() <= 0 {
		return receipt, ErrJournal
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	mac := hmac.New(sha256.New, j.cfg.SelectorKey)
	mac.Write([]byte("knowoff:privacy:selector:v1\x00" + j.cfg.InstallationID + "\x00" + request.AccountID))
	body := fixtureRecordBody{Domain: "knowoff:privacy:record:v1", Installation: j.cfg.InstallationID, KeyID: j.cfg.KeyID, RequestID: request.RequestID, Selector: hex.EncodeToString(mac.Sum(nil)), SelectorKeyDigest: selectorKeyDigest(j.cfg.SelectorKey), VerifiedAt: request.VerifiedAt.UnixMicro(), Policy: request.PolicyVersion}
	e := j.locked(ctx, func() error {
		state, e := j.read(ctx)
		if e != nil {
			return e
		}
		var head SuppressionReceipt
		if len(state.Records) > 0 {
			head = recordReceipt(state.Records[len(state.Records)-1])
		}
		for _, r := range state.Records {
			if r.Body.RequestID == request.RequestID {
				body.Sequence = r.Body.Sequence
				body.Previous = r.Body.Previous
				if body != r.Body {
					return ErrJournal
				}
				if e = j.syncState(); e != nil {
					return e
				}
				if e = j.cfg.Minimum.Advance(ctx, head); e != nil {
					return ErrJournal
				}
				receipt = recordReceipt(r)
				return nil
			}
		}
		if len(state.Records) >= journalMaxRecords {
			return ErrJournal
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		body.Sequence = head.Sequence + 1
		body.Previous = head.Digest
		r := FixtureRecord{body, ed25519.Sign(j.cfg.SigningKey, canonical(body))}
		state.Records = append(state.Records, r)
		if e = j.write(state); e != nil {
			return e
		}
		head = recordReceipt(r)
		if e = j.cfg.Minimum.Advance(ctx, head); e != nil {
			return ErrJournal
		}
		receipt = head
		return nil
	})
	return receipt, e
}
func (j *FixtureJournal) Snapshot(ctx context.Context, challenge []byte) (FixtureSnapshot, error) {
	var snapshot FixtureSnapshot
	if len(challenge) != 32 {
		return snapshot, ErrJournal
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	e := j.locked(ctx, func() error {
		state, e := j.read(ctx)
		if e != nil {
			return e
		}
		var head SuppressionReceipt
		if len(state.Records) > 0 {
			head = recordReceipt(state.Records[len(state.Records)-1])
		}
		if e = j.syncState(); e != nil {
			return e
		}
		if e = j.cfg.Minimum.Advance(ctx, head); e != nil {
			return ErrJournal
		}
		snapshot.Records = state.Records
		snapshot.Head = fixtureHead{"knowoff:privacy:head:v1", j.cfg.InstallationID, j.cfg.KeyID, bytes.Clone(challenge), j.now().UnixMicro(), head.Sequence, head.Digest}
		snapshot.Signature = ed25519.Sign(j.cfg.SigningKey, canonical(snapshot.Head))
		return nil
	})
	return snapshot, e
}
func VerifyFixtureSnapshot(snapshot FixtureSnapshot, key ed25519.PublicKey, installation, keyID string, challenge []byte, minimum SuppressionReceipt, at time.Time) error {
	h := snapshot.Head
	if len(key) != ed25519.PublicKeySize || len(challenge) != 32 || !bytes.Equal(challenge, h.Challenge) || !exactUUID(installation) || h.Domain != "knowoff:privacy:head:v1" || h.Installation != installation || h.KeyID != keyID || h.IssuedAt <= 0 || at.IsZero() || h.IssuedAt > at.UnixMicro() || at.UnixMicro()-h.IssuedAt > int64(time.Minute/time.Microsecond) || !ed25519.Verify(key, canonical(h), snapshot.Signature) {
		return ErrJournal
	}
	for _, r := range snapshot.Records {
		if r.Body.VerifiedAt > h.IssuedAt {
			return ErrJournal
		}
	}
	head, e := verifyRecords(snapshot.Records, key, installation, keyID, minimum)
	if e != nil || head.Sequence != h.Sequence || head.Digest != h.Digest {
		return ErrJournal
	}
	return nil
}
