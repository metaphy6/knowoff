// Package notices implements system notices: authoring, scheduling, locale
// fallback, active-notice queries, maintenance-drain signalling, and broadcast
// to connected clients.
package notices

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
)

// NoticeType is the kind of system notice.
type NoticeType string

const (
	NoticeMaintenance  NoticeType = "maintenance"
	NoticeDowntime     NoticeType = "downtime"
	NoticeAnnouncement NoticeType = "announcement"
)

// Notice represents a system notice with localized title/body.
type Notice struct {
	ID                     uuid.UUID
	Type                   NoticeType
	Title                  map[string]string
	Body                   map[string]string
	PublishedAt            *time.Time
	WithdrawnAt            *time.Time
	MaintenanceStart       *time.Time
	MaintenanceDurationMin int
	CreatedBy              *uuid.UUID
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// MatchmakingPauser can pause or resume matchmaking.
type MatchmakingPauser interface {
	SetReady(bool)
}

// Manager owns system notices.
type Manager struct {
	db             *sql.DB
	cfg            *config.Config
	pauser         MatchmakingPauser
	refreshMu      sync.Mutex
	changeNotifier func(context.Context) error
	refreshHash    [32]byte
}

// SetChangeNotifier installs the text transport's public cache invalidation.
// The HTTPS inbox remains authoritative; a failed refresh is retried on the poll.
func (m *Manager) SetChangeNotifier(notify func(context.Context) error) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	m.changeNotifier = notify
	m.refreshHash = [32]byte{}
}

func noticeStage(n Notice, now time.Time) int {
	if n.Type != NoticeMaintenance || n.MaintenanceStart == nil {
		return 0
	}
	start := *n.MaintenanceStart
	switch {
	case !now.Before(start.Add(time.Duration(n.MaintenanceDurationMin) * time.Minute)):
		return 5
	case !now.Before(start):
		return 4
	case !now.Before(start.Add(-10 * time.Minute)):
		return 3
	case !now.Before(start.Add(-time.Hour)):
		return 2
	case !now.Before(start.Add(-24 * time.Hour)):
		return 1
	default:
		return 0
	}
}

func (m *Manager) refreshAt(ctx context.Context, now time.Time) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if m.changeNotifier == nil {
		return nil
	}
	active, err := m.ActiveNotices(ctx, now)
	if err != nil {
		return err
	}
	stages := make([]int, len(active))
	for i, n := range active {
		stages[i] = noticeStage(n, now)
	}
	raw, err := json.Marshal([]any{active, stages})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	if digest == m.refreshHash {
		return nil
	}
	if err = m.changeNotifier(ctx); err != nil {
		return err
	}
	m.refreshHash = digest
	return nil
}

