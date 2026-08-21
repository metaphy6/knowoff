// Package notices implements system notices: authoring, scheduling, locale
// fallback, active-notice queries, maintenance-drain signalling, and broadcast
// to connected clients.
package notices

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/transport"
)

// NoticeType is the kind of system notice.
type NoticeType string

const (
	NoticeMaintenance   NoticeType = "maintenance"
	NoticeDowntime      NoticeType = "downtime"
	NoticeAnnouncement  NoticeType = "announcement"
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

// Broadcaster emits events to connected clients.
type Broadcaster interface {
	BroadcastAll(*transport.Envelope)
}

// Manager owns system notices.
type Manager struct {
	db          *sql.DB
	cfg         *config.Config
	broadcaster Broadcaster
	pauser      MatchmakingPauser
}

// NewManager returns a notice manager.
func NewManager(db *sql.DB, cfg *config.Config, pauser MatchmakingPauser) *Manager {
	var broadcaster Broadcaster
	if b, ok := pauser.(Broadcaster); ok {
		broadcaster = b
	}
	return &Manager{db: db, cfg: cfg, broadcaster: broadcaster, pauser: pauser}
}

// CreateNotice inserts a notice. If the notice is already published and not
// withdrawn, it is broadcast to connected clients as a system_notice event.
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
	if n.CreatedBy != nil {
		createdBy = *n.CreatedBy
	}

	_, err = m.db.ExecContext(ctx,
		`INSERT INTO system_notices (id, type, title, body, published_at, maintenance_start, maintenance_duration_min, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())`,
		id, string(n.Type), titleJSON, bodyJSON, n.PublishedAt, n.MaintenanceStart, n.MaintenanceDurationMin, createdBy,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert notice: %w", err)
	}

	n.ID = id
	if !n.PublishedAt.After(now) {
		m.broadcastNotice(n)
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
	ID                     uuid.UUID `json:"id"`
	Type                   string    `json:"type"`
	Title                  string    `json:"title"`
	Body                   string    `json:"body"`
	PublishedAt            *time.Time `json:"published_at,omitempty"`
	MaintenanceStart       *time.Time `json:"maintenance_start,omitempty"`
	MaintenanceDurationMin int       `json:"maintenance_duration_min,omitempty"`
}

func (m *Manager) ActiveNoticesForLocale(ctx context.Context, locale string) ([]LocalizedNotice, error) {
	if locale == "" {
		locale = m.cfg.Localization.DefaultLocale
	}
	active, err := m.ActiveNotices(ctx, time.Now().UTC())
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
		})
	}
	return out, nil
}

// WithdrawNotice marks a notice as withdrawn.
func (m *Manager) WithdrawNotice(ctx context.Context, id uuid.UUID) error {
	_, err := m.db.ExecContext(ctx,
		`UPDATE system_notices SET withdrawn_at = now(), updated_at = now() WHERE id = $1`,
		id)
	if err != nil {
		return fmt.Errorf("withdraw notice: %w", err)
	}
	return nil
}

// MarkMaintenanceDrain pauses matchmaking while a future or active maintenance
// notice exists, and resumes it otherwise.
func (m *Manager) MarkMaintenanceDrain(ctx context.Context) error {
	if m.pauser == nil {
		return nil
	}
	rows, err := m.db.QueryContext(ctx,
		`SELECT maintenance_start, maintenance_duration_min FROM system_notices
		 WHERE type = 'maintenance'
		   AND published_at IS NOT NULL AND published_at <= now()
		   AND (withdrawn_at IS NULL OR withdrawn_at > now())`)
	if err != nil {
		return fmt.Errorf("query maintenance notices: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	shouldPause := false
	for rows.Next() {
		var start sql.NullTime
		var durationMin int
		if err := rows.Scan(&start, &durationMin); err != nil {
			continue
		}
		if !start.Valid {
			continue
		}
		end := start.Time.Add(time.Duration(durationMin) * time.Minute)
		if now.Before(start.Time) || (now.Equal(start.Time) || now.After(start.Time) && now.Before(end)) {
			shouldPause = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	m.pauser.SetReady(!shouldPause)
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

func (m *Manager) broadcastNotice(n Notice) {
	if m.broadcaster == nil {
		return
	}
	payload := map[string]any{
		"id":   n.ID.String(),
		"type": string(n.Type),
	}
	if len(n.Title) > 0 {
		payload["title"] = pickLocale(n.Title, m.cfg.Localization.DefaultLocale, "en")
	}
	m.broadcaster.BroadcastAll(transport.NewEvent(transport.EventSystemNotice, payload))
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
