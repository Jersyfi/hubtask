-- The Notification context gets its two tables (C-09, arc42 §5.2).
--
-- `notification` is the record, and `notification_preference` is what a person has said about
-- being told. Both have been named in the architecture since the beginning and built by nothing:
-- `data-retention.md` §3 has been promising a `NOTIFICATION` class at 90 days against a table that
-- did not exist.
--
-- The record holds references and no content. Which entry, which comment, who caused it - never
-- the title and never the note, for the reason the outbox payload carries references only
-- (data-protection.md §5) and the invitation job carries identifiers only: a row that kept a copy
-- of the title would be a second place the title has to be deleted from, and the deletion path of
-- the first one would no longer be the whole answer. What an email says is read from the entry at
-- the moment it is rendered, which is also the only way an email can be right about an entry that
-- was renamed while the notification waited in the queue.
--
-- Forward-only and idempotent, so a re-run during a rolling update changes nothing
-- (CLAUDE.md rule 12, ADR-0003). Nothing here rewrites a table: two new tables, their indexes and
-- their policies, none of which an old pod knows about or selects from.

-- +goose Up

-- What somebody is to be told, and how far that got.
--
-- One row per recipient per event per channel, which is what the unique index below makes true. A
-- consumer that reacted to an event twice - the outbox delivers at-least-once (ADR-0007) - writes
-- the same row twice and the second insert does nothing, so the deduplication holds even where the
-- consumption record was lost between the delivery and the commit.
CREATE TABLE IF NOT EXISTS notification (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- Who is being told. Their locale is what the email is rendered in (i18n-l10n.md §1), read from
  -- the account row at render time rather than copied here: a person who switches language wants
  -- the next email in it, not the one after the queue has drained.
  recipient_id uuid NOT NULL,
  -- What kind of thing happened, at the granularity a person switches off. Coarser than the event
  -- type on purpose: somebody who does not want to hear about comments means all of them, and a
  -- preference per event type would be a settings screen nobody finishes.
  category     text NOT NULL CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION')),
  -- Email is the only channel that sends in this milestone. The column exists because the
  -- preference is per channel and the record has to say which one it was written for - a webhook
  -- and a push notification are the same decision made twice otherwise.
  channel      text NOT NULL CHECK (channel IN ('EMAIL')),
  -- PENDING is waiting for the delivery job, SENT is gone, SUPPRESSED is a decision not to send,
  -- and FAILED is a delivery that used up its attempts. SUPPRESSED is a state rather than an
  -- absent row because "the record says why" is an acceptance criterion: a person asking why they
  -- heard nothing deserves an answer better than silence.
  state        text NOT NULL CHECK (state IN ('PENDING','SENT','SUPPRESSED','FAILED')),
  -- Why it is in that state, as a detail code and never a sentence (rule 8). Set for SUPPRESSED
  -- and FAILED, null for the two states that need no explanation.
  reason       text,
  -- The outbox event that caused it, which is what the deduplication is over. Null for the
  -- invitation: that one is queued by the use case that created the account rather than by an
  -- event, and its own dedupe key is the account.
  event_id     uuid,
  -- What it is about. Null where there is no entry - an invitation is about the workspace.
  item_id      uuid,
  -- Who caused it. Null where nobody did, which today is nothing but is the honest shape: the
  -- automatic assignment acts for the system (C-02).
  actor_id     uuid,
  created_at   timestamptz NOT NULL DEFAULT now(),
  sent_at      timestamptz,
  -- What the delivery has already tried. The queue counts its own attempts; this is the record's
  -- copy, so that an operator reading the table sees a stuck notification without joining the job.
  attempts     integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),

  CONSTRAINT notification_recipient_id_fkey
    FOREIGN KEY (tenant_id, recipient_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  -- The actor's account may go while the record stays: a notification is a fact about the past,
  -- and losing it because the person who caused it left would lose the evidence of what was sent.
  CONSTRAINT notification_actor_id_fkey
    FOREIGN KEY (tenant_id, actor_id) REFERENCES account (tenant_id, id)
      ON DELETE SET NULL (actor_id)
);

-- The deduplication. Partial rather than total, because the invitation carries no event and NULLs
-- are all distinct to a unique index - without the predicate the index would simply not apply to
-- the rows it was built for and would apply to nothing else either.
CREATE UNIQUE INDEX IF NOT EXISTS notification_event_recipient_idx
  ON notification (tenant_id, event_id, recipient_id, channel)
  WHERE event_id IS NOT NULL;

-- What the delivery job claims: the pending rows of one tenant, oldest first. Partial, because a
-- table that keeps ninety days of sent notifications is mostly rows this query never wants.
CREATE INDEX IF NOT EXISTS notification_pending_idx
  ON notification (tenant_id, created_at)
  WHERE state = 'PENDING';

-- What the retention sweep walks (data-retention.md §3: anchor `created_at`, 90 days).
CREATE INDEX IF NOT EXISTS notification_retention_idx ON notification (tenant_id, created_at);

-- What a person has said about being told.
--
-- A row is an exception, not a setting: the absence of one is the default, which is on. That is
-- what keeps a new category from needing a backfill across every account in every tenant, and it
-- is why there is no `enabled` default to argue about in two places.
CREATE TABLE IF NOT EXISTS notification_preference (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  category   text NOT NULL CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION')),
  channel    text NOT NULL CHECK (channel IN ('EMAIL')),
  enabled    boolean NOT NULL DEFAULT true,
  -- Whether the entry's title may travel in the message. True by default, which is what
  -- data-protection.md §9 means by "title and link only, no full text; switchable": the minimum is
  -- the default and the switch takes even that away, leaving a message that says something
  -- concerns you and where to look. There is no setting that adds the note body - that is not a
  -- preference, it is a rule.
  include_title boolean NOT NULL DEFAULT true,
  updated_at timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (tenant_id, account_id, category, channel),
  CONSTRAINT notification_preference_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE notification ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification FORCE ROW LEVEL SECURITY;
ALTER TABLE notification_preference ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_preference FORCE ROW LEVEL SECURITY;

-- +goose StatementBegin
DO $notification_policies$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY['notification','notification_preference']
  LOOP
    IF NOT EXISTS (
      SELECT 1 FROM pg_policies
      WHERE schemaname = 'public' AND tablename = t AND policyname = 'tenant_isolation'
    ) THEN
      EXECUTE format($f$
        CREATE POLICY tenant_isolation ON %I
          USING (tenant_id = current_tenant_id())
          WITH CHECK (tenant_id = current_tenant_id())
      $f$, t);
    END IF;
  END LOOP;
END $notification_policies$;
-- +goose StatementEnd

-- Explicit rather than relying on the default privileges of 0001: those apply to tables created by
-- hubtask_migrator, and a migration must also work where the operator runs it as somebody else.
GRANT SELECT, INSERT, UPDATE, DELETE ON notification TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON notification_preference TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (CLAUDE.md rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
