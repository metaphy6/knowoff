// Package portal implements the public Contributor Portal: role applications,
// media submissions, Guard freezes, and the Weekly Nown Challenge.
package portal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/profile"
	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/lib/pq"
)

// Role identifies a portal role.
type Role string

const (
	RoleContributor Role = "contributor"
	RoleCurator     Role = "curator"
	RoleGuard       Role = "guard"
)

// Valid reports whether r is a known portal role.
func ValidRole(r string) bool {
	switch Role(r) {
	case RoleContributor, RoleCurator, RoleGuard:
		return true
	}
	return false
}

func roleRank(r Role) int {
	switch r {
	case RoleGuard:
		return 3
	case RoleCurator:
		return 2
	case RoleContributor:
		return 1
	}
	return 0
}

// highestRole picks the role with the greatest rank from a slice.
func highestRole(roles []Role) Role {
	var best Role
	bestRank := -1
	for _, r := range roles {
		if rk := roleRank(r); rk > bestRank {
			bestRank = rk
			best = r
		}
	}
	return best
}

func validApplicationStatus(s string) bool {
	switch ApplicationStatus(s) {
	case ApplicationPending, ApplicationApproved, ApplicationRejected:
		return true
	}
	return false
}

func validSubmissionStatus(s string) bool {
	switch SubmissionStatus(s) {
	case StatusDraft, StatusSubmitted, StatusInReview, StatusApproved, StatusRejected, StatusPublished:
		return true
	}
	return false
}

// SubmissionStatus is the workflow state of a media submission.
type SubmissionStatus string

const (
	StatusDraft     SubmissionStatus = "draft"
	StatusSubmitted SubmissionStatus = "submitted"
	StatusInReview  SubmissionStatus = "in_review"
	StatusApproved  SubmissionStatus = "approved"
	StatusRejected  SubmissionStatus = "rejected"
	StatusPublished SubmissionStatus = "published"
)

// MediaType is the kind of submitted media.
type MediaType string

const (
	MediaText  MediaType = "text"
	MediaImage MediaType = "image"
)

// ValidMediaType reports whether t is a known media type.
func ValidMediaType(t string) bool {
	switch MediaType(t) {
	case MediaText, MediaImage:
		return true
	}
	return false
}

// ApplicationStatus is the state of a role application.
type ApplicationStatus string

const (
	ApplicationPending  ApplicationStatus = "pending"
	ApplicationApproved ApplicationStatus = "approved"
	ApplicationRejected ApplicationStatus = "rejected"
)

// ChallengeEntryStatus is the screening state of a challenge entry.
type ChallengeEntryStatus string

const (
	EntrySubmitted ChallengeEntryStatus = "submitted"
	EntryScreening ChallengeEntryStatus = "screening"
	EntryApproved  ChallengeEntryStatus = "approved"
	EntryRejected  ChallengeEntryStatus = "rejected"
)

// RoleApplication is a player's request for a portal role.
type RoleApplication struct {
	ID        string
	AccountID string
	Role      Role
	Status    ApplicationStatus
	AppliedAt time.Time
	DecidedAt *time.Time
	DecidedBy *string
	Reason    string
}

// RoleGrant records an active or revoked portal role.
type RoleGrant struct {
	AccountID string
	Role      Role
	GrantedAt time.Time
	GrantedBy string
	RevokedAt *time.Time
}

// Submission is a media contribution.
type Submission struct {
	ID              string
	AccountID       string
	MediaType       MediaType
	Content         string
	AssetRef        string
	Status          SubmissionStatus
	Tags            []string
	ToneBucket      string
	NownID          *string
	PackTag         string
	TermsVersion    string
	TermsAcceptedAt time.Time
	SubmittedAt     *time.Time
	DecidedAt       *time.Time
	DecidedBy       *string
	RejectionReason string
}

// ChallengeTopic is the weekly Nown topic.
type ChallengeTopic struct {
	ID            string
	WeekStart     time.Time
	WeekEnd       time.Time
	NownMediaID   string
	PublishedAt   time.Time
	ClosedAt      *time.Time
	WinnerEntryID *string
}

// ChallengeEntry is a player's response to the weekly topic.
type ChallengeEntry struct {
	ID              string
	AccountID       string
	TopicID         string
	EntryType       MediaType
	Content         string
	AssetRef        string
	Status          ChallengeEntryStatus
	VoteCount       int
	SlotNumber      int
	RejectionReason string
}

// AuditLogger records reversible admin actions. It is implemented by the admin manager.
type AuditLogger interface {
	LogAction(ctx context.Context, adminID, action, entityType, entityID string, before, after map[string]any) error
}

// AuthClient validates bearer tokens and revokes sessions. Implemented by auth.Manager.
type AuthClient interface {
	ValidateAccessToken(ctx context.Context, token string) (string, error)
	RevokeAccount(ctx context.Context, accountID string) error
}

