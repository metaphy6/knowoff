DO $$ BEGIN RAISE EXCEPTION 'report identity and moderation receipts require an explicit preservation migration; automatic rollback refused'; END $$;
