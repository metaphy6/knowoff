// Package reports implements player reports and feedback.
package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ReportType distinguishes conduct reports from media reports.
type ReportType string

const (
	ReportConduct ReportType = "conduct"
	ReportMedia   ReportType = "media"
)

// Manager owns the reports and feedback tables.
type Manager struct {
	db *sql.DB

	mu         sync.Mutex
	lastReport map[string]time.Time
}

// NewManager returns a reports manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db, lastReport: make(map[string]time.Time)}
}

// Report is a player-submitted report.
type Report struct {
	ID            uuid.UUID `json:"id"`
	ReportType    string    `json:"report_type"`
	ReporterID    *string   `json:"reporter_id,omitempty"`
	TargetAccount *string   `json:"target_account_id,omitempty"`
	TargetMediaID *string   `json:"target_media_id,omitempty"`
	Reason        string    `json:"reason"`
	Description   string    `json:"description,omitempty"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// Feedback is a player-submitted feedback form.
type Feedback struct {
	ID              uuid.UUID      `json:"id"`
	AccountID       *string        `json:"account_id,omitempty"`
	Type            string         `json:"type"`
	Title           string         `json:"title,omitempty"`
	Message         string         `json:"message"`
	ContextSnapshot map[string]any `json:"context_snapshot,omitempty"`
	Status          string         `json:"status"`
	CreatedAt       time.Time      `json:"created_at"`
}

// CreateReport stores a player report. Reports are rate-limited to one per
// reporter per minute (anonymous reports share one bucket keyed by "anonymous").
func (m *Manager) CreateReport(ctx context.Context, reporterID string, reportType ReportType, targetAccountID, targetMediaID, reason, description string) error {
	if reason == "" {
		return fmt.Errorf("reason required")
	}
	if reportType != ReportConduct && reportType != ReportMedia {
		return fmt.Errorf("invalid report type")
	}
	if reportType == ReportConduct && targetAccountID == "" {
		return fmt.Errorf("target account required")
	}
	if reportType == ReportMedia && targetMediaID == "" {
		return fmt.Errorf("target media required")
	}

	bucket := reporterID
	if bucket == "" {
		bucket = "anonymous"
	}
	m.mu.Lock()
	if last, ok := m.lastReport[bucket]; ok && time.Since(last) < time.Minute {
		m.mu.Unlock()
		return fmt.Errorf("rate limited")
	}
	m.lastReport[bucket] = time.Now().UTC()
	m.mu.Unlock()

	id := uuid.New()
	var reporter interface{}
	if reporterID != "" {
		reporter = reporterID
	}
	var targetAcc interface{}
	if targetAccountID != "" {
		targetAcc = targetAccountID
	}
	var targetMedia interface{}
	if targetMediaID != "" {
		targetMedia = targetMediaID
	}

	_, err := m.db.ExecContext(ctx,
		`INSERT INTO reports (id, report_type, reporter_id, target_account_id, target_media_id, reason, description, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'new', now(), now())`,
		id, string(reportType), reporter, targetAcc, targetMedia, reason, description,
	)
	if err != nil {
		return fmt.Errorf("insert report: %w", err)
	}
	return nil
}

// CreateFeedback stores a feedback form submission.
func (m *Manager) CreateFeedback(ctx context.Context, accountID, typ, title, message string, contextSnapshot map[string]any) error {
	if message == "" {
		return fmt.Errorf("message required")
	}
	if typ != "bug" && typ != "idea" && typ != "other" {
		return fmt.Errorf("invalid feedback type")
	}
	ctxJSON, err := json.Marshal(contextSnapshot)
	if err != nil {
		ctxJSON = []byte("{}")
	}
	id := uuid.New()
	var acc interface{}
	if accountID != "" {
		acc = accountID
	}
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO feedback (id, account_id, type, title, message, context_snapshot, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, 'new', now(), now())`,
		id, acc, typ, title, message, ctxJSON,
	)
	if err != nil {
		return fmt.Errorf("insert feedback: %w", err)
	}
	return nil
}

// ListReports returns reports, optionally filtered by status.
func (m *Manager) ListReports(ctx context.Context, status string) ([]Report, error) {
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = m.db.QueryContext(ctx,
			`SELECT id, report_type, reporter_id, target_account_id, target_media_id, reason, description, status, created_at
			 FROM reports ORDER BY created_at DESC`)
	} else {
		rows, err = m.db.QueryContext(ctx,
			`SELECT id, report_type, reporter_id, target_account_id, target_media_id, reason, description, status, created_at
			 FROM reports WHERE status = $1 ORDER BY created_at DESC`,
			status)
	}
	if err != nil {
		return nil, fmt.Errorf("query reports: %w", err)
	}
	defer rows.Close()

	var out []Report
	for rows.Next() {
		var r Report
		var reporter, targetAcc, targetMedia sql.NullString
		if err := rows.Scan(&r.ID, &r.ReportType, &reporter, &targetAcc, &targetMedia, &r.Reason, &r.Description, &r.Status, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		if reporter.Valid {
			r.ReporterID = &reporter.String
		}
		if targetAcc.Valid {
			r.TargetAccount = &targetAcc.String
		}
		if targetMedia.Valid {
			r.TargetMediaID = &targetMedia.String
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListFeedback returns feedback, optionally filtered by status.
func (m *Manager) ListFeedback(ctx context.Context, status string) ([]Feedback, error) {
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = m.db.QueryContext(ctx,
			`SELECT id, account_id, type, title, message, context_snapshot, status, created_at
			 FROM feedback ORDER BY created_at DESC`)
	} else {
		rows, err = m.db.QueryContext(ctx,
			`SELECT id, account_id, type, title, message, context_snapshot, status, created_at
			 FROM feedback WHERE status = $1 ORDER BY created_at DESC`,
			status)
	}
	if err != nil {
		return nil, fmt.Errorf("query feedback: %w", err)
	}
	defer rows.Close()

	var out []Feedback
	for rows.Next() {
		var f Feedback
		var acc sql.NullString
		var ctxSnapshot []byte
		if err := rows.Scan(&f.ID, &acc, &f.Type, &f.Title, &f.Message, &ctxSnapshot, &f.Status, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan feedback: %w", err)
		}
		if acc.Valid {
			f.AccountID = &acc.String
		}
		if len(ctxSnapshot) > 0 {
			_ = json.Unmarshal(ctxSnapshot, &f.ContextSnapshot)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