// Deps bundles the dependencies needed by the portal manager.
type Deps struct {
	DB       *sql.DB
	Config   *config.Config
	Auth     AuthClient
	Profile  *profile.Manager
	Economy  *economy.Manager
	Admin    AuditLogger
	Media    *media.Manager
	Screener TextScreener
}

// Manager is the portal service.
type Manager struct {
	db       *sql.DB
	cfg      *config.Config
	auth     AuthClient
	profile  *profile.Manager
	economy  *economy.Manager
	admin    AuditLogger
	media    *media.Manager
	screener TextScreener
}

// NewManager returns a portal manager.
func NewManager(deps Deps) *Manager {
	return &Manager{
		db:       deps.DB,
		cfg:      deps.Config,
		auth:     deps.Auth,
		profile:  deps.Profile,
		economy:  deps.Economy,
		admin:    deps.Admin,
		media:    deps.Media,
		screener: deps.Screener,
	}
}

// DB returns the underlying database handle for tests.
func (m *Manager) DB() *sql.DB { return m.db }

// serverDay returns the UTC date used for daily caps.
func serverDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ── Role applications and grants ───────────────────────────────────────────

// ApplyForRole records a player's application for a portal role.
func (m *Manager) ApplyForRole(ctx context.Context, accountID string, role Role) error {
	if !ValidRole(string(role)) {
		return fmt.Errorf("invalid role")
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}
	p, err := m.profile.Get(ctx, accountID, false)
	if err != nil {
		return fmt.Errorf("load profile: %w", err)
	}
	if p.Level < m.cfg.Tuning.Portal.MinAccountLevelToApply {
		return fmt.Errorf("account level %d below required %d", p.Level, m.cfg.Tuning.Portal.MinAccountLevelToApply)
	}
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO portal_role_applications (account_id, role, status, applied_at)
		 VALUES ($1, $2, 'pending', now())
		 ON CONFLICT (account_id, role) WHERE status = 'pending' DO NOTHING`,
		accountID, string(role),
	)
	if err != nil {
		return fmt.Errorf("insert application: %w", err)
	}
	return nil
}

// ListApplications returns pending (or all) role applications.
func (m *Manager) ListApplications(ctx context.Context, status string) ([]RoleApplication, error) {
	q := `SELECT id, account_id, role, status, applied_at, decided_at, decided_by, reason
	      FROM portal_role_applications`
	args := []any{}
	if status != "" {
		if !validApplicationStatus(status) {
			return nil, fmt.Errorf("invalid application status")
		}
		q += " WHERE status = $1"
		args = append(args, status)
	}
	q += " ORDER BY applied_at DESC"
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query applications: %w", err)
	}
	defer rows.Close()
	var out []RoleApplication
	for rows.Next() {
		var r RoleApplication
		var decidedBy sql.NullString
		var reason sql.NullString
		if err := rows.Scan(&r.ID, &r.AccountID, &r.Role, &r.Status, &r.AppliedAt, &r.DecidedAt, &decidedBy, &reason); err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}
		r.DecidedBy = nullableString(decidedBy)
		r.Reason = reason.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetApplication loads a role application by id.
func (m *Manager) GetApplication(ctx context.Context, id string) (*RoleApplication, error) {
	var r RoleApplication
	var decidedBy sql.NullString
	var reason sql.NullString
	err := m.db.QueryRowContext(ctx,
		`SELECT id, account_id, role, status, applied_at, decided_at, decided_by, reason
		 FROM portal_role_applications WHERE id = $1`, id,
	).Scan(&r.ID, &r.AccountID, &r.Role, &r.Status, &r.AppliedAt, &r.DecidedAt, &decidedBy, &reason)
	if err != nil {
		return nil, fmt.Errorf("load application: %w", err)
	}
	r.DecidedBy = nullableString(decidedBy)
	r.Reason = reason.String
	return &r, nil
}

// GrantRole gives a player a portal role. The adminID is recorded and audited.
// A Curator is not automatically granted Contributor; instead HasRole enforces
// the hierarchy (Curator satisfies Contributor checks).
func (m *Manager) GrantRole(ctx context.Context, adminID, accountID string, role Role) error {
	if !ValidRole(string(role)) {
		return fmt.Errorf("invalid role")
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var oldRole sql.NullString
	_ = tx.QueryRowContext(ctx,
		"SELECT role FROM portal_roles WHERE account_id = $1 AND role = $2 AND revoked_at IS NULL",
		accountID, string(role),
	).Scan(&oldRole)

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO portal_roles (account_id, role, granted_by, granted_at, created_at, updated_at)
		 VALUES ($1, $2, $3, now(), now(), now())
		 ON CONFLICT (account_id, role) DO UPDATE SET
		   granted_by = EXCLUDED.granted_by,
		   granted_at = EXCLUDED.granted_at,
		   revoked_at = NULL,
		   revoked_by = NULL,
		   updated_at = now()`,
		accountID, string(role), adminID,
	); err != nil {
		return fmt.Errorf("upsert role: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE portal_role_applications
		 SET status = 'approved', decided_at = now(), decided_by = $1, updated_at = now()
		 WHERE account_id = $2 AND role = $3 AND status = 'pending'`,
		adminID, accountID, string(role),
	); err != nil {
		return fmt.Errorf("approve application: %w", err)
	}

	before := map[string]any{}
	if oldRole.Valid {
		before["role"] = oldRole.String
	}
	if err := auditTx(ctx, tx, adminID, "portal_role_grant", "portal_role", accountID, before, map[string]any{"role": string(role)}); err != nil {
		return err
	}
	return tx.Commit()
}

// TermsVersion is a versioned contribution terms record.
type TermsVersion struct {
	Version    string
	Title      string
	Body       string
	ActiveFrom time.Time
	CreatedAt  time.Time
}

// ListTermsVersions returns all contribution terms versions, newest first.
func (m *Manager) ListTermsVersions(ctx context.Context) ([]TermsVersion, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT version, title, body, active_from, created_at FROM portal_terms ORDER BY active_from DESC, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query terms: %w", err)
	}
	defer rows.Close()
	var out []TermsVersion
	for rows.Next() {
		var t TermsVersion
		if err := rows.Scan(&t.Version, &t.Title, &t.Body, &t.ActiveFrom, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan terms: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CreateTermsVersion inserts a new contribution terms version. The version
// string is taken from config/admin input and must be unique.
func (m *Manager) CreateTermsVersion(ctx context.Context, adminID, version, title, body string, activeFrom time.Time) error {
	version = strings.TrimSpace(version)
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if version == "" || title == "" || body == "" || len(version) > 100 || len(title) > 200 || len(body) > 50000 {
		return fmt.Errorf("version, title and body required within their limits")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO portal_terms(version,title,body,active_from) VALUES($1,$2,$3,$4)`, version, title, body, activeFrom); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, adminID, "portal_terms_create", "portal_terms", version, map[string]any{}, map[string]any{"title": title, "body": body, "active_from": activeFrom}); err != nil {
		return err
	}
	return tx.Commit()
}

