package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

func cutoverSchemaDB(t *testing.T) *sql.DB {
	t.Helper()
	db := transitionDesignDB(t, 25, false)
	if err := MigrateUp(db, transitionMigrationPath(t, 26)); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := db.QueryRow(`SELECT to_regclass('public.cutover_instances') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatal("durable cutover schema missing", err)
	}
	return db
}

func cutoverSource(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(`INSERT INTO cutover_instances(id,cluster_system_identifier,database_oid,database_name)
SELECT $1,system_identifier,d.oid,d.datname FROM pg_control_system(),pg_database d WHERE d.datname=current_database()`, id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func cutoverRequest(t *testing.T, db *sql.DB, instance string, generation int, predecessor any) string {
	t.Helper()
	id := uuid.NewString()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO cutover_requests(id,instance_id,generation,predecessor_id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at)
VALUES($1,$2,$3,$4,'[{"name":"runtime","oid":123},{"name":"migrator","oid":124}]',repeat('a',64),repeat('b',64),repeat('c',64),repeat('d',64),decode(repeat('e',64),'hex'),clock_timestamp(),clock_timestamp()+interval '5 minutes')`, id, instance, generation, predecessor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE cutover_instances SET phase='closing',generation=$3,current_request=$2 WHERE id=$1`, instance, id, generation); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return id
}

func cutoverWatermark(t *testing.T, db *sql.DB, instance, request string, generation int) string {
	t.Helper()
	id := uuid.NewString()
	_, err := db.Exec(`INSERT INTO cutover_watermarks(id,instance_id,generation,request_id,wal_lsn,evidence,evidence_sha256)
VALUES($1,$2,$3,$4,pg_current_wal_lsn(),'{"synthetic":true,"pending":0}',encode(sha256(convert_to('{"synthetic":true,"pending":0}'::jsonb::text,'UTF8')),'hex'))`, id, instance, generation, request)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func cutoverRefuse(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	_, err := db.Exec(query, args...)
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "P0001" && pg.Code.Class() != "23" {
		t.Fatal("unsafe cutover state accepted", query)
	}
}

func TestCutoverSchemaStartsEmptyAndPreservesHead25(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db := transitionDesignDB(t, 25, false)
	account := uuid.NewString()
	for _, q := range []string{
		`INSERT INTO accounts(id,nickname) VALUES($1,'cutover-legacy')`,
		`INSERT INTO noin_wallets(account_id,balance) VALUES($1,73)`,
		`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'fixture',73,'retained',CURRENT_DATE)`,
		`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'premium','legacy')`,
		`INSERT INTO store_purchases(account_id,platform,product_id,transaction_id,raw_receipt) VALUES($1,'app_store','fixture',$1::uuid::text,'{"private":"retained receipt"}')`,
	} {
		if _, err := db.Exec(q, account); err != nil {
			t.Fatal(err)
		}
	}
	before, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, transitionMigrationPath(t, 26)); err != nil {
		t.Fatal(err)
	}
	after, err := ReadTransitionPreflight(ctx, db)
	if err != nil || after.MigrationVersion == nil || *after.MigrationVersion != 26 {
		t.Fatal("cutover migration missing", err)
	}
	for _, old := range before.Tables {
		if old.Name == "schema_migrations" {
			continue
		}
		found := false
		for _, now := range after.Tables {
			if old.Name == now.Name {
				found = reflect.DeepEqual(old, now)
			}
		}
		if !found {
			t.Fatal("migration changed retained table", old.Name)
		}
	}
	for _, table := range []string{"cutover_instances", "cutover_requests", "cutover_watermarks", "cutover_handoffs"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + pq.QuoteIdentifier(table)).Scan(&n); err != nil || n != 0 {
			t.Fatal("migration created authority", table, n, err)
		}
	}
	if err := runMigration(db, transitionMigrationPath(t, 26), "down", func(m *migrate.Migrate) error { return m.Steps(-1) }); err != nil {
		t.Fatal("empty exact down", err)
	}
	restored, err := ReadTransitionPreflight(ctx, db)
	if err != nil || !reflect.DeepEqual(before.Tables, restored.Tables) {
		t.Fatal("empty down changed retained rows", err)
	}
}

func TestCutoverSchemaHandoffBindsCurrentSealedGenerationOnce(t *testing.T) {
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	request := cutoverRequest(t, db, instance, 1, nil)
	watermark := cutoverWatermark(t, db, instance, request, 1)
	query := `INSERT INTO cutover_handoffs(id,instance_id,generation,request_id,watermark_id,target_instance_id,target_cluster_system_identifier,target_database_oid,target_database_name) VALUES($1,$2,1,$3,$4,$5,2,2,'declared_target')`
	target, handoff := uuid.NewString(), uuid.NewString()
	cutoverRefuse(t, db, query, handoff, instance, request, watermark, target)
	if _, err := db.Exec(`UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(query, handoff, instance, request, watermark, target); err != nil {
		t.Fatal("current sealed handoff", err)
	}
	cutoverRefuse(t, db, query, uuid.NewString(), instance, request, watermark, uuid.NewString())
	cutoverRefuse(t, db, `UPDATE cutover_handoffs SET target_instance_id=$1`, uuid.NewString())
	cutoverRefuse(t, db, `DELETE FROM cutover_handoffs`)
	cutoverRefuse(t, db, `TRUNCATE cutover_handoffs`)
	cutoverRefuse(t, db, `INSERT INTO cutover_instances(id,cluster_system_identifier,database_oid,database_name,parent_instance_id,parent_watermark_id) VALUES($1,2,2,'declared_target',$2,$3)`, target, instance, watermark)
	// A transfer is terminal for this source; a later request cannot invalidate
	// its chosen target and create another handoff under a newer watermark.
	cutoverRefuse(t, db, `INSERT INTO cutover_requests(id,instance_id,generation,predecessor_id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at) SELECT $1,instance_id,2,id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at FROM cutover_requests WHERE id=$2`, uuid.NewString(), request)
}

