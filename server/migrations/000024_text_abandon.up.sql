-- Retain exact first expired-grace incidents; never reconstruct them after loss.
CREATE TABLE text_abandons (
 match_id UUID NOT NULL REFERENCES text_matches(id),
 account_id UUID NOT NULL REFERENCES accounts(id),
 seat INT NOT NULL CHECK(seat>=0 AND seat<6),
 occurred_at TIMESTAMPTZ NOT NULL,
 body_hash TEXT NOT NULL CHECK(body_hash ~ '^[a-f0-9]{64}$'),
 prior_count INT NOT NULL CHECK(prior_count>=0),
 applied_count INT NOT NULL CHECK(applied_count>=prior_count AND applied_count>0),
 duration_seconds INT NOT NULL CHECK(duration_seconds>0),
 cooldown_until TIMESTAMPTZ NOT NULL CHECK(cooldown_until>=occurred_at),
 PRIMARY KEY(match_id,account_id),
 UNIQUE(match_id,seat)
);
CREATE FUNCTION retain_text_abandon() RETURNS trigger LANGUAGE plpgsql SET row_security=off AS $$
BEGIN
 IF TG_OP='TRUNCATE' AND NOT EXISTS(SELECT 1 FROM text_abandons) THEN RETURN NULL; END IF;
	RAISE EXCEPTION 'text abandonment receipt is immutable; forward fix required';
END $$;
CREATE TRIGGER text_abandon_immutable BEFORE UPDATE OR DELETE ON text_abandons FOR EACH ROW EXECUTE FUNCTION retain_text_abandon();
CREATE TRIGGER text_abandon_truncate_retained BEFORE TRUNCATE ON text_abandons FOR EACH STATEMENT EXECUTE FUNCTION retain_text_abandon();