// RejectApplication marks a role application as rejected.
func (m *Manager) RejectApplication(ctx context.Context, adminID, applicationID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 2000 {
		return fmt.Errorf("rejection reason required, up to 2000 bytes")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE portal_role_applications SET status='rejected',decided_at=now(),decided_by=$2,reason=$3,updated_at=now() WHERE id=$1 AND status='pending'`, applicationID, adminID, reason)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("application not found or already decided")
	}
	if err = auditTx(ctx, tx, adminID, "portal_application_reject", "portal_role_application", applicationID, map[string]any{"status": "pending"}, map[string]any{"status": "rejected", "reason": reason}); err != nil {
		return err
	}
	return tx.Commit()
}

// RevokeRole revokes a specific portal role.
func (m *Manager) RevokeRole(ctx context.Context, adminID, accountID string, role Role) error {
	if !ValidRole(string(role)) {
		return fmt.Errorf("invalid role")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE portal_roles SET revoked_at=now(),revoked_by=$3,updated_at=now() WHERE account_id=$1 AND role=$2 AND revoked_at IS NULL`, accountID, string(role), adminID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return fmt.Errorf("no active role")
	}
	if err = auditTx(ctx, tx, adminID, "portal_role_revoke", "portal_role", accountID, map[string]any{"status": "active", "role": string(role)}, map[string]any{"status": "revoked", "role": string(role)}); err != nil {
		return err
	}
	return tx.Commit()
}

