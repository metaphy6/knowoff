// Package portal implements the public Contributor Portal: role applications,
// media submissions, Guard freezes, and the Weekly Nown Challenge.
package portal

import (
	"context"
	"database/sql"
	"fmt"
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
	MediaGIF   MediaType = "gif"
)

// ValidMediaType reports whether t is a known media type.
func ValidMediaType(t string) bool {
	switch MediaType(t) {
	case MediaText, MediaImage, MediaGIF:
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
	ID          string
	AccountID   string
	Role        Role
	Status      ApplicationStatus
	AppliedAt   time.Time
	DecidedAt   *time.Time
	DecidedBy   *string
	Reason      string
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
	ID                string
	AccountID         string
	MediaType         MediaType
	Content           string
	AssetRef          string
	Status            SubmissionStatus
	Tags              []string
	ToneBucket        string
	NownID            *string
	PackTag           string
	TermsVersion      string
	TermsAcceptedAt   time.Time
	SubmittedAt       *time.Time
	DecidedAt         *time.Time
	DecidedBy         *string
	RejectionReason   string
}

// ChallengeTopic is the weekly Nown topic.
type ChallengeTopic struct {
	ID           string
	WeekStart    time.Time
	WeekEnd      time.Time
	NownMediaID  string
	PublishedAt  time.Time
	ClosedAt     *time.Time
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
	DB      *sql.DB
	Config  *config.Config
	Auth    AuthClient
	Profile *profile.Manager
	Economy *economy.Manager
	Admin   AuditLogger
	Media   *media.Manager
}

// Manager is the portal service.
type Manager struct {
	db      *sql.DB
	cfg     *config.Config
	auth    AuthClient
	profile *profile.Manager
	economy *economy.Manager
	admin   AuditLogger
	media   *media.Manager
}

// NewManager returns a portal manager.
func NewManager(deps Deps) *Manager {
	return &Manager{
		db:      deps.DB,
		cfg:     deps.Config,
		auth:    deps.Auth,
		profile: deps.Profile,
		economy: deps.Economy,
		admin:   deps.Admin,
		media:   deps.Media,
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
		 ON CONFLICT (account_id, role, status) DO UPDATE SET
		   updated_at = now(),
		   applied_at = EXCLUDED.applied_at`,
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
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit grant role: %w", err)
	}

	before := map[string]any{}
	if oldRole.Valid {
		before["role"] = oldRole.String
	}
	_ = m.admin.LogAction(ctx, adminID, "portal_role_grant", "portal_role", accountID, before, map[string]any{"role": string(role)})
	return nil
}

// TermsVersion is a versioned contribution terms record.
type TermsVersion struct {
	Version   string
	Title     string
	Body      string
	ActiveFrom time.Time
	CreatedAt time.Time
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
	if version == "" || title == "" || body == "" {
		return fmt.Errorf("version, title and body required")
	}
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO portal_terms (version, title, body, active_from, created_at)
		 VALUES ($1, $2, $3, $4, now())`,
		version, title, body, activeFrom,
	)
	if err != nil {
		return fmt.Errorf("insert terms: %w", err)
	}
	_ = m.admin.LogAction(ctx, adminID, "portal_terms_create", "portal_terms", version,
		map[string]any{},
		map[string]any{"title": title, "active_from": activeFrom},
	)
	return nil
}

// RejectApplication marks a role application as rejected.
func (m *Manager) RejectApplication(ctx context.Context, adminID, applicationID, reason string) error {
	res, err := m.db.ExecContext(ctx,
		`UPDATE portal_role_applications
		 SET status = 'rejected', decided_at = now(), decided_by = $2, reason = $3, updated_at = now()
		 WHERE id = $1 AND status = 'pending'`,
		applicationID, adminID, reason,
	)
	if err != nil {
		return fmt.Errorf("reject application: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("application not found or already decided")
	}
	_ = m.admin.LogAction(ctx, adminID, "portal_application_reject", "portal_role_application", applicationID,
		map[string]any{"status": "pending"},
		map[string]any{"status": "rejected", "reason": reason},
	)
	return nil
}

// RevokeRole revokes a specific portal role.
func (m *Manager) RevokeRole(ctx context.Context, adminID, accountID string, role Role) error {
	res, err := m.db.ExecContext(ctx,
		`UPDATE portal_roles
		 SET revoked_at = now(), revoked_by = $3, updated_at = now()
		 WHERE account_id = $1 AND role = $2 AND revoked_at IS NULL`,
		accountID, string(role), adminID,
	)
	if err != nil {
		return fmt.Errorf("revoke role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no active role")
	}
	_ = m.admin.LogAction(ctx, adminID, "portal_role_revoke", "portal_role", accountID,
		map[string]any{"status": "active"},
		map[string]any{"status": "revoked", "role": string(role)},
	)
	return nil
}

// HasRole reports whether an account holds the requested role or a higher one
// in the portal hierarchy (Guard > Curator > Contributor).
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
	requiredRank := roleRank(role)
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return false, fmt.Errorf("scan role: %w", err)
		}
		if roleRank(Role(r)) >= requiredRank {
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
func (m *Manager) CreateDraft(ctx context.Context, accountID string, mediaType MediaType, content string) (*Submission, error) {
	if !ValidMediaType(string(mediaType)) {
		return nil, fmt.Errorf("invalid media type")
	}
	if mediaType == MediaText {
		if len(content) == 0 {
			return nil, fmt.Errorf("text content required")
		}
		maxLen := m.cfg.Tuning.Portal.MaxTextSubmissionLength
		if maxLen <= 0 {
			maxLen = 2000
		}
		if len(content) > maxLen {
			return nil, fmt.Errorf("text content exceeds %d characters", maxLen)
		}
	}
	id := uuid.New().String()
	termsVersion := m.activeTermsVersion()
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO portal_submissions (id, account_id, media_type, content, status, tags, terms_version, terms_accepted_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'draft', '{}', $5, now(), now(), now())`,
		id, accountID, string(mediaType), content, termsVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("insert draft: %w", err)
	}
	return m.GetSubmission(ctx, id)
}

// SubmitDraft moves a draft to the submitted queue. The account must hold the
// contributor role and must not have exceeded the daily cap.
func (m *Manager) SubmitDraft(ctx context.Context, accountID, submissionID string) error {
	ok, err := m.HasRole(ctx, accountID, RoleContributor)
	if err != nil {
		return fmt.Errorf("check role: %w", err)
	}
	if !ok {
		return fmt.Errorf("contributor role required")
	}
	day := serverDay(time.Now().UTC())
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var owner string
	var status string
	if err := tx.QueryRowContext(ctx,
		"SELECT account_id, status FROM portal_submissions WHERE id = $1",
		submissionID,
	).Scan(&owner, &status); err != nil {
		return fmt.Errorf("load submission: %w", err)
	}
	if owner != accountID {
		return fmt.Errorf("not owner")
	}
	if status != string(StatusDraft) {
		return fmt.Errorf("submission not draft")
	}

	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COALESCE(count,0) FROM portal_submission_counts WHERE account_id = $1 AND server_day = $2 FOR UPDATE",
		accountID, day,
	).Scan(&count); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lock submission count: %w", err)
	}
	if count >= m.cfg.Tuning.Portal.SubmissionsPerContributorPerDay {
		return fmt.Errorf("daily submission cap reached")
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE portal_submissions
		 SET status = 'submitted', submitted_at = now(), updated_at = now()
		 WHERE id = $1`,
		submissionID,
	); err != nil {
		return fmt.Errorf("update submission: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO portal_submission_counts (account_id, server_day, count)
		 VALUES ($1, $2, 1)
		 ON CONFLICT (account_id, server_day) DO UPDATE SET
		   count = portal_submission_counts.count + 1,
		   updated_at = now()`,
		accountID, day,
	); err != nil {
		return fmt.Errorf("increment count: %w", err)
	}

	return tx.Commit()
}

// DecideSubmission approves or rejects a submission. Only curators/admins may decide.
func (m *Manager) DecideSubmission(ctx context.Context, adminID, submissionID string, approve bool, reason string) error {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var s Submission
	var assetRef sql.NullString
	var toneBucket sql.NullString
	if err := tx.QueryRowContext(ctx,
		`SELECT id, account_id, status, media_type, content, asset_ref, tags, tone_bucket
		 FROM portal_submissions WHERE id = $1`,
		submissionID,
	).Scan(&s.ID, &s.AccountID, &s.Status, &s.MediaType, &s.Content, &assetRef, pq.Array(&s.Tags), &toneBucket); err != nil {
		return fmt.Errorf("load submission: %w", err)
	}
	s.AssetRef = assetRef.String
	s.ToneBucket = toneBucket.String
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
			int64(m.cfg.Tuning.Noin.DailyEarnCap),
		)
		if err != nil {
			return fmt.Errorf("grant contributor reward: %w", err)
		}
		if err := m.profile.AddContributorCreditTx(ctx, tx, s.AccountID, submissionID); err != nil {
			return fmt.Errorf("add contributor credit: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit decide: %w", err)
	}

	before := map[string]any{"status": string(s.Status)}
	after := map[string]any{"status": string(newStatus), "reason": reason}
	_ = m.admin.LogAction(ctx, adminID, "submission_decide", "submission", submissionID, before, after)
	return nil
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
	res, err := m.db.ExecContext(ctx,
		`UPDATE portal_submissions
		 SET status = 'draft', submitted_at = NULL, updated_at = now()
		 WHERE id = $1 AND account_id = $2 AND status IN ('draft','submitted','in_review')`,
		submissionID, accountID,
	)
	if err != nil {
		return fmt.Errorf("withdraw submission: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("submission not withdrawable")
	}
	return nil
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
func (m *Manager) CreateChallengeTopic(ctx context.Context, adminID string, weekStart time.Time, nownMediaID string) (*ChallengeTopic, error) {
	weekEnd := weekStart.AddDate(0, 0, 6)
	id := uuid.New().String()
	if _, err := m.db.ExecContext(ctx,
		`INSERT INTO challenge_topics (id, week_start, week_end, nown_media_id, published_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, now(), now(), now())`,
		id, weekStart, weekEnd, nownMediaID,
	); err != nil {
		return nil, fmt.Errorf("insert topic: %w", err)
	}
	_ = m.admin.LogAction(ctx, adminID, "challenge_topic_create", "challenge_topic", id,
		map[string]any{},
		map[string]any{"week_start": weekStart, "nown_media_id": nownMediaID},
	)
	return m.GetChallengeTopic(ctx, id)
}

// GetChallengeTopic loads a topic by id.
func (m *Manager) GetChallengeTopic(ctx context.Context, id string) (*ChallengeTopic, error) {
	var t ChallengeTopic
	var closedAt sql.NullTime
	var winnerID sql.NullString
	err := m.db.QueryRowContext(ctx,
		`SELECT id, week_start, week_end, nown_media_id, published_at, closed_at, winner_entry_id
		 FROM challenge_topics WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.WeekStart, &t.WeekEnd, &t.NownMediaID, &t.PublishedAt, &closedAt, &winnerID)
	if err != nil {
		return nil, fmt.Errorf("load topic: %w", err)
	}
	t.ClosedAt = nullableTime(closedAt)
	t.WinnerEntryID = nullableString(winnerID)
	return &t, nil
}

// ActiveChallengeTopic returns the currently open challenge, if any.
func (m *Manager) ActiveChallengeTopic(ctx context.Context) (*ChallengeTopic, error) {
	var t ChallengeTopic
	var id string
	var closedAt sql.NullTime
	var winnerID sql.NullString
	err := m.db.QueryRowContext(ctx,
		`SELECT id, week_start, week_end, nown_media_id, published_at, closed_at, winner_entry_id
		 FROM challenge_topics
		 WHERE closed_at IS NULL AND week_start <= $1 AND week_end >= $1
		 ORDER BY week_start DESC LIMIT 1`,
		time.Now().UTC(),
	).Scan(&id, &t.WeekStart, &t.WeekEnd, &t.NownMediaID, &t.PublishedAt, &closedAt, &winnerID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load active topic: %w", err)
	}
	t.ID = id
	t.ClosedAt = nullableTime(closedAt)
	t.WinnerEntryID = nullableString(winnerID)
	return &t, nil
}

// SubmitChallengeEntry records a player's entry. The first 100 approved entries
// get slot numbers; rejections reopen slots.
func (m *Manager) SubmitChallengeEntry(ctx context.Context, accountID, topicID string, entryType MediaType, content string) (*ChallengeEntry, error) {
	if !ValidMediaType(string(entryType)) {
		return nil, fmt.Errorf("invalid entry type")
	}
	if entryType == MediaText && len(content) == 0 {
		return nil, fmt.Errorf("text content required")
	}

	topic, err := m.GetChallengeTopic(ctx, topicID)
	if err != nil {
		return nil, err
	}
	if topic == nil || topic.ClosedAt != nil {
		return nil, fmt.Errorf("challenge not open")
	}
	if topic.WeekStart.After(time.Now().UTC()) || topic.WeekEnd.Before(time.Now().UTC()) {
		return nil, fmt.Errorf("challenge not active")
	}

	termsVersion := m.activeTermsVersion()
	id := uuid.New().String()
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO challenge_entries (id, account_id, topic_id, entry_type, content, terms_version, terms_accepted_at, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now(), 'submitted', now(), now())`,
		id, accountID, topicID, string(entryType), content, termsVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("insert entry: %w", err)
	}
	return m.GetChallengeEntry(ctx, id)
}

// GetChallengeEntry loads an entry.
func (m *Manager) GetChallengeEntry(ctx context.Context, id string) (*ChallengeEntry, error) {
	var e ChallengeEntry
	var assetRef sql.NullString
	var slotNumber sql.NullInt32
	var rejection sql.NullString
	err := m.db.QueryRowContext(ctx,
		`SELECT id, account_id, topic_id, entry_type, content, asset_ref, status, vote_count, slot_number, rejection_reason
		 FROM challenge_entries WHERE id = $1`,
		id,
	).Scan(&e.ID, &e.AccountID, &e.TopicID, &e.EntryType, &e.Content, &assetRef, &e.Status, &e.VoteCount, &slotNumber, &rejection)
	if err != nil {
		return nil, fmt.Errorf("load entry: %w", err)
	}
	e.AssetRef = assetRef.String
	e.SlotNumber = int(slotNumber.Int32)
	e.RejectionReason = rejection.String
	return &e, nil
}

// ListChallengeEntries returns visible (approved) entries for a topic.
func (m *Manager) ListChallengeEntries(ctx context.Context, topicID string) ([]ChallengeEntry, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, account_id, topic_id, entry_type, content, asset_ref, status, vote_count, slot_number, rejection_reason
		 FROM challenge_entries
		 WHERE topic_id = $1 AND status = 'approved'
		 ORDER BY slot_number ASC, created_at ASC`,
		topicID,
	)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()
	return scanChallengeEntries(rows)
}

// VoteChallengeEntry records one immutable vote per player per topic.
func (m *Manager) VoteChallengeEntry(ctx context.Context, accountID, topicID, entryID string) error {
	if accountID == "" {
		return fmt.Errorf("account required")
	}
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var entryOwner string
	var entryStatus string
	var entryTopicID string
	if err := tx.QueryRowContext(ctx,
		"SELECT account_id, status, topic_id FROM challenge_entries WHERE id = $1",
		entryID,
	).Scan(&entryOwner, &entryStatus, &entryTopicID); err != nil {
		return fmt.Errorf("load entry: %w", err)
	}
	if entryTopicID != topicID {
		return fmt.Errorf("entry does not belong to topic")
	}
	if entryOwner == accountID {
		return fmt.Errorf("cannot vote for own entry")
	}
	if entryStatus != string(EntryApproved) {
		return fmt.Errorf("entry not visible")
	}

	if _, err := tx.ExecContext(ctx,
		"INSERT INTO challenge_votes (account_id, topic_id, entry_id) VALUES ($1, $2, $3)",
		accountID, topicID, entryID,
	); err != nil {
		return fmt.Errorf("insert vote: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE challenge_entries SET vote_count = vote_count + 1, updated_at = now() WHERE id = $1",
		entryID,
	); err != nil {
		return fmt.Errorf("increment vote count: %w", err)
	}

	return tx.Commit()
}

// ApproveChallengeEntry assigns a slot number atomically (first-100 race-proof).
func (m *Manager) ApproveChallengeEntry(ctx context.Context, adminID, entryID string) error {
	max := m.cfg.Tuning.LiveOps.ChallengeMaxEntries
	if max <= 0 {
		max = 100
	}
	// ReadCommitted + row lock on the topic so concurrent approvals serialize
	// and see each other's committed slot assignments.
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var topicID string
	var status string
	if err := tx.QueryRowContext(ctx,
		"SELECT topic_id, status FROM challenge_entries WHERE id = $1",
		entryID,
	).Scan(&topicID, &status); err != nil {
		return fmt.Errorf("load entry: %w", err)
	}
	if status != string(EntrySubmitted) && status != string(EntryScreening) {
		return fmt.Errorf("entry not screenable")
	}

	// Serialize concurrent approvals on the topic row.
	if _, err := tx.ExecContext(ctx,
		"SELECT 1 FROM challenge_topics WHERE id = $1 FOR UPDATE",
		topicID,
	); err != nil {
		return fmt.Errorf("lock topic: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		"SELECT slot_number FROM challenge_entries WHERE topic_id = $1 AND status = 'approved' AND slot_number IS NOT NULL ORDER BY slot_number",
		topicID,
	)
	if err != nil {
		return fmt.Errorf("list slots: %w", err)
	}
	var used []int
	for rows.Next() {
		var s int
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return fmt.Errorf("scan slot: %w", err)
		}
		used = append(used, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("slot rows: %w", err)
	}
	if len(used) >= max {
		return fmt.Errorf("challenge full")
	}
	nextSlot := 1
	for _, s := range used {
		if s == nextSlot {
			nextSlot++
		}
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE challenge_entries
		 SET status = 'approved', slot_number = $2, screen_decided_at = now(), screen_decided_by = $3, updated_at = now()
		 WHERE id = $1 AND status IN ('submitted','screening')`,
		entryID, nextSlot, adminID,
	)
	if err != nil {
		return fmt.Errorf("approve entry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("entry no longer screenable")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit approve: %w", err)
	}
	_ = m.admin.LogAction(ctx, adminID, "challenge_entry_approve", "challenge_entry", entryID,
		map[string]any{"status": status},
		map[string]any{"status": "approved", "slot_number": nextSlot},
	)
	return nil
}

// RejectChallengeEntry removes the entry from the challenge.
func (m *Manager) RejectChallengeEntry(ctx context.Context, adminID, entryID, reason string) error {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var topicID string
	var status string
	if err := tx.QueryRowContext(ctx,
		"SELECT topic_id, status FROM challenge_entries WHERE id = $1",
		entryID,
	).Scan(&topicID, &status); err != nil {
		return fmt.Errorf("load entry: %w", err)
	}
	if status != string(EntrySubmitted) && status != string(EntryScreening) && status != string(EntryApproved) {
		return fmt.Errorf("entry not rejectable")
	}

	// Serialize with concurrent approvals so slot accounting stays consistent.
	if _, err := tx.ExecContext(ctx,
		"SELECT 1 FROM challenge_topics WHERE id = $1 FOR UPDATE",
		topicID,
	); err != nil {
		return fmt.Errorf("lock topic: %w", err)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE challenge_entries
		 SET status = 'rejected', screen_decided_at = now(), screen_decided_by = $2, rejection_reason = $3, slot_number = NULL, updated_at = now()
		 WHERE id = $1 AND status IN ('submitted','screening','approved')`,
		entryID, adminID, reason,
	)
	if err != nil {
		return fmt.Errorf("reject entry: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("entry not rejectable")
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reject: %w", err)
	}
	_ = m.admin.LogAction(ctx, adminID, "challenge_entry_reject", "challenge_entry", entryID,
		map[string]any{},
		map[string]any{"status": "rejected", "reason": reason},
	)
	return nil
}

// CloseChallengeWeek finalizes the active challenge: determines the winner,
// grants the title + Noin payout, and records the result. Idempotent.
func (m *Manager) CloseChallengeWeek(ctx context.Context, adminID string, topicID string) (*ChallengeEntry, error) {
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var topic ChallengeTopic
	var closedAt sql.NullTime
	var winnerID sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT id, week_start, week_end, nown_media_id, published_at, closed_at, winner_entry_id
		 FROM challenge_topics WHERE id = $1`,
		topicID,
	).Scan(&topic.ID, &topic.WeekStart, &topic.WeekEnd, &topic.NownMediaID, &topic.PublishedAt, &closedAt, &winnerID)
	if err != nil {
		return nil, fmt.Errorf("load topic: %w", err)
	}
	if closedAt.Valid {
		// Already closed: return recorded winner if any.
		if !winnerID.Valid {
			return nil, nil
		}
		return m.GetChallengeEntry(ctx, winnerID.String)
	}

	var winner *ChallengeEntry
	row := tx.QueryRowContext(ctx,
		`SELECT id, account_id, topic_id, entry_type, content, asset_ref, status, vote_count, slot_number, rejection_reason
		 FROM challenge_entries
		 WHERE topic_id = $1 AND status = 'approved'
		 ORDER BY vote_count DESC, slot_number ASC LIMIT 1`,
		topicID,
	)
	winner, err = scanChallengeEntry(row)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("find winner: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE challenge_topics SET closed_at = now(), winner_entry_id = $2, updated_at = now() WHERE id = $1",
		topicID, nullableWinnerID(winner),
	); err != nil {
		return nil, fmt.Errorf("close topic: %w", err)
	}

	if winner != nil {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO challenge_winners (topic_id, entry_id, account_id, title_granted_at, created_at)
			 VALUES ($1, $2, $3, now(), now())
			 ON CONFLICT (topic_id) DO UPDATE SET
			   entry_id = EXCLUDED.entry_id,
			   account_id = EXCLUDED.account_id,
			   title_granted_at = EXCLUDED.title_granted_at`,
			topicID, winner.ID, winner.AccountID,
		); err != nil {
			return nil, fmt.Errorf("record winner: %w", err)
		}
		_, err = m.economy.Wallet.GrantTx(ctx, tx, winner.AccountID, economy.LedgerChallengeWinner,
			m.cfg.Tuning.Noin.ChallengeWinner,
			"weekly nown challenge winner",
			int64(m.cfg.Tuning.Noin.DailyEarnCap),
		)
		if err != nil {
			return nil, fmt.Errorf("grant winner noin: %w", err)
		}
		if err := m.profile.AddWeekWinnerTitleTx(ctx, tx, winner.AccountID); err != nil {
			return nil, fmt.Errorf("grant winner title: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit close: %w", err)
	}

	_ = m.admin.LogAction(ctx, adminID, "challenge_week_close", "challenge_topic", topicID,
		map[string]any{"closed_at": nil},
		map[string]any{"closed_at": time.Now().UTC(), "winner_entry_id": nullableWinnerID(winner)},
	)
	return winner, nil
}

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
