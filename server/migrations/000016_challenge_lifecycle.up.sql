ALTER TABLE challenge_topics ADD COLUMN activated_at TIMESTAMPTZ;
ALTER TABLE challenge_topics ADD COLUMN source_revision TEXT;
ALTER TABLE challenge_topics ADD CONSTRAINT challenge_source_revision CHECK(source_revision IS NULL OR source_revision ~ '^[0-9a-f]{64}$');
-- Historical publication evidence stays historical; new scheduling explicitly
-- fills the pinned revision and activation record through the reviewed service.
UPDATE challenge_topics SET activated_at=published_at;
ALTER TABLE challenge_winners ADD COLUMN payout_amount INTEGER CHECK(payout_amount>=0);
CREATE TABLE challenge_current_winner (
 singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK(singleton),
 topic_id UUID REFERENCES challenge_topics(id),
 entry_id UUID REFERENCES challenge_entries(id),
 account_id UUID REFERENCES accounts(id),
 week_start DATE,
 crowned_at TIMESTAMPTZ,
 CHECK((topic_id IS NULL AND entry_id IS NULL AND account_id IS NULL AND week_start IS NULL AND crowned_at IS NULL)
 OR (topic_id IS NOT NULL AND entry_id IS NOT NULL AND account_id IS NOT NULL AND week_start IS NOT NULL AND crowned_at IS NOT NULL))
);
INSERT INTO challenge_current_winner(singleton) VALUES(true);
CREATE INDEX challenge_topics_scheduled ON challenge_topics(week_start,id) WHERE activated_at IS NULL AND closed_at IS NULL;
