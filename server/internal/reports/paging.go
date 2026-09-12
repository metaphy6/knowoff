package reports

import (
	"context"
	"database/sql"
	"encoding/json"
)

func (m *Manager) ListReportsPage(ctx context.Context, status, after, caseID string, limit int) ([]Report, string, error) {
	cursor, err := pageArgs(after, limit)
	if err != nil {
		return nil, "", err
	}
	caseCursor, err := pageArgs(caseID, limit)
	if err != nil {
		return nil, "", err
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id,report_type,reporter_id,target_account_id,target_media_id,left(reason,256),left(COALESCE(description,''),4096),status,created_at FROM reports WHERE ($1='' OR status=$1) AND ($2::uuid IS NULL OR (created_at,id)<(SELECT created_at,id FROM reports WHERE id=$2)) AND ($3::uuid IS NULL OR case_id=$3) ORDER BY created_at DESC,id DESC LIMIT $4`, status, cursor, caseCursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Report{}
	for rows.Next() {
		var r Report
		var reporter, account, media sql.NullString
		if err = rows.Scan(&r.ID, &r.ReportType, &reporter, &account, &media, &r.Reason, &r.Description, &r.Status, &r.CreatedAt); err != nil {
			return nil, "", err
		}
		if reporter.Valid {
			r.ReporterID = &reporter.String
		}
		if account.Valid {
			r.TargetAccount = &account.String
		}
		if media.Valid {
			r.TargetMediaID = &media.String
		}
		out = append(out, r)
	}
	next := ""
	if len(out) == limit {
		next = out[len(out)-1].ID.String()
	}
	return out, next, rows.Err()
}
func (m *Manager) ListFeedbackPage(ctx context.Context, status, after string, limit int) ([]Feedback, string, error) {
	cursor, err := pageArgs(after, limit)
	if err != nil {
		return nil, "", err
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id,account_id,type,left(COALESCE(title,''),256),left(message,4096),context_snapshot,status,created_at FROM feedback WHERE ($1='' OR status=$1) AND ($2::uuid IS NULL OR (created_at,id)<(SELECT created_at,id FROM feedback WHERE id=$2)) ORDER BY created_at DESC,id DESC LIMIT $3`, status, cursor, limit)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Feedback{}
	for rows.Next() {
		var f Feedback
		var account sql.NullString
		var raw []byte
		if err = rows.Scan(&f.ID, &account, &f.Type, &f.Title, &f.Message, &raw, &f.Status, &f.CreatedAt); err != nil {
			return nil, "", err
		}
		if account.Valid {
			f.AccountID = &account.String
		}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &f.ContextSnapshot); err != nil {
				return nil, "", err
			}
		}
		out = append(out, f)
	}
	next := ""
	if len(out) == limit {
		next = out[len(out)-1].ID.String()
	}
	return out, next, rows.Err()
}
