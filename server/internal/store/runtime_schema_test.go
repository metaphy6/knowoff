package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeSchemaUsesPublicVersionAndMakesNoRepairs(t *testing.T) {
	db := disposableMigrationDB(t)
	if err := MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`SET search_path=public; UPDATE public.schema_migrations SET dirty=false; DROP SCHEMA IF EXISTS runtime_shadow CASCADE`); err != nil {
			t.Error(err)
		}
	})
	db.SetMaxOpenConns(1)
	if err := CheckRuntimeSchema(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA runtime_shadow; CREATE TABLE runtime_shadow.schema_migrations AS TABLE public.schema_migrations; UPDATE public.schema_migrations SET dirty=true; SET search_path=runtime_shadow,public`); err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntimeSchema(t.Context(), db); !errors.Is(err, ErrRuntimeSchema) {
		t.Fatal("shadow schema masked dirty public version", err)
	}
	var dirty bool
	if err := db.QueryRow(`SELECT dirty FROM public.schema_migrations`).Scan(&dirty); err != nil || !dirty {
		t.Fatal("startup repaired dirty state", dirty, err)
	}
}

func TestRuntimeSchemaHonorsCallerDeadlineBeforeConnection(t *testing.T) {
	db := disposableMigrationDB(t)
	if err := MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err = CheckRuntimeSchema(ctx, db); !errors.Is(err, ErrRuntimeSchema) || time.Since(started) > time.Second {
		t.Fatal("schema check ignored pool deadline", err)
	}
}
