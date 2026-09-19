package privacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// FixtureMinimum is an independent local file authority for rehearsals. Its
// directory must be excluded from both application and journal restore sets.
// This models persistence; production isolation/rollback resistance still needs
// a separately configured and verified storage authority.
type FixtureMinimum struct {
	root        *os.Root
	cfg         FixtureMinimumConfig
	afterRename func() error
}
type FixtureMinimumConfig struct {
	Directory, JournalDirectory string
	RestorableRoots             []string
	InstallationID, KeyID       string
	SigningKey                  ed25519.PrivateKey
}
type minimumBody struct {
	Domain       string             `json:"domain"`
	Installation string             `json:"installation"`
	KeyID        string             `json:"key_id"`
	Receipt      SuppressionReceipt `json:"receipt"`
}
type minimumState struct {
	Body      minimumBody `json:"body"`
	Signature []byte      `json:"signature"`
}

func minimumConfig(c FixtureMinimumConfig) (FixtureMinimumConfig, error) {
	if c.JournalDirectory == "" || len(c.RestorableRoots) == 0 || !exactUUID(c.InstallationID) || !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(c.KeyID) || len(c.SigningKey) != ed25519.PrivateKeySize {
		return c, ErrJournal
	}
	if !bytes.Equal(ed25519.NewKeyFromSeed(c.SigningKey.Seed()), c.SigningKey) {
		return c, ErrJournal
	}
	excluded := append(append([]string{}, c.RestorableRoots...), c.JournalDirectory)
	path, e := privateLocation(c.Directory, excluded)
	if e != nil {
		return c, e
	}
	c.Directory = path
	c.SigningKey = bytes.Clone(c.SigningKey)
	return c, nil
}
func CreateFixtureMinimum(c FixtureMinimumConfig) (*FixtureMinimum, error) {
	c, e := minimumConfig(c)
	if e != nil {
		return nil, e
	}
	if os.Mkdir(c.Directory, 0700) != nil {
		return nil, ErrJournal
	}
	r, e := os.OpenRoot(c.Directory)
	if e != nil {
		return nil, ErrJournal
	}
	m := &FixtureMinimum{root: r, cfg: c}
	info, e := r.Stat(".")
	if e != nil || !privateOwned(info, true) {
		m.Close()
		return nil, ErrJournal
	}
	f, e := r.OpenFile("lock", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		m.Close()
		return nil, ErrJournal
	}
	e = f.Sync()
	f.Close()
	if e != nil {
		m.Close()
		return nil, ErrJournal
	}
	if e = m.write(SuppressionReceipt{}); e != nil {
		m.Close()
		return nil, e
	}
	parent, e := os.Open(filepath.Dir(c.Directory))
	if e != nil {
		m.Close()
		return nil, ErrJournal
	}
	e = parent.Sync()
	parent.Close()
	if e != nil {
		m.Close()
		return nil, ErrJournal
	}
	return m, nil
}
func OpenFixtureMinimum(c FixtureMinimumConfig) (*FixtureMinimum, error) {
	c, e := minimumConfig(c)
	if e != nil {
		return nil, e
	}
	r, e := os.OpenRoot(c.Directory)
	if e != nil {
		return nil, ErrJournal
	}
	m := &FixtureMinimum{root: r, cfg: c}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, e = m.Load(ctx); e != nil {
		m.Close()
		return nil, e
	}
	return m, nil
}
func (m *FixtureMinimum) Close() error { return m.root.Close() }
func (m *FixtureMinimum) read() (SuppressionReceipt, error) {
	var state minimumState
	info, e := m.root.Lstat("state.json")
	if e != nil || !privateOwned(info, false) || info.Size() > 8192 {
		return SuppressionReceipt{}, ErrJournal
	}
	f, e := m.root.Open("state.json")
	if e != nil {
		return SuppressionReceipt{}, ErrJournal
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !os.SameFile(info, actual) {
		return SuppressionReceipt{}, ErrJournal
	}
	raw, e := io.ReadAll(io.LimitReader(f, 8193))
	if e != nil || len(raw) > 8192 || json.Unmarshal(raw, &state) != nil || !bytes.Equal(raw, canonical(state)) {
		return SuppressionReceipt{}, ErrJournal
	}
	b := state.Body
	if b.Domain != "knowoff:privacy:minimum:v1" || b.Installation != m.cfg.InstallationID || b.KeyID != m.cfg.KeyID || !validMinimum(b.Receipt) || !ed25519.Verify(m.cfg.SigningKey.Public().(ed25519.PublicKey), canonical(b), state.Signature) {
		return SuppressionReceipt{}, ErrJournal
	}
	return b.Receipt, nil
}
func validMinimum(r SuppressionReceipt) bool {
	return r.Sequence >= 0 && r.Sequence <= journalMaxRecords && ((r.Sequence == 0) == (r.Digest == ([32]byte{})))
}
func (m *FixtureMinimum) write(r SuppressionReceipt) error {
	body := minimumBody{"knowoff:privacy:minimum:v1", m.cfg.InstallationID, m.cfg.KeyID, r}
	return writePrivateState(m.root, canonical(minimumState{body, ed25519.Sign(m.cfg.SigningKey, canonical(body))}), m.afterRename)
}
func (m *FixtureMinimum) Load(ctx context.Context) (SuppressionReceipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result SuppressionReceipt
	e := withPrivateLock(ctx, m.root, func() error { var e error; result, e = m.read(); return e })
	return result, e
}
func (m *FixtureMinimum) Advance(ctx context.Context, r SuppressionReceipt) error {
	if !validMinimum(r) {
		return ErrJournal
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return withPrivateLock(ctx, m.root, func() error {
		current, e := m.read()
		if e != nil {
			return e
		}
		if r.Sequence < current.Sequence || r.Sequence == current.Sequence && r.Digest != current.Digest {
			return ErrJournal
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if r == current {
			return syncPrivateState(m.root)
		}
		return m.write(r)
	})
}