// HasRole reports whether an account holds the requested role or a higher one
// for content work (Curator includes Contributor). Guard is independent.
func (m *Manager) HasRole(ctx context.Context, accountID string, role Role) (bool, error) {
	if !ValidRole(string(role)) {
		return false, fmt.Errorf("invalid role")
	}
	rows, err := m.db.QueryContext(ctx,
		"SELECT role FROM portal_roles WHERE account_id = $1 AND revoked_at IS NULL",
		accountID,
	)
	if err != nil {
		return false, fmt.Errorf("check role: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return false, fmt.Errorf("scan role: %w", err)
		}
		if Role(r) == role || (role == RoleContributor && Role(r) == RoleCurator) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// ActiveRole returns the account's highest active portal role, if any.
func (m *Manager) ActiveRole(ctx context.Context, accountID string) (Role, error) {
	rows, err := m.db.QueryContext(ctx,
		"SELECT role FROM portal_roles WHERE account_id = $1 AND revoked_at IS NULL",
		accountID,
	)
	if err != nil {
		return "", fmt.Errorf("load roles: %w", err)
	}
	defer rows.Close()
	var roles []Role
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return "", fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, Role(r))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return highestRole(roles), nil
}

// ── Submission pipeline ────────────────────────────────────────────────────

// CreateDraft creates a draft submission.
func (m *Manager) CreateDraft(ctx context.Context, accountID string, mediaType MediaType, content string, consent ...ContributionConsent) (*Submission, error) {
	content = strings.TrimSpace(content)
	if mediaType != MediaText || content == "" || len(content) > m.MaxTextBytes() {
		return nil, fmt.Errorf("text content must contain 1 to %d bytes", m.MaxTextBytes())
	}
	if len(consent) != 1 || !consent[0].Accepted {
		return nil, fmt.Errorf("explicit contribution terms acceptance required")
	}
	has, err := m.HasRole(ctx, accountID, RoleContributor)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf("contributor role required")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var version string
	if err = tx.QueryRowContext(ctx, `SELECT version FROM portal_terms WHERE active_from<=now() ORDER BY active_from DESC,version DESC LIMIT 1 FOR SHARE`).Scan(&version); err != nil {
		return nil, err
	}
	if consent[0].Version != version {
		return nil, fmt.Errorf("contribution terms changed; reload and accept the current version")
	}
	id := uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO portal_submissions(id,account_id,media_type,content,status,tags,terms_version,terms_accepted_at) VALUES($1,$2,'text',$3,'draft','{}',$4,now())`, id, accountID, content, version)
	if err != nil {
		return nil, err
	}
	if err = auditTx(ctx, tx, "", "submission_draft_create", "submission", id, map[string]any{}, map[string]any{"actor_account_id": accountID, "status": "draft", "terms_version": version}); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return m.GetSubmission(ctx, id)
}

// EditDraft can change only the owner's unsubmitted text. Submitted revisions
// require a withdrawal and another counted submission.
func (m *Manager) EditDraft(ctx context.Context, accountID, id, content string) error {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > m.MaxTextBytes() {
		return fmt.Errorf("text content must contain 1 to %d bytes", m.MaxTextBytes())
	}
	has, err := m.HasRole(ctx, accountID, RoleContributor)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("contributor role required")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old string
	if err = tx.QueryRowContext(ctx, `SELECT content FROM portal_submissions WHERE id=$1 AND account_id=$2 AND status='draft' AND media_type='text' FOR UPDATE`, id, accountID).Scan(&old); err != nil {
		return fmt.Errorf("only your own draft can be edited")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE portal_submissions SET content=$2,updated_at=now() WHERE id=$1`, id, content); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, "", "submission_draft_edit", "submission", id, map[string]any{"content": old}, map[string]any{"actor_account_id": accountID, "content": content}); err != nil {
		return err
	}
	return tx.Commit()
}

// SubmitDraft moves a draft to the submitted queue. The account must hold the
// contributor role and must not have exceeded the daily cap.
func (m *Manager) SubmitDraft(ctx context.Context, accountID, submissionID string) error {
	has, err := m.HasRole(ctx, accountID, RoleContributor)
	if err != nil {
		return err
	}
	if !has {
		return fmt.Errorf("contributor role required")
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A stable account row exists before the first counter row. Lock it first so
	// two first submissions cannot both observe an absent daily counter.
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 AND banned_at IS NULL FOR UPDATE`, accountID).Scan(&locked); err != nil {
		return fmt.Errorf("account unavailable")
	}
	var owner, status, terms string
	if err = tx.QueryRowContext(ctx, `SELECT account_id,status,terms_version FROM portal_submissions WHERE id=$1 FOR UPDATE`, submissionID).Scan(&owner, &status, &terms); err != nil {
		return fmt.Errorf("submission unavailable")
	}
	if owner != accountID {
		return fmt.Errorf("not owner")
	}
	if status != "draft" {
		return fmt.Errorf("submission not draft")
	}
	var current string
	if err = tx.QueryRowContext(ctx, `SELECT version FROM portal_terms WHERE active_from<=now() ORDER BY active_from DESC,version DESC LIMIT 1`).Scan(&current); err != nil {
		return err
	}
	if terms != current {
		return fmt.Errorf("contribution terms changed; create a new draft with current consent")
	}
	day := serverDay(time.Now().UTC())
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT count FROM portal_submission_counts WHERE account_id=$1 AND server_day=$2`, accountID, day).Scan(&count); err != nil && err != sql.ErrNoRows {
		return err
	}
	cap := m.cfg.Tuning.Portal.SubmissionsPerContributorPerDay
	if cap <= 0 {
		cap = 10
	}
	if count >= cap {
		return fmt.Errorf("daily submission cap reached")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE portal_submissions SET status='submitted',submitted_at=now(),updated_at=now() WHERE id=$1`, submissionID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO portal_submission_counts(account_id,server_day,count) VALUES($1,$2,1) ON CONFLICT(account_id,server_day) DO UPDATE SET count=portal_submission_counts.count+1,updated_at=now()`, accountID, day); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, "", "submission_submit", "submission", submissionID, map[string]any{"status": "draft"}, map[string]any{"status": "submitted", "actor_account_id": accountID}); err != nil {
		return err
	}
	return tx.Commit()
}

