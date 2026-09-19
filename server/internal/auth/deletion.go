package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/privacy"
	"github.com/lib/pq"
	"golang.org/x/oauth2"
)

var (
	ErrDeletionRateLimited      = errors.New("deletion.rate_limited")
	ErrDeletionAuthority        = errors.New("deletion.authority_unavailable")
	ErrDeletionRecoveryRequired = errors.New("deletion.recovery_required")
)

const DeletionPolicyVersion = "deletion-2026-09-19"

// DeletionManager uses a separately provisioned executor pool. It deliberately
// has no gameplay session issuance or automatic HTTP registration path.
type DeletionManager struct {
	auth      *Manager
	executor  *sql.DB
	publisher privacy.Publisher
	intentTTL time.Duration
}

func NewDeletionManager(auth *Manager, executor *sql.DB, publisher privacy.Publisher, intentTTL time.Duration) (*DeletionManager, error) {
	if auth == nil || executor == nil || intentTTL <= 0 || intentTTL > 10*time.Minute {
		return nil, ErrDeletionAuthority
	}
	return &DeletionManager{auth: auth, executor: executor, publisher: publisher, intentTTL: intentTTL}, nil
}

type DeletionIntent struct {
	ID        string    `json:"intent_id"`
	Secret    string    `json:"intent_secret"`
	ExpiresAt time.Time `json:"expires_at"`
	URL       string    `json:"authorization_url,omitempty"`
}
type DeletionConfirmCommand struct {
	IntentID, IntentSecret         string
	CapabilityID, CapabilitySecret string
	AccountID                      string
	RequestID, StatusSecret        string
}
type DeletionConfirmation struct {
	RequestID string `json:"request_id"`
	Phase     string `json:"phase"`
}

func deletionID(value string) bool {
	parsed, err := uuid.Parse(value)
	return err == nil && parsed != uuid.Nil && parsed.String() == value
}
func deletionSecret(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, ErrDeletionAuthority
	}
	digest := sha256.Sum256(decoded)
	return digest[:], nil
}
func deletionSQLError(err error) error {
	if err == nil {
		return nil
	}
	var pg *pq.Error
	if errors.As(err, &pg) && pg.Code == "22023" {
		if pg.Message == "deletion rate limited" {
			return ErrDeletionRateLimited
		}
		if pg.Message == "deletion recovery required" {
			return ErrDeletionRecoveryRequired
		}
		return ErrDeletionAuthority
	}
	return err
}

// Intent challenges have an engineering ceiling of ten minutes.
func (m *DeletionManager) newIntent() (*DeletionIntent, []byte, error) {
	secret, err := randomCodeVerifier()
	if err != nil {
		return nil, nil, err
	}
	digest, err := deletionSecret(secret)
	if err != nil {
		return nil, nil, err
	}
	return &DeletionIntent{ID: uuid.NewString(), Secret: secret, ExpiresAt: time.Now().UTC().Add(m.intentTTL)}, digest, nil
}
func (m *DeletionManager) BeginEnrollment(ctx context.Context, accessToken string) (*DeletionIntent, error) {
	if _, err := m.auth.ValidateAccessBinding(ctx, accessToken); err != nil {
		return nil, ErrDeletionAuthority
	}
	claims, err := m.auth.parseToken(accessToken, TokenAccess)
	if err != nil || claims.Purpose != "player" {
		return nil, ErrDeletionAuthority
	}
	intent, digest, err := m.newIntent()
	if err != nil {
		return nil, err
	}
	if claims.ExpiresAt.Time.Before(intent.ExpiresAt) {
		intent.ExpiresAt = claims.ExpiresAt.Time
	}
	var id string
	if _, err = m.executor.ExecContext(ctx, `SELECT public.privacy_expire_deletion_intents(100)`); err != nil {
		return nil, deletionSQLError(err)
	}
	err = m.executor.QueryRowContext(ctx, `SELECT public.privacy_begin_enrollment($1,$2,$3,$4,$5,$6,$7,$8)`, intent.ID, claims.AccountID, digest, intent.ExpiresAt, claims.SessionEpoch, claims.ID, claims.ExpiresAt.Time, claims.DeviceHash).Scan(&id)
	if err != nil {
		return nil, deletionSQLError(err)
	}
	return intent, nil
}

