package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// These prove the reviewed migration design against PostgreSQL, not the future
// production 000009–000012 implementation. Every database is runner-disposable.
func TestTransitionDesignBoundedResumePreservesLegacy(t *testing.T) {
	db := transitionDesignDB(t, 8, true)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	before := transitionLegacySnapshot(t, ctx, db)
	if err := transitionFixtureExpand(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := transitionFixtureExpand(ctx, db); err != nil {
		t.Fatal("repeated additive up:", err)
	}
	manifest := transitionSourceManifest(t, ctx, db)
	if len(manifest) < 5 {
		t.Fatal("realistic fixture did not cover enough source rows")
	}
	if err := transitionFixtureStart(ctx, db, manifest); err != nil {
		t.Fatal(err)
	}
	started := transitionFixtureSnapshot(t, ctx, db)
	if err := transitionFixtureStart(ctx, db, manifest); err != nil {
		t.Fatal("identical manifest replay:", err)
	}
	if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, started) {
		t.Fatal("identical manifest replay changed progress")
	}
	first, err := transitionFixtureBatch(ctx, db, 2, 0)
	if err != nil || first.Processed != 2 {
		t.Fatalf("first bounded batch: %+v %v", first, err)
	}
	committed := transitionFixtureSnapshot(t, ctx, db)
	if _, err := transitionFixtureBatch(ctx, db, 2, 1); !errors.Is(err, errTransitionInjected) {
		t.Fatalf("mid-batch failure: %v", err)
	}
	if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, committed) {
		t.Fatal("partial batch changed mapping or durable cursor")
	}
	// A fresh pool represents process restart; the only resume state is in SQL.
	db = transitionReconnect(t)
	copyRows, verifiedRows := 2, 0
	for attempts := 0; attempts < 20; attempts++ {
		step, err := transitionFixtureBatch(ctx, db, 2, 0)
		if err != nil {
			t.Fatal(err)
		}
		if step.Processed > 2 {
			t.Fatal("batch exceeded requested bound")
		}
		if step.Pass == "copy" {
			copyRows += step.Processed
		} else if step.Pass == "verify" {
			verifiedRows += step.Processed
		}
		if step.Done {
			break
		}
		if attempts == 19 {
			t.Fatal("bounded resume never completed")
		}
	}
	if copyRows != len(manifest) || verifiedRows != len(manifest) {
		t.Fatalf("copy/verify lost source rows: %d/%d want %d", copyRows, verifiedRows, len(manifest))
	}
	finished := transitionFixtureSnapshot(t, ctx, db)
	if step, err := transitionFixtureBatch(ctx, db, 2, 0); err != nil || !step.Done || step.Processed != 0 {
		t.Fatalf("completed replay: %+v %v", step, err)
	}
	if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, finished) {
		t.Fatal("completed replay changed rows")
	}
	assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
	var unreviewed, distinctSources int
	if err := db.QueryRowContext(ctx, `SELECT count(*),count(DISTINCT(source_kind,source_id)) FROM transition_fixture_archive WHERE archival_state='legacy_unreviewed' AND mode_id IS NULL AND content_language IS NULL AND content_revision IS NULL`).Scan(&unreviewed, &distinctSources); err != nil {
		t.Fatal(err)
	}
	if unreviewed != len(manifest) || distinctSources != len(manifest) {
		t.Fatal("archival copy fabricated approval/suitability or conflated source identities")
	}
}

func TestTransitionDesignRejectsUnsupportedHeadBeforeDDL(t *testing.T) {
	for _, tc := range []struct {
		name  string
		head  int
		dirty bool
	}{{"actual_pre8", 7, false}, {"dirty8", 8, true}} {
		t.Run(tc.name, func(t *testing.T) {
			db := transitionDesignDB(t, tc.head, false)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if tc.dirty {
				if _, err := db.ExecContext(ctx, `UPDATE schema_migrations SET dirty=true`); err != nil {
					t.Fatal(err)
				}
			}
			before := transitionLegacySnapshot(t, ctx, db)
			if err := transitionFixtureExpand(ctx, db); err == nil {
				t.Fatal("unsupported transition input accepted")
			}
			var exists bool
			if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.transition_fixture_archive') IS NOT NULL`).Scan(&exists); err != nil || exists {
				t.Fatalf("refusal created fixture DDL: %v", err)
			}
			assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
		})
	}
}