// DecideSubmission approves or rejects a submission. Only curators/admins may decide.
func (m *Manager) DecideSubmission(ctx context.Context, adminID, submissionID string, approve bool, reason string, revision ...string) error {
	var screenedContent string
	if approve {
		pending, err := m.GetSubmission(ctx, submissionID)
		if err != nil {
			return err
		}
		if pending.Status != StatusSubmitted && pending.Status != StatusInReview {
			return fmt.Errorf("submission not in reviewable state")
		}
		if pending.MediaType != MediaText {
			return fmt.Errorf("only text screening is available")
		}
		if len(revision) > 0 && revision[0] != ContentRevision(pending.Content) {
			return fmt.Errorf("submission changed since review; reload and review again")
		}
		screenedContent = pending.Content
		if err = m.screenText(ctx, screenedContent); err != nil {
			return err
		}
	}

	if !approve && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("rejection reason required")
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var s Submission
	var assetRef sql.NullString
	var toneBucket sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT id, account_id, status, media_type, content, asset_ref, tags, tone_bucket
		 FROM portal_submissions WHERE id = $1 FOR UPDATE`,
		submissionID,
	).Scan(&s.ID, &s.AccountID, &s.Status, &s.MediaType, &s.Content, &assetRef, pq.Array(&s.Tags), &toneBucket); err != nil {
		return fmt.Errorf("load submission: %w", err)
	}
	s.AssetRef = assetRef.String
	s.ToneBucket = toneBucket.String
	if approve && s.Content != screenedContent {
		return fmt.Errorf("submission changed during screening; review again")
	}
	if s.Status != StatusSubmitted && s.Status != StatusInReview {
		return fmt.Errorf("submission not in reviewable state")
	}

	newStatus := StatusRejected
	if approve {
		newStatus = StatusApproved
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE portal_submissions
		 SET status = $1, decided_at = now(), decided_by = $2, rejection_reason = $3, updated_at = now()
		 WHERE id = $4`,
		string(newStatus), adminID, reason, submissionID,
	); err != nil {
		return fmt.Errorf("update submission: %w", err)
	}

	if approve {
		_, err := m.economy.Wallet.GrantTx(ctx, tx, s.AccountID, economy.LedgerContributorReward,
			m.cfg.Tuning.Noin.ContributorAcceptedAsset,
			fmt.Sprintf("accepted submission %s", submissionID),
			0, // The play-earn cap does not cap accepted community work.
		)
		if err != nil {
			return fmt.Errorf("grant contributor reward: %w", err)
		}
		if err := m.profile.AddContributorCreditTx(ctx, tx, s.AccountID, submissionID); err != nil {
			return fmt.Errorf("add contributor credit: %w", err)
		}
	}

	before := map[string]any{"status": string(s.Status)}
	after := map[string]any{"status": string(newStatus), "reason": reason}
	if err := auditTx(ctx, tx, adminID, "submission_decide", "submission", submissionID, before, after); err != nil {
		return err
	}
	return tx.Commit()
}