// The caller generates and securely persists the capability before this call;
// retries never require the server to retain/replay a plaintext secret.
func (m *DeletionManager) EnrollCapability(ctx context.Context, intentID, intentSecret, capabilityID, capabilitySecret string) error {
	nonce, err := deletionSecret(intentSecret)
	if err != nil {
		return err
	}
	capHash, err := deletionSecret(capabilitySecret)
	if err != nil {
		return err
	}
	if !deletionID(intentID) || !deletionID(capabilityID) {
		return ErrDeletionAuthority
	}
	var id string
	err = m.executor.QueryRowContext(ctx, `SELECT public.privacy_enroll_capability($1,$2,$3,$4)`, intentID, nonce, capabilityID, capHash).Scan(&id)
	return deletionSQLError(err)
}
func (m *DeletionManager) BeginCapabilityIntent(ctx context.Context, capabilityID, capabilitySecret string) (*DeletionIntent, error) {
	capHash, err := deletionSecret(capabilitySecret)
	if err != nil || !deletionID(capabilityID) {
		return nil, ErrDeletionRecoveryRequired
	}
	intent, digest, err := m.newIntent()
	if err != nil {
		return nil, err
	}
	var id string
	if _, err = m.executor.ExecContext(ctx, `SELECT public.privacy_expire_deletion_intents(100)`); err != nil {
		return nil, deletionSQLError(err)
	}
	err = m.executor.QueryRowContext(ctx, `SELECT public.privacy_begin_deletion_intent($1,$2,$3,$4,$5)`, intent.ID, capabilityID, capHash, digest, intent.ExpiresAt).Scan(&id)
	if err != nil {
		return nil, deletionSQLError(err)
	}
	return intent, nil
}
func (m *DeletionManager) BeginProviderIntent(ctx context.Context, provider, expectedAccount string) (*DeletionIntent, error) {
	if expectedAccount != "" && !deletionID(expectedAccount) {
		return nil, ErrDeletionAuthority
	}
	cfg := m.auth.oauthCfgs[provider]
	if cfg == nil {
		return nil, ErrOAuthUnavailable
	}
	intent, digest, err := m.newIntent()
	if err != nil {
		return nil, err
	}
	state, err := randomCodeVerifier()
	if err != nil {
		return nil, err
	}
	nonce, err := randomCodeVerifier()
	if err != nil {
		return nil, err
	}
	verifier, err := randomCodeVerifier()
	if err != nil {
		return nil, err
	}
	stateHash := sha256.Sum256([]byte("delete_" + state))
	var id string
	var expected any
	if expectedAccount != "" {
		expected = expectedAccount
	}
	if _, err = m.executor.ExecContext(ctx, `SELECT public.privacy_expire_deletion_intents(100)`); err != nil {
		return nil, deletionSQLError(err)
	}
	err = m.executor.QueryRowContext(ctx, `SELECT public.privacy_begin_deletion_oauth($1,$2,$3,$4,$5,$6,$7,$8)`, intent.ID, digest, intent.ExpiresAt, provider, stateHash[:], verifier, codeChallenge(nonce), expected).Scan(&id)
	if err != nil {
		return nil, deletionSQLError(err)
	}
	options := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("prompt", "select_account")}
	if provider == "google" {
		options = append(options, oauth2.SetAuthURLParam("nonce", nonce), oauth2.SetAuthURLParam("code_challenge", codeChallenge(verifier)), oauth2.SetAuthURLParam("code_challenge_method", "S256"))
	}
	intent.URL = cfg.AuthCodeURL("delete_"+state, options...)
	return intent, nil
}

