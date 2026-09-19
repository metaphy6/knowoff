package privacy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var ErrInstallationAuthority = errors.New("installation privacy authority unavailable")

// InstallationKeyring is trusted injected key material, never an HTTP input.
// Keys returns the current key and all versions needed by live matching evidence.
// The database independently refuses omitted required versions. No production
// provider or automatic registration is enabled by this interface.
type InstallationKeyring interface {
	// Keys returns a local immutable snapshot. Remote key retrieval belongs to
	// startup/rotation, never an account or installation authority transaction.
	Keys(context.Context) (installationID string, keys map[string][]byte, err error)
}

type installationKeySnapshot struct {
	installation string
	keys         map[string][]byte
}

func (s installationKeySnapshot) Keys(context.Context) (string, map[string][]byte, error) {
	return s.installation, s.keys, nil
}

type InstallationAuthority struct {
	executor *sql.DB
	keyring  InstallationKeyring
}

func NewInstallationAuthority(executor *sql.DB, keyring InstallationKeyring) (*InstallationAuthority, error) {
	if executor == nil || keyring == nil {
		return nil, ErrInstallationAuthority
	}
	return &InstallationAuthority{executor: executor, keyring: keyring}, nil
}

type installationTags struct {
	ids                   []string
	sanctions, bootstraps pq.ByteaArray
}

func installationSelectors(ctx context.Context, keyring InstallationKeyring, raw string) (installationTags, error) {
	var result installationTags
	if raw == "" || len(raw) > 256 || !utf8.ValidString(raw) || strings.ContainsAny(raw, "\x00\r\n\t ") {
		return result, ErrInstallationAuthority
	}
	for _, r := range raw {
		if r <= 32 || r == 127 {
			return result, ErrInstallationAuthority
		}
	}
	installation, keys, err := keyring.Keys(ctx)
	if err != nil || !exactUUID(installation) || len(keys) < 1 || len(keys) > 4 {
		return result, ErrInstallationAuthority
	}
	for id, key := range keys {
		if id == "" || len(id) > 128 || !utf8.ValidString(id) || strings.ContainsRune(id, 0) || len(key) < 32 || len(key) > 64 {
			return result, ErrInstallationAuthority
		}
		result.ids = append(result.ids, id)
	}
	sort.Strings(result.ids)
	for _, id := range result.ids {
		derive := func(purpose string) []byte {
			mac := hmac.New(sha256.New, keys[id])
			mac.Write([]byte("knowoff:installation-selector:v1"))
			for _, s := range []string{installation, purpose, raw} {
				var n [4]byte
				binary.BigEndian.PutUint32(n[:], uint32(len(s)))
				mac.Write(n[:])
				mac.Write([]byte(s))
			}
			return mac.Sum(nil)
		}
		result.sanctions = append(result.sanctions, derive("sanction"))
		result.bootstraps = append(result.bootstraps, derive("erased-bootstrap"))
	}
	return result, nil
}

func registerInstallation(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, raw string, tags installationTags) error {
	_, err := q.ExecContext(ctx, `SELECT public.privacy_register_installation_keys($1,$2,$3,$4)`, raw, pq.Array(tags.ids), tags.sanctions, tags.bootstraps)
	if err != nil {
		return ErrInstallationAuthority
	}
	return nil
}

// Register completes before any later account-authority transaction. Callers
// must never invoke it while already holding an installation row lock.
func (a *InstallationAuthority) Register(ctx context.Context, raw string) error {
	if a == nil || a.executor == nil || a.keyring == nil {
		return ErrInstallationAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tags, err := installationSelectors(ctx, a.keyring, raw)
	if err != nil {
		return err
	}
	return registerInstallation(ctx, a.executor, raw, tags)
}

type InstallationErasureResult struct {
	Processed int  `json:"processed"`
	Mutations int  `json:"mutations"`
	Complete  bool `json:"complete"`
}

// EraseBatch keeps source discovery, CPU-only HMAC derivation, registration and
// erasure in one short transaction. Source SQL locks account→request→global keys
// before installations; no remote provider operation is performed here.
func (a *InstallationAuthority) EraseBatch(ctx context.Context, request string, limit int) (InstallationErasureResult, error) {
	var result InstallationErasureResult
	if a == nil || a.executor == nil || a.keyring == nil || limit < 1 || limit > 128 {
		return result, ErrInstallationAuthority
	}
	id, err := uuid.Parse(request)
	if err != nil || id == uuid.Nil || id.String() != request {
		return result, ErrInstallationAuthority
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	installation, keyMaterial, err := a.keyring.Keys(ctx)
	if err != nil {
		return result, ErrInstallationAuthority
	}
	snapshot := installationKeySnapshot{installation: installation, keys: make(map[string][]byte, len(keyMaterial))}
	for id, key := range keyMaterial {
		snapshot.keys[id] = append([]byte(nil), key...)
	}
	tags, err := installationSelectors(ctx, snapshot, "keyring-validation")
	if err != nil {
		return result, err
	}
	tx, err := a.executor.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT public.privacy_installation_sources($1,$2)`, request, limit).Scan(&raw); err != nil {
		return result, err
	}
	var sources struct {
		Installations []string `json:"installations"`
	}
	if json.Unmarshal(raw, &sources) != nil || len(sources.Installations) > limit {
		return result, ErrInstallationAuthority
	}
	// Even a completed replay needs a valid keyring, never an unconfigured bypass.
	keys := strings.Join(tags.ids, "\x00")
	for _, hash := range sources.Installations {
		tags, err = installationSelectors(ctx, snapshot, hash)
		if err != nil || strings.Join(tags.ids, "\x00") != keys {
			return result, ErrInstallationAuthority
		}
		if err = registerInstallation(ctx, tx, hash, tags); err != nil {
			return result, err
		}
	}
	if err = tx.QueryRowContext(ctx, `SELECT public.privacy_erase_installations_batch($1,$2,$3)`, request, limit, pq.Array(tags.ids)).Scan(&raw); err != nil {
		return result, err
	}
	if json.Unmarshal(raw, &result) != nil || result.Processed < 0 || result.Processed > limit || result.Mutations < 0 || result.Mutations > 12*limit+1 {
		return result, ErrInstallationAuthority
	}
	if err = tx.Commit(); err != nil {
		return InstallationErasureResult{}, err
	}
	return result, nil
}