// PublishSubmission marks an approved submission as published in a pack version.
func (m *Manager) PublishSubmission(ctx context.Context, adminID, submissionID, packTag string) error {
	res, err := m.db.ExecContext(ctx,
		`UPDATE portal_submissions
		 SET status = 'published', pack_tag = $2, updated_at = now()
		 WHERE id = $1 AND status = 'approved'`,
		submissionID, packTag,
	)
	if err != nil {
		return fmt.Errorf("publish submission: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("submission not approved")
	}
	_ = m.admin.LogAction(ctx, adminID, "submission_publish", "submission", submissionID,
		map[string]any{"status": "approved"},
		map[string]any{"status": "published", "pack_tag": packTag},
	)
	return nil
}

// WithdrawSubmission allows a contributor to withdraw their submitted draft.
// The queue slot is consumed (per spec: withdraw + resubmit costs the slot).
func (m *Manager) WithdrawSubmission(ctx context.Context, accountID, submissionID string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old string
	if err = tx.QueryRowContext(ctx, `SELECT status FROM portal_submissions WHERE id=$1 AND account_id=$2 AND status IN ('submitted','in_review') FOR UPDATE`, submissionID, accountID).Scan(&old); err != nil {
		return fmt.Errorf("submission not withdrawable")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE portal_submissions SET status='draft',submitted_at=NULL,updated_at=now() WHERE id=$1`, submissionID); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, "", "submission_withdraw", "submission", submissionID, map[string]any{"status": old}, map[string]any{"status": "draft", "actor_account_id": accountID}); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSubmission loads a submission by id.
func (m *Manager) GetSubmission(ctx context.Context, id string) (*Submission, error) {
	var s Submission
	var assetRef sql.NullString
	var toneBucket sql.NullString
	var packTag sql.NullString
	var decidedAt sql.NullTime
	var decidedBy sql.NullString
	var rejection sql.NullString
	var submittedAt sql.NullTime
	err := m.db.QueryRowContext(ctx,
		`SELECT id, account_id, media_type, content, asset_ref, status, tags, tone_bucket,
		        nown_id, pack_tag, terms_version, terms_accepted_at, submitted_at,
		        decided_at, decided_by, rejection_reason
		 FROM portal_submissions WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.AccountID, &s.MediaType, &s.Content, &assetRef, &s.Status, pq.Array(&s.Tags),
		&toneBucket, &s.NownID, &packTag, &s.TermsVersion, &s.TermsAcceptedAt,
		&submittedAt, &decidedAt, &decidedBy, &rejection)
	if err != nil {
		return nil, fmt.Errorf("load submission: %w", err)
	}
	s.AssetRef = assetRef.String
	s.ToneBucket = toneBucket.String
	s.PackTag = packTag.String
	s.SubmittedAt = nullableTime(submittedAt)
	s.DecidedAt = nullableTime(decidedAt)
	s.DecidedBy = nullableString(decidedBy)
	s.RejectionReason = rejection.String
	return &s, nil
}

// ListSubmissions returns submissions for an account or by status.
func (m *Manager) ListSubmissions(ctx context.Context, accountID, status string) ([]Submission, error) {
	q := `SELECT id, account_id, media_type, content, asset_ref, status, tags, tone_bucket,
	             nown_id, pack_tag, terms_version, terms_accepted_at, submitted_at,
	             decided_at, decided_by, rejection_reason
	      FROM portal_submissions`
	args := []any{}
	i := 1
	if accountID != "" {
		q += fmt.Sprintf(" WHERE account_id = $%d", i)
		args = append(args, accountID)
		i++
	}
	if status != "" {
		if !validSubmissionStatus(status) {
			return nil, fmt.Errorf("invalid submission status")
		}
		if accountID != "" {
			q += fmt.Sprintf(" AND status = $%d", i)
		} else {
			q += fmt.Sprintf(" WHERE status = $%d", i)
		}
		args = append(args, status)
		i++
	}
	q += " ORDER BY created_at DESC"
	rows, err := m.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query submissions: %w", err)
	}
	defer rows.Close()
	return scanSubmissions(rows)
}

// ── Guard freezes ──────────────────────────────────────────────────────────

// FreezeAccount creates a timeboxed Guard freeze. It immediately revokes the
// account's sessions by setting banned_at, which drops live connections.
func (m *Manager) FreezeAccount(ctx context.Context, guardAdminID, accountID, reason string) error {
	maxH := m.cfg.Tuning.Portal.GuardFreezeMaxH
	if maxH <= 0 {
		maxH = 48
	}
	expires := time.Now().UTC().Add(time.Duration(maxH) * time.Hour)

	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// One active freeze per Guard per target.
	var active int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM guard_freezes
		 WHERE account_id = $1 AND frozen_by = $2
		   AND dismissed_at IS NULL AND converted_to_ban_at IS NULL
		   AND expires_at > now()`,
		accountID, guardAdminID,
	).Scan(&active); err != nil {
		return fmt.Errorf("check active freeze: %w", err)
	}
	if active > 0 {
		return fmt.Errorf("guard already has active freeze on target")
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO guard_freezes (account_id, frozen_by, reason, frozen_at, expires_at, created_at, updated_at)
		 VALUES ($1, $2, $3, now(), $4, now(), now())`,
		accountID, guardAdminID, reason, expires,
	); err != nil {
		return fmt.Errorf("insert freeze: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE accounts SET banned_at = now(), updated_at = now() WHERE id = $1",
		accountID,
	); err != nil {
		return fmt.Errorf("ban account: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit freeze: %w", err)
	}

	_ = m.auth.RevokeAccount(ctx, accountID)
	_ = m.admin.LogAction(ctx, guardAdminID, "guard_freeze", "account", accountID,
		map[string]any{"banned_at": nil},
		map[string]any{"banned_at": time.Now().UTC(), "expires_at": expires, "reason": reason},
	)
	return nil
}

// DismissFreeze clears a Guard freeze and unbans the account.
func (m *Manager) DismissFreeze(ctx context.Context, adminID, freezeID string) error {
	var accountID string
	err := m.db.QueryRowContext(ctx,
		`UPDATE guard_freezes
		 SET dismissed_at = now(), dismissed_by = $2, updated_at = now()
		 WHERE id = $1 AND dismissed_at IS NULL AND converted_to_ban_at IS NULL
		 RETURNING account_id`,
		freezeID, adminID,
	).Scan(&accountID)
	if err != nil {
		return fmt.Errorf("dismiss freeze: %w", err)
	}
	if _, err := m.db.ExecContext(ctx,
		"UPDATE accounts SET banned_at = NULL, updated_at = now() WHERE id = $1 AND banned_at IS NOT NULL",
		accountID,
	); err != nil {
		return fmt.Errorf("unban account: %w", err)
	}
	_ = m.admin.LogAction(ctx, adminID, "guard_freeze_dismiss", "account", accountID,
		map[string]any{"status": "frozen"},
		map[string]any{"status": "dismissed"},
	)
	return nil
}

