package reports

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

type Case struct {
	ID                                                                                            uuid.UUID
	Kind, Status, TargetAccount, TargetMediaID, Reason, Description, Resolution, ResolutionReason string
	TextTarget                                                                                    *TextTarget
	Text                                                                                          string
	Reports                                                                                       int
	CreatedAt                                                                                     time.Time
}

func pageArgs(after string, limit int) (any, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	if after == "" {
		return nil, nil
	}
	if _, err := uuid.Parse(after); err != nil {
		return nil, ErrInvalid
	}
	return after, nil
}
func (m *Manager) ListCases(ctx context.Context, status, after string, limit int) ([]Case, string, error) {
	return m.listCases(ctx, status, "", after, limit)
}
func (m *Manager) ListCaseQueue(ctx context.Context, queue, after string, limit int) ([]Case, string, error) {
	return m.listCases(ctx, "", queue, after, limit)
}
func (m *Manager) listCases(ctx context.Context, status, queue, after string, limit int) ([]Case, string, error) {
	if queue != "" && queue != "conduct" && queue != "curation" {
		return nil, "", ErrInvalid
	}
	cursor, err := pageArgs(after, limit)
	if err != nil {
		return nil, "", err
	}
	if status != "" && status != "new" && status != "in_review" && status != "resolved" {
		return nil, "", ErrInvalid
	}
	rows, err := m.db.QueryContext(ctx, `SELECT c.id,c.kind,c.status,COALESCE(c.target_account_id::text,''),COALESCE(c.target_media_id,''),c.text_target,
 COALESCE(c.resolution,''),COALESCE(c.resolution_reason,''),c.created_at,
 (SELECT count(*) FROM reports r WHERE r.case_id=c.id),
 COALESCE((SELECT left(reason,256) FROM reports r WHERE r.case_id=c.id ORDER BY created_at,id LIMIT 1),''),
 COALESCE((SELECT left(description,4096) FROM reports r WHERE r.case_id=c.id ORDER BY created_at,id LIMIT 1),'')
 FROM report_cases c WHERE ($1='' OR c.status=$1) AND ($4='' OR ($4='conduct' AND c.kind='conduct') OR ($4='curation' AND c.kind<>'conduct')) AND ($2::uuid IS NULL OR (c.created_at,c.id)<(SELECT created_at,id FROM report_cases WHERE id=$2)) ORDER BY c.created_at DESC,c.id DESC LIMIT $3`, status, cursor, limit, queue)
	if err != nil {
		return nil, "", err
	}
	out := []Case{}
	for rows.Next() {
		var c Case
		var raw []byte
		if err = rows.Scan(&c.ID, &c.Kind, &c.Status, &c.TargetAccount, &c.TargetMediaID, &raw, &c.Resolution, &c.ResolutionReason, &c.CreatedAt, &c.Reports, &c.Reason, &c.Description); err != nil {
			rows.Close()
			return nil, "", err
		}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &c.TextTarget); err != nil {
				rows.Close()
				return nil, "", err
			}
		}
		out = append(out, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	for i := range out {
		if out[i].TextTarget != nil {
			out[i].Text, err = m.caseText(ctx, *out[i].TextTarget)
			if err != nil {
				return nil, "", err
			}
		}
	}
	next := ""
	if len(out) == limit {
		next = out[len(out)-1].ID.String()
	}
	return out, next, nil
}
func (m *Manager) caseText(ctx context.Context, target TextTarget) (string, error) {
	var text string
	err := m.db.QueryRowContext(ctx, `SELECT member->>'text' FROM text_releases r CROSS JOIN LATERAL jsonb_array_elements((r.bundle->'nowns')||(r.bundle->'cards')) member WHERE r.release_id=$1 AND r.language=$2 AND r.rules_version=$3 AND member->>'id'=$4 AND member->>'revision'=$5 LIMIT 1`, target.PackReleaseID, target.Language, target.RulesVersion, target.ContentRef.ContentID, target.ContentRef.Revision).Scan(&text)
	sum := sha256.Sum256([]byte(text))
	if err != nil || hex.EncodeToString(sum[:]) != target.TextSHA256 {
		return "", ErrTargetUnavailable
	}
	return text, nil
}

// LockAdmin rechecks the current account and role inside a moderation transaction.
// It deliberately does not apply a Guard's matchmaking/portal-only freeze.
func LockAdmin(ctx context.Context, tx *sql.Tx, admin string) error {
	return store.LockAdminTx(ctx, tx, admin, []string{"admin", "superadmin"})
}

type Takedown func(context.Context, *sql.Tx, string, string, string) error

// ResolveCase commits one immutable decision, release withdrawal and audit. The
// release lock precedes the case lock, matching standalone release operations.
func (m *Manager) ResolveCase(ctx context.Context, admin, id, decision, reason string, takedown Takedown) error {
	if _, err := uuid.Parse(id); err != nil || !boundedText(reason, 1024, true) || (decision != "dismissed" && decision != "release_takedown") {
		return ErrInvalid
	}
	return store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		if err := LockAdmin(ctx, tx, admin); err != nil {
			return err
		}
		var kind string
		var raw []byte
		if err := tx.QueryRowContext(ctx, `SELECT kind,text_target FROM report_cases WHERE id=$1`, id).Scan(&kind, &raw); err != nil {
			return ErrTargetUnavailable
		}
		if decision == "release_takedown" {
			if kind != "text" || takedown == nil {
				return ErrInvalid
			}
			var target TextTarget
			if err := json.Unmarshal(raw, &target); err != nil {
				return err
			}
			if err := takedown(ctx, tx, admin, target.PackReleaseID, reason); err != nil {
				return err
			}
		}
		var old, oldReason, oldAdmin string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(resolution,''),COALESCE(resolution_reason,''),COALESCE(resolved_by::text,'') FROM report_cases WHERE id=$1 FOR UPDATE`, id).Scan(&old, &oldReason, &oldAdmin); err != nil {
			return err
		}
		if old != "" {
			if old == decision && oldReason == reason && oldAdmin == admin {
				return LockAdmin(ctx, tx, admin)
			}
			return fmt.Errorf("report.resolution_conflict")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE report_cases SET status='resolved',resolution=$2,resolution_reason=$3,resolved_by=$4,resolved_at=now(),updated_at=now() WHERE id=$1`, id, decision, reason, admin); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE reports SET status='resolved',updated_at=now() WHERE case_id=$1`, id); err != nil {
			return err
		}
		detail, _ := json.Marshal(map[string]string{"decision": decision, "reason": reason})
		_, err := tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,after_state) VALUES($1,'report_case_resolve','report_case',$2,$3)`, admin, id, string(detail))
		if err != nil {
			return err
		}
		return LockAdmin(ctx, tx, admin)
	})
}