func TestCutoverSchemaBindsPhysicalIdentityAndClosedRecovery(t *testing.T) {
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	cutoverRefuse(t, db, `INSERT INTO cutover_instances(id,cluster_system_identifier,database_oid,database_name) SELECT $1,cluster_system_identifier,database_oid,database_name FROM cutover_instances`, uuid.NewString())
	cutoverRefuse(t, db, `INSERT INTO cutover_instances(id,cluster_system_identifier,database_oid,database_name) VALUES($1,1,1,'forged')`, uuid.NewString())
	request := cutoverRequest(t, db, instance, 1, nil)
	cutoverRefuse(t, db, `UPDATE cutover_instances SET phase='closing',generation=1,current_request=$2 WHERE id=$1`, instance, uuid.NewString())
	cutoverRefuse(t, db, `UPDATE cutover_instances SET phase='sealed',generation=1,current_request=$2 WHERE id=$1`, instance, request)
	cutoverRefuse(t, db, `UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance)
	cutoverWatermark(t, db, instance, request, 1)
	if _, err := db.Exec(`UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance); err != nil {
		t.Fatal(err)
	}
	cutoverRefuse(t, db, `UPDATE cutover_instances SET phase='ready',generation=0,current_request=NULL WHERE id=$1`, instance)
	cutoverRequest(t, db, instance, 2, request)
	cutoverRefuse(t, db, `UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance)
	cutoverRefuse(t, db, `UPDATE cutover_instances SET generation=1,current_request=$2 WHERE id=$1`, instance, request)
	var phase string
	var generation int
	if err := db.QueryRow(`SELECT phase,generation FROM cutover_instances WHERE id=$1`, instance).Scan(&phase, &generation); err != nil || phase != "closing" || generation != 2 {
		t.Fatal("stale attempt reopened or changed recovery", phase, generation, err)
	}
}

func TestCutoverSchemaRejectsMalformedAndImmutableEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	request := cutoverRequest(t, db, instance, 1, nil)
	watermark := cutoverWatermark(t, db, instance, request, 1)
	for _, q := range []string{
		`UPDATE cutover_requests SET lease_sha256=decode(repeat('a',64),'hex')`,
		`UPDATE cutover_watermarks SET evidence='{}'`,
		`UPDATE cutover_instances SET database_name='renamed'`,
		`DELETE FROM cutover_requests`, `DELETE FROM cutover_watermarks`, `DELETE FROM cutover_instances`,
		`TRUNCATE cutover_requests CASCADE`, `TRUNCATE cutover_instances CASCADE`,
		`UPDATE cutover_instances SET generation=9223372036854775807`,
	} {
		cutoverRefuse(t, db, q)
	}
	cutoverRefuse(t, db, `INSERT INTO cutover_watermarks(id,instance_id,generation,request_id,wal_lsn,evidence,evidence_sha256) VALUES($1,$2,2,$3,'0/1','{}',repeat('a',64))`, uuid.NewString(), instance, request)
	if _, err := db.Exec(`INSERT INTO cutover_watermarks SELECT * FROM cutover_watermarks WHERE id=$1 ON CONFLICT(id) DO NOTHING`, watermark); err != nil {
		t.Fatal("identical immutable retry", err)
	}
	body, err := os.ReadFile("../../migrations/000026_cutover_authority.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	before, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	cutoverRefuse(t, db, string(body))
	// Explicit transaction in the rejected down must not poison the pooled session.
	if _, err := db.Exec(`ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	after, err := ReadTransitionPreflight(ctx, db)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("retained authority changed on down refusal", err)
	}
}