// ConvertFreezeToBan makes a Guard freeze permanent.
func (m *Manager) ConvertFreezeToBan(ctx context.Context, adminID, freezeID, banReason string) error {
	var accountID string
	err := m.db.QueryRowContext(ctx,
		`UPDATE guard_freezes
		 SET converted_to_ban_at = now(), converted_to_ban_by = $2, updated_at = now()
		 WHERE id = $1 AND dismissed_at IS NULL AND converted_to_ban_at IS NULL
		 RETURNING account_id`,
		freezeID, adminID,
	).Scan(&accountID)
	if err != nil {
		return fmt.Errorf("convert freeze: %w", err)
	}
	if _, err := m.db.ExecContext(ctx,
		"UPDATE accounts SET banned_at = now(), updated_at = now() WHERE id = $1",
		accountID,
	); err != nil {
		return fmt.Errorf("ban account: %w", err)
	}
	_ = m.auth.RevokeAccount(ctx, accountID)
	_ = m.admin.LogAction(ctx, adminID, "guard_freeze_convert_ban", "account", accountID,
		map[string]any{"status": "frozen"},
		map[string]any{"status": "banned", "reason": banReason},
	)
	return nil
}

// ListActiveFreezes returns freezes that have not expired, been dismissed, or converted.
func (m *Manager) ListActiveFreezes(ctx context.Context) ([]map[string]any, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, account_id, frozen_by, reason, frozen_at, expires_at
		 FROM guard_freezes
		 WHERE dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expires_at > now()
		 ORDER BY frozen_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query freezes: %w", err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, accountID, frozenBy, reason string
		var frozenAt, expiresAt time.Time
		if err := rows.Scan(&id, &accountID, &frozenBy, &reason, &frozenAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan freeze: %w", err)
		}
		out = append(out, map[string]any{
			"id": id, "account_id": accountID, "frozen_by": frozenBy,
			"reason": reason, "frozen_at": frozenAt, "expires_at": expiresAt,
		})
	}
	return out, rows.Err()
}

// ExpireFreezes clears any freezes past their expiry and unbans the account if
// no other active freeze exists. Safe to call on startup and periodically.
func (m *Manager) ExpireFreezes(ctx context.Context) error {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT id, account_id FROM guard_freezes
		 WHERE dismissed_at IS NULL AND converted_to_ban_at IS NULL
		   AND expires_at <= now() FOR UPDATE`)
	if err != nil {
		return fmt.Errorf("query expired freezes: %w", err)
	}
	type freezeRef struct {
		id        string
		accountID string
	}
	var refs []freezeRef
	for rows.Next() {
		var r freezeRef
		if err := rows.Scan(&r.id, &r.accountID); err != nil {
			rows.Close()
			return fmt.Errorf("scan expired freeze: %w", err)
		}
		refs = append(refs, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range refs {
		if _, err := tx.ExecContext(ctx,
			"UPDATE guard_freezes SET updated_at = now() WHERE id = $1",
			r.id,
		); err != nil {
			return fmt.Errorf("touch expired freeze: %w", err)
		}
		var active int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM guard_freezes
			 WHERE account_id = $1 AND dismissed_at IS NULL AND converted_to_ban_at IS NULL
			   AND expires_at > now()`,
			r.accountID,
		).Scan(&active); err != nil {
			return fmt.Errorf("check remaining freezes: %w", err)
		}
		if active == 0 {
			if _, err := tx.ExecContext(ctx,
				"UPDATE accounts SET banned_at = NULL, updated_at = now() WHERE id = $1",
				r.accountID,
			); err != nil {
				return fmt.Errorf("unban on expiry: %w", err)
			}
		}
	}
	return tx.Commit()
}

// ── Weekly Nown Challenge ──────────────────────────────────────────────────

// CreateChallengeTopic publishes a new weekly topic.
// ── Helpers ────────────────────────────────────────────────────────────────

