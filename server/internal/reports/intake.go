package reports

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

const MaxBodyBytes = 16384
const MaxDescriptionBytes = 4096

var ErrTargetUnavailable = errors.New("report.target_unavailable")
var ErrInvalid = errors.New("report.invalid")
var ErrRateLimited = errors.New("report.rate_limited")

// TextTargetRequest carries only a visible reference. All stored context is
// resolved by the authoritative recipient projection, before catalog lookup.
type TextTargetRequest struct {
	MatchID    string        `json:"match_id"`
	ContentRef v2.ContentRef `json:"content_ref"`
}
type VisibleText func(context.Context, string, string, v2.ContentRef) (v2.MatchContract, v2.TextContent, error)
type TextTarget struct {
	MatchID       string              `json:"match_id"`
	ModeID        gamecontract.ModeID `json:"mode_id"`
	Language      string              `json:"content_language"`
	RulesVersion  string              `json:"rules_version"`
	PackReleaseID string              `json:"pack_release_id"`
	ContentRef    v2.ContentRef       `json:"content_ref"`
	TextSHA256    string              `json:"text_sha256"`
}

func boundedText(s string, max int, required bool) bool {
	if !utf8.ValidString(s) || len(s) > max || (required && strings.TrimSpace(s) == "") {
		return false
	}
	for _, r := range s {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || r == 0x202e || r == 0x202d || r == 0x202a || r == 0x202b || r == 0x202c {
			return false
		}
	}
	return true
}
func digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func (m *Manager) CreateTextReport(ctx context.Context, reporter string, req TextTargetRequest, reason, description string, visible VisibleText) error {
	if visible == nil || req.MatchID == "" || req.ContentRef.ContentID == "" || req.ContentRef.Revision == 0 {
		return ErrTargetUnavailable
	}
	contract, content, err := visible(ctx, reporter, req.MatchID, req.ContentRef)
	if err != nil || contract.MatchID != req.MatchID || content.ContentRef != req.ContentRef {
		return ErrTargetUnavailable
	}
	// The projection is a permission check, not a release existence oracle. Pinned
	// withdrawn releases remain reportable by a player still seeing their content.
	var text string
	err = m.db.QueryRowContext(ctx, `SELECT member->>'text' FROM text_releases r CROSS JOIN LATERAL jsonb_array_elements((r.bundle->'nowns')||(r.bundle->'cards')) member
 WHERE r.release_id=$1 AND r.language=$2 AND r.rules_version=$3 AND r.snapshot_sha256=$4
 AND member->>'id'=$5 AND member->>'revision'=$6 AND (member->'modes') ? $7 LIMIT 1`, contract.PackReleaseID, contract.ContentLanguage, contract.RulesVersion, contract.PackSHA256, req.ContentRef.ContentID, req.ContentRef.Revision, string(contract.ModeID)).Scan(&text)
	if err != nil || text != content.Text {
		return ErrTargetUnavailable
	}
	hash := sha256.Sum256([]byte(text))
	target := TextTarget{contract.MatchID, contract.ModeID, contract.ContentLanguage, contract.RulesVersion, contract.PackReleaseID, content.ContentRef, hex.EncodeToString(hash[:])}
	key := "text:" + digest([]any{target.Language, target.PackReleaseID, target.ContentRef, target.TextSHA256})
	return m.insertReport(ctx, reporter, ReportMedia, "", "", reason, description, "text", key, &target)
}
func (m *Manager) insertReport(ctx context.Context, reporter string, typ ReportType, account, legacy, reason, description, kind, key string, target *TextTarget) error {
	if !boundedText(reason, 256, true) || !boundedText(description, MaxDescriptionBytes, false) {
		return ErrInvalid
	}
	if reporter != "" {
		if _, err := uuid.Parse(reporter); err != nil {
			return ErrInvalid
		}
	}
	bodyHash := digest([]any{reporter, typ, account, legacy, reason, description, target})
	raw, _ := json.Marshal(target)
	var targetJSON any
	if target != nil {
		targetJSON = string(raw)
	}
	bucket := reporter
	if bucket == "" {
		bucket = "anonymous"
	}
	return store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO report_rate_limits(bucket) VALUES($1) ON CONFLICT DO NOTHING`, bucket); err != nil {
			return err
		}
		var last sql.NullTime
		if err := tx.QueryRowContext(ctx, `SELECT last_at FROM report_rate_limits WHERE bucket=$1 FOR UPDATE`, bucket).Scan(&last); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM reports WHERE COALESCE(reporter_id::text,'anonymous')=$1 AND submission_sha256=$2)`, bucket, bodyHash).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return nil
		}
		var now time.Time
		if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		if last.Valid && now.Sub(last.Time) < time.Minute {
			return ErrRateLimited
		}
		var caseID, status string
		if err := tx.QueryRowContext(ctx, `INSERT INTO report_cases(case_key,kind,target_account_id,target_media_id,text_target) VALUES($1,$2,$3,$4,$5) ON CONFLICT(case_key) DO UPDATE SET case_key=EXCLUDED.case_key RETURNING id,status`, key, kind, nullable(account), nullable(legacy), targetJSON).Scan(&caseID, &status); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO reports(report_type,reporter_id,target_account_id,target_media_id,reason,description,case_id,text_target,submission_sha256,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, typ, nullable(reporter), nullable(account), nullable(legacy), reason, description, caseID, targetJSON, bodyHash, status); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE report_rate_limits SET last_at=$2 WHERE bucket=$1`, bucket, now)
		return err
	})
}