func TestCutoverSchemaRejectsLeaseAndWriterShapes(t *testing.T) {
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	base := `INSERT INTO cutover_requests(id,instance_id,generation,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at) VALUES($1,$2,1,$3,repeat('a',64),repeat('b',64),repeat('c',64),repeat('d',64),$4,$5,$6)`
	at := time.Now().UTC()
	for _, row := range []struct {
		roles           string
		digest          []byte
		issued, expires time.Time
	}{
		{`null`, make([]byte, 32), at, at.Add(time.Minute)},
		{`[]`, make([]byte, 32), at, at.Add(time.Minute)},
		{`[{"name":"runtime","oid":123},{"name":"runtime","oid":124}]`, make([]byte, 32), at, at.Add(time.Minute)},
		{`[{"name":"runtime","oid":123}]`, []byte{1}, at, at.Add(time.Minute)},
		{`[{"name":"runtime","oid":123}]`, make([]byte, 32), at, at},
		{`[{"name":"runtime","oid":123}]`, make([]byte, 32), at, at.Add(2 * time.Hour)},
		{`[{"name":"` + strings.Repeat("x", 64) + `","oid":123}]`, make([]byte, 32), at, at.Add(time.Minute)},
	} {
		// Reject the malformed INSERT itself, before deferred request binding;
		// an orphan-commit failure would not prove any of these shape checks.
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(base, uuid.NewString(), instance, row.roles, row.digest, row.issued, row.expires)
		tx.Rollback()
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "P0001" && pg.Code.Class() != "23" {
			t.Fatal("invalid request shape not rejected at insertion", row.roles, err)
		}
	}
}

func TestCutoverSchemaRejectsCommittedOrphanRequest(t *testing.T) {
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	cutoverRefuse(t, db, `INSERT INTO cutover_requests(id,instance_id,generation,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at)
VALUES($1,$2,1,'[{"name":"runtime","oid":123}]',repeat('a',64),repeat('b',64),repeat('c',64),repeat('d',64),decode(repeat('e',64),'hex'),clock_timestamp(),clock_timestamp()+interval '1 minute')`, uuid.NewString(), instance)
	var count, generation int
	if err := db.QueryRow(`SELECT count(*) FROM cutover_requests`).Scan(&count); err != nil || count != 0 {
		t.Fatal("orphan request survived rollback", count, err)
	}
	if err := db.QueryRow(`SELECT generation FROM cutover_instances WHERE id=$1`, instance).Scan(&generation); err != nil || generation != 0 {
		t.Fatal("orphan request consumed generation", generation, err)
	}
	request := cutoverRequest(t, db, instance, 1, nil)
	if _, err := db.Exec(`INSERT INTO cutover_requests SELECT * FROM cutover_requests WHERE id=$1 ON CONFLICT(id) DO NOTHING`, request); err != nil {
		t.Fatal("identical committed request replay", err)
	}
	cutoverRequest(t, db, instance, 2, request)
	if _, err := db.Exec(`INSERT INTO cutover_requests SELECT * FROM cutover_requests WHERE id=$1 ON CONFLICT(id) DO NOTHING`, request); err != nil {
		t.Fatal("historical identical request replay", err)
	}
}

