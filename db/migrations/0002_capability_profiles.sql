-- The system-defined capability profiles: the capability matrix of docs/architecture/domain-model.md
-- §2, as data rather than as code.
--
-- Data because it is a policy, not a rule: a tenant may narrow a profile with a row of its own
-- (tenant_id set), and /meta/capabilities answers from whichever applies. A constant in Go would
-- have to be kept in step with the overrides in the table, and the two would disagree.
--
-- tenant_id IS NULL marks a system default. Every tenant may read those rows and none may write
-- them - the policy on item_capability_profile allows the read and its WITH CHECK forbids the
-- write (db/schema.sql).
--
-- Forward-only and idempotent, so a re-run during a rolling update changes nothing
-- (CLAUDE.md rule 12, ADR-0003).

-- +goose Up

INSERT INTO item_capability_profile (tenant_id, type, capabilities, allowed_child_types, max_depth)
VALUES
  (NULL, 'TASK', ARRAY[
     'COMPLETION','DUE_DATE','REMINDER','ASSIGNMENT','MEMBERS','BUCKET','NOTES','LABELS',
     'COMMENTS','COVER','ATTACHMENTS','HISTORY','RECURRENCE','CUSTOM_FIELDS'
   ], ARRAY['WORK_PACKAGE']::item_type[], 3),

  (NULL, 'WORK_PACKAGE', ARRAY[
     'COMPLETION','DUE_DATE','REMINDER','ASSIGNMENT','MEMBERS','NOTES','LABELS','COMMENTS',
     'ATTACHMENTS','HISTORY','CUSTOM_FIELDS'
   ], ARRAY['ACTIVITY']::item_type[], 2),

  -- An activity is the reduced level: done or open, a date, a reminder, exactly one assignee,
  -- and a compact history. No notes, no labels, no comments, no children.
  (NULL, 'ACTIVITY', ARRAY[
     'COMPLETION','DUE_DATE','REMINDER','ASSIGNMENT','HISTORY'
   ], ARRAY[]::item_type[], 1)
ON CONFLICT DO NOTHING;

-- +goose Down

-- Removing the profiles would leave the installation unable to answer what an item type can do,
-- which is worse than a schema that has moved on. The answer to a bad deploy is a restore
-- (ADR-0003).
SELECT 1;
