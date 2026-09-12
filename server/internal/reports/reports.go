// Package reports implements player reports and feedback.
package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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
}

// NewManager returns a reports manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
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
	kind, key := "", ""
	switch reportType {
	case ReportConduct:
		if _, err := uuid.Parse(targetAccountID); err != nil || targetMediaID != "" {
			return ErrInvalid
		}
		kind, key = "conduct", "conduct:"+targetAccountID
	case ReportMedia:
		if !boundedText(targetMediaID, 256, true) || targetAccountID != "" {
			return ErrInvalid
		}
		kind, key = "legacy_media", "legacy:"+targetMediaID
	default:
		return ErrInvalid
	}
	return m.insertReport(ctx, reporterID, reportType, targetAccountID, targetMediaID, reason, description, kind, key, nil)
}

// CreateFeedback stores a feedback form submission.
func (m *Manager) CreateFeedback(ctx context.Context, accountID, typ, title, message string, contextSnapshot map[string]any) error {
	if !boundedText(message, MaxDescriptionBytes, true) || !boundedText(title, 256, false) {
		return fmt.Errorf("message required")
	}
	if typ != "bug" && typ != "idea" && typ != "other" {
		return fmt.Errorf("invalid feedback type")
	}
	for key, value := range contextSnapshot {
		if key != "version" && key != "app_version" && key != "pack_tag" && key != "last_match_id" {
			return ErrInvalid
		}
		text, ok := value.(string)
		if !ok || !boundedText(text, 256, false) {
			return ErrInvalid
		}
	}
	ctxJSON, err := json.Marshal(contextSnapshot)
	if err != nil {
		return ErrInvalid
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

// ListReports returns the first bounded page, optionally filtered by status.
func (m *Manager) ListReports(ctx context.Context, status string) ([]Report, error) {
	rows, _, err := m.ListReportsPage(ctx, status, "", "", 100)
	return rows, err
}
func (m *Manager) ListFeedback(ctx context.Context, status string) ([]Feedback, error) {
	rows, _, err := m.ListFeedbackPage(ctx, status, "", 100)
	return rows, err
}
