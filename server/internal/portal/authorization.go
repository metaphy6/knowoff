package portal

import (
	"context"
	"database/sql"
	"errors"

	"github.com/knowoff/knowoff/server/pkg/media"
)

// Guard/admin state and role revocation serialize with every portal mutation.
// Contribution consent is checked separately and cannot satisfy user terms.
func (m *Manager) authorizePortalWriteTx(ctx context.Context, tx *sql.Tx, account string, role Role, authored bool) error {
	if err := lockPortalAccounts(ctx, tx, account); err != nil {
		return err
	}
	if err := portalActorAllowedTx(ctx, tx, account, m.now()); err != nil {
		return err
	}
	if role != "" {
		var has bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM portal_roles WHERE account_id=$1 AND revoked_at IS NULL AND (role=$2 OR ($2='contributor' AND role='curator')))`, account, string(role)).Scan(&has); err != nil {
			return err
		}
		if !has {
			return errors.New("contributor role required")
		}
	}
	if authored {
		version := m.cfg.Trust.UserTermsVersion
		if version == "" {
			return errors.New("current user terms unavailable")
		}
		var accepted bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_terms_acceptances a JOIN user_terms_versions v ON v.version=a.version WHERE a.account_id=$1 AND a.version=$2 AND v.active_from<=$3)`, account, version, m.now()).Scan(&accepted); err != nil {
			return err
		}
		if !accepted {
			return errors.New("current user terms acceptance required")
		}
	}
	return nil
}

func (m *Manager) normalizedContribution(raw string) (string, error) {
	return media.NormalizeText(raw, m.MaxTextBytes())
}
