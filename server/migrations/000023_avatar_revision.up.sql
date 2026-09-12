-- Historical avatar blobs remain retained and unapproved. A positive matching
-- revision is written only after normalization and successful image screening.
ALTER TABLE accounts ADD COLUMN avatar_revision BIGINT NOT NULL DEFAULT 0 CHECK (avatar_revision >= 0);
ALTER TABLE custom_avatars ADD COLUMN revision BIGINT CHECK (revision > 0);
CREATE FUNCTION keep_avatar_revision_monotonic() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.avatar_revision < OLD.avatar_revision THEN
  RAISE EXCEPTION 'avatar revision cannot decrease';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER accounts_avatar_revision_monotonic BEFORE UPDATE OF avatar_revision ON accounts
 FOR EACH ROW EXECUTE FUNCTION keep_avatar_revision_monotonic();