func TestCutoverSchemaRestoredHandoffBindsActualTargetOnce(t *testing.T) {
	source := cutoverSchemaDB(t)
	instance := cutoverSource(t, source)
	request := cutoverRequest(t, source, instance, 1, nil)
	watermark := cutoverWatermark(t, source, instance, request, 1)
	if _, err := source.Exec(`UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance); err != nil {
		t.Fatal(err)
	}
	// The primary fixture has already checked DSN token and current_database.
	// Create only this owned random sibling on that same disposable cluster.
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	name := "knowoff_test_" + os.Getenv("KNOWOFF_TEST_DB_TOKEN") + "_target_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	if !cutoverName(name) {
		t.Fatal("invalid disposable target")
	}
	if _, err := source.Exec(`CREATE DATABASE ` + pq.QuoteIdentifier(name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := source.Exec(`DROP DATABASE ` + pq.QuoteIdentifier(name) + ` WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	})
	u.Path = "/" + name
	target, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { target.Close() })
	var actualName, cluster string
	var oid int64
	if err := target.QueryRow(`SELECT current_database(),system_identifier::text,d.oid::bigint FROM pg_control_system(),pg_database d WHERE d.datname=current_database()`).Scan(&actualName, &cluster, &oid); err != nil || actualName != name {
		t.Fatal("target identity check", err)
	}
	if err := MigrateUp(target, transitionMigrationPath(t, 26)); err != nil {
		t.Fatal(err)
	}
	targetID := uuid.NewString()
	if _, err := source.Exec(`INSERT INTO cutover_handoffs(id,instance_id,generation,request_id,watermark_id,target_instance_id,target_cluster_system_identifier,target_database_oid,target_database_name) VALUES($1,$2,1,$3,$4,$5,$6,$7,$8)`, uuid.NewString(), instance, request, watermark, targetID, cluster, oid, name); err != nil {
		t.Fatal(err)
	}
	// This isolated schema proof restores exact retained rows with the trusted
	// owner before enabling their triggers, as a schema/data restore does. It
	// does not exercise the production backup tool or certify quiescence.
	tables := []string{"cutover_instances", "cutover_requests", "cutover_watermarks", "cutover_handoffs"}
	for _, table := range tables {
		if _, err := target.Exec(`ALTER TABLE ` + pq.QuoteIdentifier(table) + ` DISABLE TRIGGER ALL`); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range tables {
		var row string
		if err := source.QueryRow(`SELECT row_to_json(t)::text FROM ` + pq.QuoteIdentifier(table) + ` t`).Scan(&row); err != nil {
			t.Fatal(err)
		}
		if _, err := target.Exec(`INSERT INTO `+pq.QuoteIdentifier(table)+` SELECT * FROM json_populate_record(NULL::`+pq.QuoteIdentifier(table)+`,$1)`, row); err != nil {
			t.Fatal(err)
		}
		var restored string
		if err := target.QueryRow(`SELECT row_to_json(t)::text FROM ` + pq.QuoteIdentifier(table) + ` t`).Scan(&restored); err != nil || restored != row {
			t.Fatal("restored row drift", table, err)
		}
	}
	for _, table := range tables {
		if _, err := target.Exec(`ALTER TABLE ` + pq.QuoteIdentifier(table) + ` ENABLE TRIGGER ALL`); err != nil {
			t.Fatal(err)
		}
	}
	insert := `INSERT INTO cutover_instances(id,cluster_system_identifier,database_oid,database_name,parent_instance_id,parent_watermark_id) VALUES($1,$2,$3,$4,$5,$6)`
	cutoverRefuse(t, target, insert, uuid.NewString(), cluster, oid, name, instance, watermark)
	cutoverRefuse(t, target, insert, targetID, cluster, oid, name, instance, uuid.NewString())
	cutoverRefuse(t, target, insert, targetID, cluster, oid, name, nil, nil)
	if _, err := target.Exec(insert, targetID, cluster, oid, name, instance, watermark); err != nil {
		t.Fatal("authorized physical target refused", err)
	}
	cutoverRefuse(t, target, insert, uuid.NewString(), cluster, oid, name, instance, watermark)
	cutoverRefuse(t, target, insert, targetID, cluster, oid, name, instance, watermark)
	cutoverRequest(t, target, targetID, 1, nil)
	var phase string
	if err := target.QueryRow(`SELECT phase FROM cutover_instances WHERE id=$1`, instance).Scan(&phase); err != nil || phase != "sealed" {
		t.Fatal("target activation changed restored source", phase, err)
	}
}