func TestTransitionDesignDuplicateAndConflictingMappings(t *testing.T) {
	t.Run("destination_collision", func(t *testing.T) {
		db := transitionDesignDB(t, 8, true)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		legacy := transitionLegacySnapshot(t, ctx, db)
		if err := transitionFixtureExpand(ctx, db); err != nil {
			t.Fatal(err)
		}
		manifest := transitionSourceManifest(t, ctx, db)
		if err := transitionFixtureStart(ctx, db, manifest); err != nil {
			t.Fatal(err)
		}
		if err := transitionFixturePut(ctx, db, manifest[1]); err != nil {
			t.Fatal(err)
		}
		// Simulate an incompatible earlier mapping claiming this row's destination.
		if _, err := db.ExecContext(ctx, `UPDATE transition_fixture_archive SET content_id=$1 WHERE source_kind=$2 AND source_id=$3`, "legacy:"+manifest[0].Kind+":"+manifest[0].ID, manifest[1].Kind, manifest[1].ID); err != nil {
			t.Fatal(err)
		}
		before := transitionFixtureSnapshot(t, ctx, db)
		if _, err := transitionFixtureBatch(ctx, db, 2, 0); !errors.Is(err, errTransitionConflict) {
			t.Fatalf("destination collision accepted: %v", err)
		}
		if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, before) {
			t.Fatal("destination collision overwrote mapping/cursor")
		}
		assertTransitionLegacyUnchanged(t, legacy, transitionLegacySnapshot(t, ctx, db))
	})
	t.Run("duplicate_manifest", func(t *testing.T) {
		db := transitionDesignDB(t, 8, true)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := transitionFixtureExpand(ctx, db); err != nil {
			t.Fatal(err)
		}
		manifest := transitionSourceManifest(t, ctx, db)
		manifest = append(manifest, manifest[0])
		before := transitionFixtureSnapshot(t, ctx, db)
		if err := transitionFixtureStart(ctx, db, manifest); !errors.Is(err, errTransitionConflict) {
			t.Fatalf("duplicate manifest key: %v", err)
		}
		if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, before) {
			t.Fatal("duplicate manifest changed progress")
		}
	})
	for _, conflict := range []bool{false, true} {
		name := "identical_replay"
		if conflict {
			name = "changed_hash"
		}
		t.Run(name, func(t *testing.T) {
			db := transitionDesignDB(t, 8, true)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			legacy := transitionLegacySnapshot(t, ctx, db)
			if err := transitionFixtureExpand(ctx, db); err != nil {
				t.Fatal(err)
			}
			manifest := transitionSourceManifest(t, ctx, db)
			if err := transitionFixtureStart(ctx, db, manifest); err != nil {
				t.Fatal(err)
			}
			row := manifest[0]
			if conflict {
				row.Hash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
			}
			if err := transitionFixturePut(ctx, db, row); err != nil {
				t.Fatal(err)
			}
			before := transitionFixtureSnapshot(t, ctx, db)
			_, err := transitionFixtureBatch(ctx, db, 2, 0)
			if conflict {
				if !errors.Is(err, errTransitionConflict) {
					t.Fatalf("changed mapping hash accepted: %v", err)
				}
				if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, before) {
					t.Fatal("conflicting replay overwrote mapping/cursor")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, legacy, transitionLegacySnapshot(t, ctx, db))
		})
	}
}

func TestTransitionDesignDetectsSourceDrift(t *testing.T) {
	for _, kind := range []string{"changed_behind_cursor", "changed_ahead_cursor", "late_low_key", "late_high_key", "deleted_ahead_cursor"} {
		t.Run(kind, func(t *testing.T) {
			db := transitionDesignDB(t, 8, true)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := transitionFixtureExpand(ctx, db); err != nil {
				t.Fatal(err)
			}
			manifest := transitionSourceManifest(t, ctx, db)
			if err := transitionFixtureStart(ctx, db, manifest); err != nil {
				t.Fatal(err)
			}
			if _, err := transitionFixtureBatch(ctx, db, 2, 0); err != nil {
				t.Fatal(err)
			}
			transitionMutateSource(t, ctx, db, kind, manifest)
			legacyAfterExternalWrite := transitionLegacySnapshot(t, ctx, db)
			failed := false
			for attempts := 0; attempts < 20; attempts++ {
				before := transitionFixtureSnapshot(t, ctx, db)
				step, err := transitionFixtureBatch(ctx, db, 2, 0)
				if err != nil {
					if !errors.Is(err, errTransitionSourceDrift) {
						t.Fatal(err)
					}
					if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, before) {
						t.Fatal("failing drift batch changed cursor/mappings")
					}
					failed = true
					break
				}
				if step.Done {
					t.Fatal("reported complete after source drift")
				}
			}
			if !failed {
				t.Fatal("source drift was never detected")
			}
			assertTransitionLegacyUnchanged(t, legacyAfterExternalWrite, transitionLegacySnapshot(t, ctx, db))
		})
	}
}

func TestTransitionDesignControlledDown(t *testing.T) {
	for _, populated := range []bool{false, true} {
		name := "empty_additions"
		if populated {
			name = "retained_additions"
		}
		t.Run(name, func(t *testing.T) {
			db := transitionDesignDB(t, 8, true)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			before := transitionLegacySnapshot(t, ctx, db)
			if err := transitionFixtureExpand(ctx, db); err != nil {
				t.Fatal(err)
			}
			if populated {
				if err := transitionFixtureStart(ctx, db, transitionSourceManifest(t, ctx, db)); err != nil {
					t.Fatal(err)
				}
				if _, err := transitionFixtureBatch(ctx, db, 2, 0); err != nil {
					t.Fatal(err)
				}
			}
			additions := transitionFixtureSnapshot(t, ctx, db)
			err := transitionFixtureDown(ctx, db)
			if populated {
				if err == nil {
					t.Fatal("down erased retained transition records")
				}
				if got := transitionFixtureSnapshot(t, ctx, db); !reflect.DeepEqual(got, additions) {
					t.Fatal("refused down changed retained additions")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
		})
	}
}