// Run detects publication, withdrawal and maintenance reminder boundaries.
// No gameplay state or financial claim depends on this best-effort invalidation.
func (m *Manager) Run(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		work, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := m.refreshAt(work, time.Now().UTC())
		cancel()
		if err != nil && report != nil && ctx.Err() == nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// NewManager returns a notice manager.
func NewManager(db *sql.DB, cfg *config.Config, pauser MatchmakingPauser) *Manager {
	return &Manager{db: db, cfg: cfg, pauser: pauser}
}

// CreateNotice commits the notice and its audit atomically. The polling
// notifier publishes only public invalidation after commit and retries failures.
func (m *Manager) CreateNotice(ctx context.Context, n Notice) (uuid.UUID, error) {
	if n.Type != NoticeMaintenance && n.Type != NoticeDowntime && n.Type != NoticeAnnouncement {
		return uuid.Nil, fmt.Errorf("invalid notice type")
	}
	if n.Type == NoticeMaintenance && (n.MaintenanceStart == nil || n.MaintenanceDurationMin <= 0) {
		return uuid.Nil, fmt.Errorf("maintenance notices require start time and duration")
	}
	if len(n.Title) == 0 || len(n.Body) == 0 {
		return uuid.Nil, fmt.Errorf("title and body required")
	}

	for _, localized := range []map[string]string{n.Title, n.Body} {
		for locale, text := range localized {
			if strings.TrimSpace(locale) == "" || strings.TrimSpace(text) == "" {
				return uuid.Nil, fmt.Errorf("title and body required in every provided locale")
			}
		}
	}
	now := time.Now().UTC()
	if n.PublishedAt == nil || n.PublishedAt.IsZero() {
		n.PublishedAt = &now
	}

	titleJSON, err := json.Marshal(n.Title)
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal title: %w", err)
	}
	bodyJSON, err := json.Marshal(n.Body)
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal body: %w", err)
	}

	id := uuid.New()
	var createdBy interface{}
	var actor string
	if n.CreatedBy != nil {
		createdBy = *n.CreatedBy
		actor = n.CreatedBy.String()
	}

	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin notice transaction: %w", err)
	}
	defer tx.Rollback()
	if err = lockNoticeActor(ctx, tx, actor); err != nil {
		return uuid.Nil, err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO system_notices (id, type, title, body, published_at, maintenance_start, maintenance_duration_min, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())`,
		id, string(n.Type), titleJSON, bodyJSON, n.PublishedAt, n.MaintenanceStart, n.MaintenanceDurationMin, createdBy,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert notice: %w", err)
	}

	if err = auditNotice(ctx, tx, createdBy, "notice_create", id, map[string]any{}, map[string]any{"type": n.Type, "title": n.Title, "body": n.Body, "published_at": n.PublishedAt, "maintenance_start": n.MaintenanceStart, "maintenance_duration_min": n.MaintenanceDurationMin}); err != nil {
		return uuid.Nil, err
	}
	if err = lockNoticeActor(ctx, tx, actor); err != nil {
		return uuid.Nil, err
	}
	if err = tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("commit notice: %w", err)
	}
	return id, nil
}

// ActiveNotices returns all published and not-withdrawn notices at the given time.
func (m *Manager) ActiveNotices(ctx context.Context, now time.Time) ([]Notice, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, type, title, body, published_at, withdrawn_at, maintenance_start, maintenance_duration_min, created_by, created_at, updated_at
		 FROM system_notices
		 WHERE published_at IS NOT NULL AND published_at <= $1 AND (withdrawn_at IS NULL OR withdrawn_at > $1)
		 ORDER BY created_at DESC`,
		now)
	if err != nil {
		return nil, fmt.Errorf("query notices: %w", err)
	}
	defer rows.Close()

	var out []Notice
	for rows.Next() {
		var n Notice
		var nType string
		var title, body []byte
		var createdBy sql.NullString
		if err := rows.Scan(&n.ID, &nType, &title, &body, &n.PublishedAt, &n.WithdrawnAt, &n.MaintenanceStart, &n.MaintenanceDurationMin, &createdBy, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notice: %w", err)
		}
		n.Type = NoticeType(nType)
		if err := json.Unmarshal(title, &n.Title); err != nil {
			n.Title = map[string]string{"en": string(title)}
		}
		if err := json.Unmarshal(body, &n.Body); err != nil {
			n.Body = map[string]string{"en": string(body)}
		}
		if createdBy.Valid {
			if uid, err := uuid.Parse(createdBy.String); err == nil {
				n.CreatedBy = &uid
			}
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ActiveNoticesForLocale returns active notices with title/body resolved to the
// requested locale (falling back to the configured default locale, then any).
type LocalizedNotice struct {
	ID                     uuid.UUID  `json:"id"`
	Type                   string     `json:"type"`
	Title                  string     `json:"title"`
	Body                   string     `json:"body"`
	PublishedAt            *time.Time `json:"published_at,omitempty"`
	MaintenanceStart       *time.Time `json:"maintenance_start,omitempty"`
	MaintenanceDurationMin int        `json:"maintenance_duration_min,omitempty"`
	ReminderStage          int        `json:"reminder_stage"`
}

func (m *Manager) ActiveNoticesForLocale(ctx context.Context, locale string) ([]LocalizedNotice, error) {
	if locale == "" {
		locale = m.cfg.Localization.DefaultLocale
	}
	now := time.Now().UTC()
	active, err := m.ActiveNotices(ctx, now)
	if err != nil {
		return nil, err
	}
	out := make([]LocalizedNotice, 0, len(active))
	for _, n := range active {
		out = append(out, LocalizedNotice{
			ID:                     n.ID,
			Type:                   string(n.Type),
			Title:                  pickLocale(n.Title, locale, m.cfg.Localization.DefaultLocale),
			Body:                   pickLocale(n.Body, locale, m.cfg.Localization.DefaultLocale),
			PublishedAt:            n.PublishedAt,
			MaintenanceStart:       n.MaintenanceStart,
			MaintenanceDurationMin: n.MaintenanceDurationMin,
			ReminderStage:          noticeStage(n, now),
		})
	}
	return out, nil
}

// WithdrawNotice marks a notice as withdrawn.
func (m *Manager) WithdrawNotice(ctx context.Context, id uuid.UUID, adminID ...string) error {
	var actor any
	var namedActor string
	if len(adminID) > 1 {
		return fmt.Errorf("invalid notice actor")
	}
	if len(adminID) == 1 {
		parsed, err := uuid.Parse(adminID[0])
		if err != nil {
			return fmt.Errorf("invalid notice actor")
		}
		actor = parsed
		namedActor = parsed.String()
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockNoticeActor(ctx, tx, namedActor); err != nil {
		return err
	}
	var previous sql.NullTime
	if err = tx.QueryRowContext(ctx, `SELECT withdrawn_at FROM system_notices WHERE id=$1 FOR UPDATE`, id).Scan(&previous); err != nil {
		return fmt.Errorf("notice unavailable: %w", err)
	}
	if previous.Valid {
		return lockNoticeActor(ctx, tx, namedActor)
	}
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `UPDATE system_notices SET withdrawn_at=$2,updated_at=$2 WHERE id=$1`, id, now); err != nil {
		return fmt.Errorf("withdraw notice: %w", err)
	}
	if err = auditNotice(ctx, tx, actor, "notice_withdraw", id, map[string]any{"withdrawn_at": nil}, map[string]any{"withdrawn_at": now}); err != nil {
		return err
	}
	if err = lockNoticeActor(ctx, tx, namedActor); err != nil {
		return err
	}
	return tx.Commit()
}

func lockNoticeActor(ctx context.Context, tx *sql.Tx, actor string) error {
	if actor == "" && !store.HasAdminAuthorization(ctx) {
		return nil // Explicit trusted system notice, never a browser fallback.
	}
	return store.LockAdminTx(ctx, tx, actor, []string{"admin"})
}

func auditNotice(ctx context.Context, tx *sql.Tx, actor any, action string, id uuid.UUID, before, after map[string]any) error {
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,before_state,after_state) VALUES($1,$2,'system_notice',$3,$4,$5)`, actor, action, id, b, a)
	if err != nil {
		return fmt.Errorf("audit notice: %w", err)
	}
	return nil
}

// MarkMaintenanceDrain closes admission only inside a published maintenance
// window. Announcements before its start do not interrupt matchmaking.
func (m *Manager) MarkMaintenanceDrain(ctx context.Context) error {
	return m.markMaintenanceDrain(ctx, time.Now().UTC())
}
func (m *Manager) markMaintenanceDrain(ctx context.Context, now time.Time) error {
	if m.pauser == nil {
		return nil
	}
	var paused bool
	err := m.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM system_notices
 WHERE type='maintenance' AND published_at IS NOT NULL AND published_at<=$1
 AND (withdrawn_at IS NULL OR withdrawn_at>$1) AND maintenance_start<=$1
 AND maintenance_start + maintenance_duration_min * interval '1 minute'>$1)`, now).Scan(&paused)
	if err != nil {
		m.pauser.SetReady(false)
		return fmt.Errorf("query maintenance notices: %w", err)
	}
	m.pauser.SetReady(!paused)
	return nil
}

// HTTPActiveNotices serves GET /api/notices.
func (m *Manager) HTTPActiveNotices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	locale := r.URL.Query().Get("locale")
	notices, err := m.ActiveNoticesForLocale(ctx, locale)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"notices": notices})
}

func pickLocale(m map[string]string, locales ...string) string {
	for _, l := range locales {
		if v, ok := m[l]; ok && v != "" {
			return v
		}
	}
	for _, v := range m {
		if v != "" {
			return v
		}
	}
	return ""
}