// EnsureActiveTermsVersion creates a portal_terms row for the configured active
// version if one does not already exist. This lets deployments start from a
// fresh database without an admin first creating the terms record.
func (m *Manager) EnsureActiveTermsVersion(ctx context.Context) error {
	version := m.activeTermsVersion()
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO portal_terms (version, title, body, active_from)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (version) DO NOTHING`,
		version, "Contribution Terms "+version, "Terms body for "+version,
	)
	if err != nil {
		return fmt.Errorf("ensure portal terms %s: %w", version, err)
	}
	return nil
}

func (m *Manager) activeTermsVersion() string {
	// The active terms version is authoritative in config; the database table
	// stores the record of accepted versions. Fallback to "v1" if unset.
	if m.cfg.Tuning.Portal.TermsVersion != "" {
		return m.cfg.Tuning.Portal.TermsVersion
	}
	return "v1"
}

func nullableString(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func nullableTime(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

func nullableWinnerID(w *ChallengeEntry) *string {
	if w == nil {
		return nil
	}
	return &w.ID
}

func scanSubmissions(rows *sql.Rows) ([]Submission, error) {
	var out []Submission
	for rows.Next() {
		var s Submission
		var assetRef sql.NullString
		var toneBucket sql.NullString
		var packTag sql.NullString
		var decidedAt sql.NullTime
		var decidedBy sql.NullString
		var rejection sql.NullString
		var submittedAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.AccountID, &s.MediaType, &s.Content, &assetRef, &s.Status, pq.Array(&s.Tags),
			&toneBucket, &s.NownID, &packTag, &s.TermsVersion, &s.TermsAcceptedAt,
			&submittedAt, &decidedAt, &decidedBy, &rejection); err != nil {
			return nil, fmt.Errorf("scan submission: %w", err)
		}
		s.AssetRef = assetRef.String
		s.ToneBucket = toneBucket.String
		s.PackTag = packTag.String
		s.SubmittedAt = nullableTime(submittedAt)
		s.DecidedAt = nullableTime(decidedAt)
		s.DecidedBy = nullableString(decidedBy)
		s.RejectionReason = rejection.String
		out = append(out, s)
	}
	return out, rows.Err()
}

func scanChallengeEntries(rows *sql.Rows) ([]ChallengeEntry, error) {
	var out []ChallengeEntry
	for rows.Next() {
		var e ChallengeEntry
		var assetRef sql.NullString
		var slotNumber sql.NullInt32
		var rejection sql.NullString
		if err := rows.Scan(&e.ID, &e.AccountID, &e.TopicID, &e.EntryType, &e.Content, &assetRef, &e.Status, &e.VoteCount, &slotNumber, &rejection); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.AssetRef = assetRef.String
		e.SlotNumber = int(slotNumber.Int32)
		e.RejectionReason = rejection.String
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanChallengeEntry(row *sql.Row) (*ChallengeEntry, error) {
	var e ChallengeEntry
	var assetRef sql.NullString
	var slotNumber sql.NullInt32
	var rejection sql.NullString
	err := row.Scan(&e.ID, &e.AccountID, &e.TopicID, &e.EntryType, &e.Content, &assetRef, &e.Status, &e.VoteCount, &slotNumber, &rejection)
	if err != nil {
		return nil, err
	}
	e.AssetRef = assetRef.String
	e.SlotNumber = int(slotNumber.Int32)
	e.RejectionReason = rejection.String
	return &e, nil
}

// ListOwnApplications never exposes another account's application history.
func (m *Manager) ListOwnApplications(ctx context.Context, accountID string) ([]RoleApplication, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id,role,status,applied_at,COALESCE(reason,'') FROM portal_role_applications WHERE account_id=$1 ORDER BY applied_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoleApplication{}
	for rows.Next() {
		var a RoleApplication
		a.AccountID = accountID
		if err = rows.Scan(&a.ID, &a.Role, &a.Status, &a.AppliedAt, &a.Reason); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListRoleGrants includes revoked roles so an administrator can inspect history.
func (m *Manager) ListRoleGrants(ctx context.Context) ([]RoleGrant, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT account_id,role,granted_at,COALESCE(granted_by::text,''),revoked_at FROM portal_roles ORDER BY granted_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoleGrant{}
	for rows.Next() {
		var a RoleGrant
		if err = rows.Scan(&a.AccountID, &a.Role, &a.GrantedAt, &a.GrantedBy, &a.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ApproveApplication serializes the application decision with rejection and
// binds the granted role to the exact pending request shown to the reviewer.
func (m *Manager) ApproveApplication(ctx context.Context, adminID, applicationID string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var account, role, status string
	if err = tx.QueryRowContext(ctx, `SELECT account_id,role,status FROM portal_role_applications WHERE id=$1 FOR UPDATE`, applicationID).Scan(&account, &role, &status); err != nil {
		return fmt.Errorf("application unavailable")
	}
	if status != "pending" {
		return fmt.Errorf("application already decided")
	}
	if !ValidRole(role) {
		return fmt.Errorf("invalid role")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO portal_roles(account_id,role,granted_by) VALUES($1,$2,$3) ON CONFLICT(account_id,role) DO UPDATE SET granted_by=$3,granted_at=now(),revoked_at=NULL,revoked_by=NULL,updated_at=now()`, account, role, adminID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE portal_role_applications SET status='approved',decided_by=$2,decided_at=now(),updated_at=now() WHERE id=$1`, applicationID, adminID); err != nil {
		return err
	}
	if err = auditTx(ctx, tx, adminID, "portal_application_approve", "portal_role_application", applicationID, map[string]any{"status": "pending"}, map[string]any{"status": "approved", "role": role, "account_id": account}); err != nil {
		return err
	}
	return tx.Commit()
}

// ContentRevision binds a human approval form to the exact text shown.
func ContentRevision(content string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(content))) }
