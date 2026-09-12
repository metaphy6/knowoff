-- Downgrading would discard attributed Guard decisions and timed sanctions.
-- Restore a reviewed pre-migration backup instead of erasing safety history.
DO $$ BEGIN RAISE EXCEPTION 'guard isolation requires reviewed backup restore; automatic rollback is unsupported'; END $$;
