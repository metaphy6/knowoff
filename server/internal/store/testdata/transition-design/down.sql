-- Exact disposable-fixture down only: never all-migrations-down, never erase
-- retained sidecars, job evidence or original content/value records.
LOCK TABLE transition_fixture_archive, transition_fixture_progress IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM transition_fixture_archive)
       OR EXISTS (SELECT 1 FROM transition_fixture_progress) THEN
        RAISE EXCEPTION 'retained transition data requires compatible application rollback or forward fix';
    END IF;
END $$;
DROP TABLE transition_fixture_archive;
DROP TABLE transition_fixture_progress;
