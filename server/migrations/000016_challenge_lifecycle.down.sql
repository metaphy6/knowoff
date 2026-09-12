DO $$ BEGIN RAISE EXCEPTION 'challenge lifecycle requires reviewed backup restore; automatic rollback would erase title and payout evidence'; END $$;