func TestCutoverSchemaRollbackAndStaleHandoffKeepRecoveryClosed(t *testing.T) {
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	request := cutoverRequest(t, db, instance, 1, nil)
	watermark := cutoverWatermark(t, db, instance, request, 1)
	if _, err := db.Exec(`UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO cutover_requests(id,instance_id,generation,predecessor_id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at) SELECT $1,instance_id,2,id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at FROM cutover_requests WHERE id=$2`, uuid.NewString(), request); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	recovery := cutoverRequest(t, db, instance, 2, request)
	cutoverRefuse(t, db, `INSERT INTO cutover_handoffs(id,instance_id,generation,request_id,watermark_id,target_instance_id,target_cluster_system_identifier,target_database_oid,target_database_name) VALUES($1,$2,1,$3,$4,$5,2,2,'stale_target')`, uuid.NewString(), instance, request, watermark, uuid.NewString())
	cutoverWatermark(t, db, instance, recovery, 2)
	if _, err := db.Exec(`UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance); err != nil {
		t.Fatal(err)
	}
	cutoverRefuse(t, db, `INSERT INTO cutover_handoffs(id,instance_id,generation,request_id,watermark_id,target_instance_id,target_cluster_system_identifier,target_database_oid,target_database_name) VALUES($1,$2,1,$3,$4,$5,2,2,'stale_target')`, uuid.NewString(), instance, request, watermark, uuid.NewString())
	var generation, count int
	if err := db.QueryRow(`SELECT generation,(SELECT count(*) FROM cutover_requests) FROM cutover_instances WHERE id=$1`, instance).Scan(&generation, &count); err != nil || generation != 2 || count != 2 {
		t.Fatal("rollback consumed authority or stale handoff changed recovery", generation, count, err)
	}
}

func TestCutoverSchemaConcurrentRecoveryChoosesOneGeneration(t *testing.T) {
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	request := cutoverRequest(t, db, instance, 1, nil)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	start, results := make(chan struct{}), make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback()
			id := uuid.NewString()
			_, err = tx.ExecContext(ctx, `INSERT INTO cutover_requests(id,instance_id,generation,predecessor_id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at) SELECT $1,instance_id,2,id,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at FROM cutover_requests WHERE id=$2`, id, request)
			if err == nil {
				_, err = tx.ExecContext(ctx, `UPDATE cutover_instances SET phase='closing',generation=2,current_request=$2 WHERE id=$1`, instance, id)
			}
			if err == nil {
				err = tx.Commit()
			}
			results <- err
		}()
	}
	close(start)
	passed := 0
	for range 2 {
		err := <-results
		if err == nil {
			passed++
			continue
		}
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "P0001" && pg.Code != "23505" {
			t.Fatal("unexpected concurrent refusal", err)
		}
	}
	var generation, count int
	if err := db.QueryRow(`SELECT generation,(SELECT count(*) FROM cutover_requests) FROM cutover_instances WHERE id=$1`, instance).Scan(&generation, &count); err != nil || passed != 1 || generation != 2 || count != 2 {
		t.Fatal("competing controllers committed inconsistent authority", passed, generation, count, err)
	}
}

func TestCutoverSchemaExpiredLeaseCannotSealOrRecordWatermark(t *testing.T) {
	db := cutoverSchemaDB(t)
	instance := cutoverSource(t, db)
	request := uuid.NewString()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO cutover_requests(id,instance_id,generation,writer_roles,schema_sha256,image_sha256,config_sha256,content_sha256,lease_sha256,issued_at,expires_at) VALUES($1,$2,1,'[{"name":"runtime","oid":123}]',repeat('a',64),repeat('b',64),repeat('c',64),repeat('d',64),decode(repeat('e',64),'hex'),clock_timestamp(),clock_timestamp()+interval '3 seconds')`, request, instance); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE cutover_instances SET phase='closing',generation=1,current_request=$2 WHERE id=$1`, instance, request); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	cutoverWatermark(t, db, instance, request, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM expires_at-clock_timestamp())+0.01)) FROM cutover_requests WHERE id=$1`, request); err != nil {
		t.Fatal(err)
	}
	cutoverRefuse(t, db, `UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance)
	// Remove no history: a new ID must hit the expired authority guard itself,
	// before the per-request watermark uniqueness constraint is considered.
	_, err = db.Exec(`INSERT INTO cutover_watermarks(id,instance_id,generation,request_id,wal_lsn,evidence,evidence_sha256) SELECT $1,instance_id,generation,request_id,wal_lsn,evidence,evidence_sha256 FROM cutover_watermarks`, uuid.NewString())
	var pg *pq.Error
	if !errors.As(err, &pg) || pg.Code != "P0001" || pg.Message != "cutover.stale_watermark" {
		t.Fatal("expired watermark did not refuse authority", err)
	}
	var phase string
	if err := db.QueryRow(`SELECT phase FROM cutover_instances`).Scan(&phase); err != nil || phase != "closing" {
		t.Fatal("expired controller reopened source", phase, err)
	}
	cutoverRequest(t, db, instance, 2, request)
}