// CompleteProviderIntent converts a fresh verified provider proof into the
// existing bounded deletion intent. Its expiry and independent security epoch
// govern that reauthentication lease; no gameplay session is issued.
func (m *DeletionManager) CompleteProviderIntent(ctx context.Context, provider, state, code string) error {
	if m.auth.oauthCfgs[provider] == nil || !strings.HasPrefix(state, "delete_") || len(state) != 50 || len(code) < 1 || len(code) > 4096 {
		return ErrDeletionAuthority
	}
	stateHash := sha256.Sum256([]byte(state))
	var raw []byte
	if err := m.executor.QueryRowContext(ctx, `SELECT public.privacy_claim_deletion_oauth($1,$2)`, stateHash[:], provider).Scan(&raw); err != nil {
		return deletionSQLError(err)
	}
	var claimed struct {
		ID        string `json:"intent_id"`
		Verifier  string `json:"verifier"`
		NonceHash string `json:"nonce_hash"`
	}
	if err := json.Unmarshal(raw, &claimed); err != nil {
		return fmt.Errorf("decode private deletion claim: %w", err)
	}
	subject, _, err := m.auth.verifyOAuthCode(ctx, provider, code, claimed.Verifier, claimed.NonceHash)
	if err != nil {
		return ErrDeletionAuthority
	}
	_, err = m.executor.ExecContext(ctx, `SELECT public.privacy_complete_deletion_oauth($1,$2,$3)`, claimed.ID, provider, subject)
	return deletionSQLError(err)
}
func (m *DeletionManager) Confirm(ctx context.Context, c DeletionConfirmCommand) (*DeletionConfirmation, error) {
	if !deletionID(c.IntentID) || !deletionID(c.RequestID) || !deletionID(c.AccountID) {
		return nil, ErrDeletionAuthority
	}
	nonce, err := deletionSecret(c.IntentSecret)
	if err != nil {
		return nil, err
	}
	status, err := deletionSecret(c.StatusSecret)
	if err != nil {
		return nil, err
	}
	var capID, capHash any
	if c.CapabilityID != "" || c.CapabilitySecret != "" {
		if !deletionID(c.CapabilityID) {
			return nil, ErrDeletionAuthority
		}
		digest, err := deletionSecret(c.CapabilitySecret)
		if err != nil {
			return nil, err
		}
		capID, capHash = c.CapabilityID, digest
	}
	var raw []byte
	if err = m.executor.QueryRowContext(ctx, `SELECT public.privacy_confirm_deletion($1,$2,$3,$4,$5,$6,$7)`, c.IntentID, nonce, capID, capHash, c.RequestID, status, c.AccountID).Scan(&raw); err != nil {
		return nil, deletionSQLError(err)
	}
	var job struct {
		RequestID     string    `json:"request_id"`
		AccountID     string    `json:"account_id"`
		VerifiedAt    time.Time `json:"verified_at"`
		PolicyVersion string    `json:"policy_version"`
	}
	if err = json.Unmarshal(raw, &job); err != nil {
		return nil, fmt.Errorf("decode deletion request: %w", err)
	}
	result := &DeletionConfirmation{RequestID: job.RequestID, Phase: "prepared"}
	// The transaction has committed. Publication never runs under an account lock.
	// Uncertainty leaves a real prepared request, rather than claiming completion.
	if m.publisher == nil {
		return result, nil
	}
	receipt, err := m.publisher.Publish(ctx, privacy.SuppressionRequest{RequestID: job.RequestID, AccountID: job.AccountID, VerifiedAt: job.VerifiedAt, PolicyVersion: job.PolicyVersion})
	if err != nil || receipt.Sequence <= 0 || receipt.Digest == [32]byte{} {
		return result, nil
	}
	if _, err = m.executor.ExecContext(ctx, `SELECT public.privacy_bind_suppression($1,$2,$3)`, job.RequestID, receipt.Sequence, receipt.Digest[:]); err != nil {
		return result, nil
	}
	result.Phase = "suppression_bound"
	return result, nil
}
func (m *DeletionManager) Status(ctx context.Context, statusSecret string) (json.RawMessage, error) {
	digest, err := deletionSecret(statusSecret)
	if err != nil {
		return nil, ErrDeletionAuthority
	}
	var raw []byte
	if err = m.auth.db.QueryRowContext(ctx, `SELECT public.account_deletion_status($1)`, digest).Scan(&raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, ErrDeletionAuthority
	}
	return raw, nil
}

// SecurityRevoke is an internal security operation; it is not a public deletion
// route and never reuses the gameplay-only session revocation operation.
func (m *DeletionManager) SecurityRevoke(ctx context.Context, account string) error {
	if !deletionID(account) {
		return ErrDeletionAuthority
	}
	_, err := m.executor.ExecContext(ctx, `SELECT public.privacy_security_revoke_deletion($1)`, account)
	return deletionSQLError(err)
}

func (m *DeletionManager) IntentStatus(ctx context.Context, intentID, intentSecret string) (json.RawMessage, error) {
	if !deletionID(intentID) {
		return nil, ErrDeletionAuthority
	}
	digest, err := deletionSecret(intentSecret)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if err = m.executor.QueryRowContext(ctx, `SELECT public.privacy_deletion_intent_status($1,$2)`, intentID, digest).Scan(&raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, ErrDeletionAuthority
	}
	return raw, nil
}
