-- Hubtask - the reference schema.
-- Was the target state while the model was designed; since db/migrations/0001_init.sql exists,
-- the migrations are the source and this file is the readable reference of the same state.
-- Creates the core tables including tenant isolation through row level security.
-- Conventions: UUIDv7 (generated in the application), timestamptz in UTC, tenant_id in every
-- business table, soft delete through deleted_at, optimistic locking through version.
-- Every table a tenant-scoped foreign key points at carries UNIQUE (tenant_id, id) beside its
-- primary key: a composite key needs a unique index on exactly the columns it references, and
-- the tenant-first index is what row level security compares first anyway (ADR-0024, ADR-0010).
-- See docs/architecture/{domain-model,multi-tenancy}.md

BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS unaccent;
-- CREATE EXTENSION IF NOT EXISTS vector;   -- optional, semantic search

-- ---------------------------------------------------------------------------
-- Roles (created once, outside the migration)
--   hubtask_migrator : owns the objects, runs the migrations
--   hubtask_app      : the application role, NO BYPASSRLS, not an owner
-- ---------------------------------------------------------------------------

-- unaccent() is only STABLE and therefore not indexable -> an IMMUTABLE wrapper
CREATE OR REPLACE FUNCTION imm_unaccent(text) RETURNS text
  LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT AS
$$ SELECT public.unaccent('public.unaccent'::regdictionary, $1) $$;

CREATE OR REPLACE FUNCTION current_tenant_id() RETURNS uuid
  LANGUAGE sql STABLE AS
$$ SELECT nullif(current_setting('app.tenant_id', true), '')::uuid $$;

-- The text search configuration one item's language is indexed and searched under (C-08,
-- ADR-0034). STABLE rather than IMMUTABLE - which is what a trigger permits and a generated column
-- does not - so that the catalogue can be asked what it has and anything it has not falls back to
-- `simple` instead of failing the write.
-- The mapping, and the one place it is written down: /meta/capabilities answers this same
-- function, so a client's language picker is data rather than a constant.
CREATE OR REPLACE FUNCTION hubtask_text_languages()
  RETURNS TABLE (tag text, configuration text) LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT m.tag, c.cfgname::text
    FROM (VALUES
      ('ar','arabic'),    ('hy','armenian'),   ('eu','basque'),      ('ca','catalan'),
      ('da','danish'),    ('nl','dutch'),      ('en','english'),     ('fi','finnish'),
      ('fr','french'),    ('de','german'),     ('el','greek'),       ('hi','hindi'),
      ('hu','hungarian'), ('id','indonesian'), ('ga','irish'),       ('it','italian'),
      ('lt','lithuanian'),('ne','nepali'),     ('nb','norwegian'),   ('nn','norwegian'),
      ('no','norwegian'), ('pt','portuguese'), ('ro','romanian'),    ('ru','russian'),
      ('sr','serbian'),   ('es','spanish'),    ('sv','swedish'),     ('ta','tamil'),
      ('tr','turkish'),   ('yi','yiddish')
    ) AS m(tag, cfgname)
    JOIN pg_ts_config c ON c.cfgname = m.cfgname
   ORDER BY m.tag
$$;

CREATE OR REPLACE FUNCTION hubtask_text_config(language text) RETURNS regconfig
  LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT coalesce(
    (SELECT l.configuration::regconfig
       FROM hubtask_text_languages() l
      WHERE l.tag = lower(split_part(btrim(coalesce(language, '')), '-', 1))
      LIMIT 1),
    'simple'::regconfig)
$$;

-- The title weighted A and the notes B, so that ts_rank_cd ranks a hit in a title above one buried
-- in a note.
CREATE OR REPLACE FUNCTION hubtask_search_document(language text, title text, notes text)
  RETURNS tsvector LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT setweight(to_tsvector(hubtask_text_config(language), coalesce(title, '')), 'A')
      || setweight(to_tsvector(hubtask_text_config(language), coalesce(notes, '')), 'B')
$$;

CREATE OR REPLACE FUNCTION work_item_search_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  NEW.search_document := hubtask_search_document(NEW.content_language, NEW.title, NEW.notes);
  RETURN NEW;
END $$;

-- ============================ Identity & Access ============================

CREATE TYPE tenant_status AS ENUM ('ACTIVE', 'SUSPENDED', 'PENDING_DELETION');

CREATE TABLE tenant (
  id                 uuid PRIMARY KEY,
  slug               text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{2,39}$'),
  display_name       text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 200),
  status             tenant_status NOT NULL DEFAULT 'ACTIVE',
  default_locale     text NOT NULL DEFAULT 'en',
  default_time_zone  text NOT NULL DEFAULT 'UTC',
  settings           jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  deleted_at         timestamptz,
  version            integer NOT NULL DEFAULT 1
);

CREATE TYPE account_kind   AS ENUM ('USER', 'SERVICE_ACCOUNT');
-- RESTRICTED is Art. 18 as a technical state (readable, not processed) and ANONYMIZED is an
-- erasure carried out in the mode that keeps the authorship (db/migrations/0044_privacy_requests.sql).
CREATE TYPE account_status AS ENUM ('ACTIVE', 'INVITED', 'DISABLED', 'RESTRICTED', 'ANONYMIZED');

CREATE TABLE account (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  kind              account_kind NOT NULL DEFAULT 'USER',
  email             text,
  display_name      text NOT NULL,
  external_subject  text,
  password_hash     text,                      -- local accounts only (argon2id)
  locale            text,
  time_zone         text,
  week_start        text,
  status            account_status NOT NULL DEFAULT 'ACTIVE',
  ai_consent        boolean NOT NULL DEFAULT false,
  -- The redemption token the invitation mints (H-01): hashed under its own purpose label, shown
  -- once, dead on redemption. One open invitation per invited account, so it lives on the row.
  redemption_token_hash bytea,
  redemption_expires_at timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz,
  version           integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX account_tenant_id_uq ON account (tenant_id, id);
CREATE UNIQUE INDEX account_email_uq ON account (tenant_id, lower(email))
  WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX account_subject_uq ON account (tenant_id, external_subject)
  WHERE external_subject IS NOT NULL;
-- The lookup the public redemption route makes: the hash covers the whole presented string,
-- tenant half included, and is unique across the installation.
CREATE UNIQUE INDEX account_redemption_token_uq ON account (redemption_token_hash)
  WHERE redemption_token_hash IS NOT NULL;

CREATE TABLE account_group (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  name         text NOT NULL,
  description  text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  version      integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX account_group_tenant_id_uq ON account_group (tenant_id, id);
CREATE UNIQUE INDEX account_group_name_uq ON account_group (tenant_id, lower(name));

CREATE TABLE account_group_member (
  tenant_id  uuid NOT NULL,
  group_id   uuid NOT NULL,
  account_id uuid NOT NULL,
  PRIMARY KEY (group_id, account_id),
  CONSTRAINT account_group_member_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT account_group_member_group_id_fkey
    FOREIGN KEY (tenant_id, group_id) REFERENCES account_group (tenant_id, id) ON DELETE CASCADE
);

CREATE TYPE membership_scope AS ENUM ('TENANT', 'HUB', 'COLLECTION', 'ITEM');
-- AUDITOR is last because it is not a rung on the same ladder: it reads the audit trail and the
-- configuration and no content (audit.md §5). Appended rather than sorted in, for the reason
-- 0042_auditor_role.sql gives.
CREATE TYPE membership_role  AS ENUM ('OWNER', 'ADMIN', 'MEMBER', 'CONTRIBUTOR', 'VIEWER', 'GUEST', 'AUDITOR');

CREATE TABLE membership (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id  uuid,
  group_id    uuid,
  scope_type  membership_scope NOT NULL,
  scope_id    uuid,                              -- NULL when scope_type = TENANT
  role        membership_role NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK ((account_id IS NULL) <> (group_id IS NULL)),
  CHECK ((scope_type = 'TENANT') = (scope_id IS NULL)),
  CONSTRAINT membership_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT membership_group_id_fkey
    FOREIGN KEY (tenant_id, group_id) REFERENCES account_group (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX membership_lookup_idx ON membership (tenant_id, account_id, scope_type, scope_id);
CREATE INDEX membership_scope_idx  ON membership (tenant_id, scope_type, scope_id);

CREATE TABLE access_token (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id    uuid NOT NULL,
  name          text NOT NULL,
  token_hash    bytea NOT NULL,
  token_prefix  text NOT NULL,
  scopes        text[] NOT NULL DEFAULT '{}',
  expires_at    timestamptz,
  last_used_at  timestamptz,
  revoked_at    timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT access_token_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX access_token_hash_uq ON access_token (token_hash);
CREATE INDEX access_token_account_idx ON access_token (tenant_id, account_id, created_at DESC);

-- A sign-in (H-01, security.md §5): the row /auth/sessions lists and revocation stamps. Both
-- tokens of the pair point at it, so ending it ends them together. `last_seen_at` is the
-- retention anchor of the SESSION data kind; `user_agent` and `ip_class` are the client-binding
-- hint T-01 asks to log - the network coarsened at recording time, never the full address.
CREATE TABLE session (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id   uuid NOT NULL,
  created_at   timestamptz NOT NULL,
  last_seen_at timestamptz,
  user_agent   text,
  ip_class     text,
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz,
  -- The step-up (H-03): a fresh re-authentication recorded on the session, one live proof at a
  -- time, consumed by the one privileged action it is presented to.
  step_up_token_hash  bytea,
  step_up_at          timestamptz,
  step_up_method      text,
  step_up_consumed_at timestamptz,
  CONSTRAINT session_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX session_tenant_id_uq ON session (tenant_id, id);
CREATE UNIQUE INDEX session_step_up_token_uq ON session (step_up_token_hash)
  WHERE step_up_token_hash IS NOT NULL;
CREATE INDEX session_account_idx ON session (account_id, created_at DESC);

-- One refresh token of a session's family. Rotation retires a row and inserts the next; retired
-- rows stay until the session goes, because a retired hash presented again is the reuse signal
-- T-01 exists for.
CREATE TABLE session_refresh_token (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  session_id  uuid NOT NULL,
  token_hash  bytea NOT NULL,
  created_at  timestamptz NOT NULL,
  expires_at  timestamptz NOT NULL,
  rotated_at  timestamptz,
  CONSTRAINT session_refresh_token_session_fkey FOREIGN KEY (tenant_id, session_id)
    REFERENCES session (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX session_refresh_token_hash_uq ON session_refresh_token (token_hash);
CREATE INDEX session_refresh_token_session_idx ON session_refresh_token (session_id);

-- One TOTP enrolment per account (H-02, security.md §5): the secret sealed through the envelope
-- encryption, armed only once `confirmed_at` is set, `last_step` as the replay refusal.
CREATE TABLE account_mfa (
  account_id    uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  secret_enc    bytea NOT NULL,
  secret_key_id text NOT NULL,
  confirmed_at  timestamptz,
  last_step     bigint,
  created_at    timestamptz NOT NULL,
  updated_at    timestamptz NOT NULL,
  CONSTRAINT account_mfa_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- Ten single-use recovery codes per enrolment, stored only as hashes, burned by first use.
CREATE TABLE account_recovery_code (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  code_hash  bytea NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL,
  CONSTRAINT account_recovery_code_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX account_recovery_code_hash_uq ON account_recovery_code (code_hash);
CREATE INDEX account_recovery_code_account_idx ON account_recovery_code (account_id);

-- The pending credential of a two-step sign-in (H-02): short-lived, single-use, hashed under
-- its own purpose label - a row with the session machinery's discipline, not a session.
CREATE TABLE auth_pending (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id  uuid NOT NULL,
  token_hash  bytea NOT NULL,
  purpose     text NOT NULL CHECK (purpose IN ('TOTP', 'ENROLL')),
  user_agent  text,
  ip_class    text,
  created_at  timestamptz NOT NULL,
  expires_at  timestamptz NOT NULL,
  consumed_at timestamptz,
  CONSTRAINT auth_pending_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX auth_pending_token_uq ON auth_pending (token_hash);

-- The sign-in attempt ledger (T-02): failures per account and per source network, the subject
-- only ever a hash under its own purpose label - the ledger counts attempts against addresses
-- that hold no account without becoming a list of guessed addresses.
CREATE TABLE auth_attempt (
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  subject_hash    bytea NOT NULL,
  failures        integer NOT NULL DEFAULT 0,
  last_failure_at timestamptz,
  locked_until    timestamptz,
  PRIMARY KEY (tenant_id, subject_hash)
);

-- ============================ Work Management ==============================

CREATE TYPE container_type AS ENUM ('HUB', 'COLLECTION');

CREATE TABLE container (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  type         container_type NOT NULL,
  parent_id    uuid,
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
  description  text,
  icon         text,
  color_token  text,
  order_key    text NOT NULL,
  -- policies keys (domain-model.md §3.3): completion_policy (MANUAL | ROLLUP, read since B-07),
  -- default_bucket_id, capability_overrides, auto_assign. An absent key means the default; the column
  -- starts as {} and UpdateContainerPolicies (B-06) is what writes into it.
  --
  -- auto_assign is a key of the policies *document*, not of this column: C-02 stores it in
  -- auto_assign_policy, because ROUND_ROBIN's cursor has to be lockable, and the container queries
  -- join it back so every read still carries the whole document.
  --
  -- default_bucket_id has no writer and is not read. B-09 needed a column for a deleted bucket's
  -- items to fall back to and derived it instead - the collection's leftmost remaining bucket -
  -- because a stored default is a value nothing keeps up to date: a column deleted while the key
  -- still named it would send items into a bucket that is no longer on the board.
  policies     jsonb NOT NULL DEFAULT '{}'::jsonb,
  archived_at  timestamptz,
  deleted_at   timestamptz,
  trash_batch_id uuid,
  created_by   uuid NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  version      integer NOT NULL DEFAULT 1,
  CHECK ((type = 'HUB') = (parent_id IS NULL))
);
CREATE UNIQUE INDEX container_tenant_id_uq ON container (tenant_id, id);
ALTER TABLE container ADD CONSTRAINT container_parent_id_fkey
    FOREIGN KEY (tenant_id, parent_id) REFERENCES container (tenant_id, id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX container_name_uq
  ON container (tenant_id, coalesce(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(imm_unaccent(name)))
  WHERE deleted_at IS NULL;
CREATE INDEX container_parent_idx ON container (tenant_id, parent_id, order_key)
  WHERE deleted_at IS NULL;
-- The trash view and the way back out of it. Every other index on this table is partial on
-- `deleted_at IS NULL` and therefore describes exactly what the trash is not (B-10).
CREATE INDEX container_trash_idx ON container (tenant_id, deleted_at)
  WHERE deleted_at IS NOT NULL;
CREATE INDEX container_trash_batch_idx ON container (tenant_id, trash_batch_id)
  WHERE trash_batch_id IS NOT NULL;

CREATE TABLE bucket (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id  uuid NOT NULL,
  name           text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  order_key      text NOT NULL,
  wip_limit      integer CHECK (wip_limit IS NULL OR wip_limit > 0),
  is_done_bucket boolean NOT NULL DEFAULT false,
  color_token    text,
  deleted_at     timestamptz,
  version        integer NOT NULL DEFAULT 1,
  CONSTRAINT bucket_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX bucket_tenant_id_uq ON bucket (tenant_id, id);
CREATE UNIQUE INDEX bucket_name_uq ON bucket (tenant_id, collection_id, lower(imm_unaccent(name)))
  WHERE deleted_at IS NULL;
CREATE INDEX bucket_order_idx ON bucket (tenant_id, collection_id, order_key);

CREATE TABLE label (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id  uuid NOT NULL,
  name           text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  color_token    text NOT NULL,
  description    text,
  deleted_at     timestamptz,
  version        integer NOT NULL DEFAULT 1,
  CONSTRAINT label_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX label_tenant_id_uq ON label (tenant_id, id);
CREATE UNIQUE INDEX label_name_uq ON label (tenant_id, collection_id, lower(imm_unaccent(name)))
  WHERE deleted_at IS NULL;

-- Extensible: new item types are added here, the capability profile lives in the code
-- or in item_capability_profile.
CREATE TYPE item_type AS ENUM ('TASK', 'WORK_PACKAGE', 'ACTIVITY');

CREATE TABLE item_capability_profile (
  tenant_id           uuid,                      -- NULL = a system default
  type                item_type NOT NULL,
  capabilities        text[] NOT NULL,
  allowed_child_types item_type[] NOT NULL DEFAULT '{}',
  max_depth           integer NOT NULL
);
CREATE UNIQUE INDEX icp_uq ON item_capability_profile
  (coalesce(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), type);

CREATE TABLE work_item (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id      uuid NOT NULL,
  type               item_type NOT NULL,
  parent_id          uuid,
  path               text NOT NULL,              -- '/<uuid>/<uuid>/…', materialised
  depth              integer NOT NULL CHECK (depth >= 0),
  title              text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 500),
  notes              text,
  is_completed       boolean NOT NULL DEFAULT false,
  completed_at       timestamptz,
  completed_by       uuid,
  bucket_id          uuid,
  order_key          text NOT NULL,
  start_at           timestamptz,
  due_at             timestamptz,
  due_date_only      boolean NOT NULL DEFAULT false,
  due_time_zone      text,
  assignee_id        uuid,
  cover_kind         text CHECK (cover_kind IN ('COLOR', 'IMAGE')),
  cover_color_token  text,
  cover_media_id     uuid,
  custom_fields      jsonb NOT NULL DEFAULT '{}'::jsonb,
  -- Which definition each custom field value was written under: {key: definition id}. A value is
  -- visible only while exactly that definition lives, which is what keeps a deleted-and-recreated
  -- key from resurrecting the old value (C-07, migration 0018).
  custom_field_refs  jsonb NOT NULL DEFAULT '{}'::jsonb,
  recurrence_rule_id uuid,
  origin_jumble_id   uuid,
  content_language   text,
  search_vector      tsvector GENERATED ALWAYS AS
                       (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(notes, ''))) STORED,
  -- The language-dependent document the search reads, maintained by the trigger below and dropping
  -- the generated column above in a later migration (C-08, migration 0019, ADR-0034).
  search_document    tsvector,
  due_soon_announced_at timestamptz,
  overdue_announced_at  timestamptz,
  -- What a marked object carries between the two phases of a retention run (migration 0038,
  -- data-retention.md §5, §6): when the act is due, under which rule, and what the act is.
  retention_pending_until timestamptz,
  retention_rule_id  uuid,
  retention_action   text,
  -- Why the act is not happening, when something is stopping it (migration 0041). A blocked entry
  -- carries the rule and the action and no `retention_pending_until`: the absence of a due moment
  -- is what keeps phase two off it.
  retention_blocked_by text,
  archived_at        timestamptz,
  deleted_at         timestamptz,
  trash_batch_id     uuid,
  created_by         uuid NOT NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  version            integer NOT NULL DEFAULT 1,
  CHECK (is_completed = (completed_at IS NOT NULL)),
  CHECK ((type = 'TASK') = (parent_id IS NULL)),
  CHECK (bucket_id IS NULL OR type = 'TASK'),
  -- The cover's integrity (C-06, migration 0013). No TASK tie: which types carry COVER is the
  -- capability matrix, which is data per tenant.
  CONSTRAINT work_item_cover_consistent CHECK (
    (cover_kind IS NULL AND cover_color_token IS NULL AND cover_media_id IS NULL)
    OR (cover_kind = 'COLOR' AND cover_color_token IS NOT NULL AND cover_media_id IS NULL)
    OR (cover_kind = 'IMAGE' AND cover_media_id IS NOT NULL AND cover_color_token IS NULL)
  ),
  CONSTRAINT work_item_assignee_id_fkey
    FOREIGN KEY (tenant_id, assignee_id) REFERENCES account (tenant_id, id) ON DELETE SET NULL (assignee_id),
  CONSTRAINT work_item_bucket_id_fkey
    FOREIGN KEY (tenant_id, bucket_id) REFERENCES bucket (tenant_id, id) ON DELETE SET NULL (bucket_id),
  CONSTRAINT work_item_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX work_item_tenant_id_uq ON work_item (tenant_id, id);
ALTER TABLE work_item ADD CONSTRAINT work_item_parent_id_fkey
    FOREIGN KEY (tenant_id, parent_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE;
CREATE INDEX wi_board_idx    ON work_item (tenant_id, collection_id, bucket_id, order_key)
  WHERE deleted_at IS NULL AND archived_at IS NULL;
CREATE INDEX wi_due_idx      ON work_item (tenant_id, collection_id, due_at)
  WHERE deleted_at IS NULL AND archived_at IS NULL AND is_completed = false;
-- The occurrences of one series: what an ON_COMPLETION series is waiting for, and when it was
-- last done (D-05).
CREATE INDEX wi_recurrence_idx ON work_item (recurrence_rule_id)
  WHERE recurrence_rule_id IS NOT NULL;
CREATE INDEX wi_due_announce_idx ON work_item (tenant_id, due_at)
  WHERE due_at IS NOT NULL
    AND deleted_at IS NULL AND archived_at IS NULL AND is_completed = false
    AND (due_soon_announced_at IS NULL OR overdue_announced_at IS NULL);
CREATE INDEX wi_assignee_idx ON work_item (tenant_id, assignee_id, is_completed, due_at)
  WHERE deleted_at IS NULL;
CREATE INDEX wi_parent_idx   ON work_item (tenant_id, parent_id, order_key);
CREATE INDEX wi_cover_media_idx ON work_item (tenant_id, cover_media_id) WHERE cover_media_id IS NOT NULL;
-- The plain item list of GET /items: one level of one collection, in its manual order. `id` last,
-- because the cursor is a keyset over (order_key, id) (B-04, api-guidelines.md §4).
CREATE INDEX wi_level_order_idx
  ON work_item (tenant_id, collection_id, parent_id, order_key COLLATE "C", id)
  WHERE deleted_at IS NULL;
-- The query language's default order: one whole collection, in its manual order. Without the
-- `parent_id` column of the index above, which a query spanning a collection does not constrain
-- (B-12, ADR-0026).
CREATE INDEX wi_query_order_idx
  ON work_item (tenant_id, collection_id, order_key COLLATE "C", id)
  WHERE deleted_at IS NULL;
CREATE INDEX wi_path_idx     ON work_item (tenant_id, path text_pattern_ops);
CREATE INDEX wi_search_idx   ON work_item USING gin (search_vector);
-- Every path that writes a row maintains the document, including the ones that are not use cases.
-- UPDATE is narrowed to the columns it is built from, so completing a task recomputes nothing.
CREATE TRIGGER work_item_search_document
  BEFORE INSERT OR UPDATE OF title, notes, content_language ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_search_document();
CREATE INDEX wi_search_document_idx ON work_item USING gin (search_document);
-- The supplement for the scripts a tsquery cannot serve: CJK and Thai have no word boundaries, so
-- one run of characters is one token and a substring of it is unfindable (i18n-l10n.md §5).
CREATE INDEX wi_search_trgm_idx ON work_item
  USING gin ((coalesce(title, '') || ' ' || coalesce(notes, '')) gin_trgm_ops);
CREATE INDEX wi_custom_idx   ON work_item USING gin (custom_fields jsonb_path_ops);
CREATE INDEX wi_trash_idx    ON work_item (tenant_id, deleted_at) WHERE deleted_at IS NOT NULL;
-- Restoring a deletion is one statement keyed on the batch every row of it shares (B-10, I-C2).
CREATE INDEX wi_trash_batch_idx ON work_item (tenant_id, trash_batch_id)
  WHERE trash_batch_id IS NOT NULL;

CREATE TABLE item_label (
  tenant_id uuid NOT NULL,
  item_id   uuid NOT NULL,
  label_id  uuid NOT NULL,
  PRIMARY KEY (item_id, label_id),
  CONSTRAINT item_label_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT item_label_label_id_fkey
    FOREIGN KEY (tenant_id, label_id) REFERENCES label (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX item_label_reverse_idx ON item_label (tenant_id, label_id, item_id);

CREATE TABLE item_member (
  tenant_id  uuid NOT NULL,
  item_id    uuid NOT NULL,
  account_id uuid NOT NULL,
  PRIMARY KEY (item_id, account_id),
  CONSTRAINT item_member_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT item_member_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX item_member_reverse_idx ON item_member (tenant_id, account_id, item_id);

CREATE TABLE custom_field_definition (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id uuid,   -- NULL = tenant-wide
  key           text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  kind          text NOT NULL CHECK (kind IN ('TEXT','NUMBER','DATE','SELECT','MULTI_SELECT','BOOL','USER','URL')),
  options       jsonb NOT NULL DEFAULT '[]'::jsonb,
  is_required   boolean NOT NULL DEFAULT false,
  applies_to    item_type[] NOT NULL DEFAULT '{TASK}',
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  version       integer     NOT NULL DEFAULT 1,
  deleted_at    timestamptz,
  CONSTRAINT custom_field_definition_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX custom_field_definition_tenant_id_uq ON custom_field_definition (tenant_id, id);
CREATE INDEX cfd_scope_idx ON custom_field_definition (tenant_id, collection_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX cfd_key_uq
  ON custom_field_definition (tenant_id, coalesce(collection_id, '00000000-0000-0000-0000-000000000000'::uuid), key)
  WHERE deleted_at IS NULL;

CREATE TABLE comment (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id           uuid NOT NULL,
  author_id         uuid NOT NULL,
  parent_comment_id uuid,
  -- Conditional since migration 0012: a tombstone's body is empty - deletion clears the text
  -- rather than hiding it (C-03) - while a living comment carries 1 to 20000 characters.
  body              text NOT NULL CHECK (deleted_at IS NOT NULL OR length(body) BETWEEN 1 AND 20000),
  created_at        timestamptz NOT NULL DEFAULT now(),
  edited_at         timestamptz,
  deleted_at        timestamptz,
  version           integer NOT NULL DEFAULT 1,
  CONSTRAINT comment_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX comment_tenant_id_uq ON comment (tenant_id, id);
ALTER TABLE comment ADD CONSTRAINT comment_parent_comment_id_fkey
    FOREIGN KEY (tenant_id, parent_comment_id) REFERENCES comment (tenant_id, id) ON DELETE CASCADE;
CREATE INDEX comment_item_idx ON comment (tenant_id, item_id, created_at);

CREATE TABLE activity_entry (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- nullable, and the key below is MATCH SIMPLE: container_id is there for a container's own
  -- history, which has no reader yet. An entry without an item is refused by the domain.
  item_id      uuid,
  container_id uuid,
  actor_type   text NOT NULL CHECK (actor_type IN ('USER','SERVICE_ACCOUNT','AUTOMATION','AI_AGENT','SYSTEM')),
  actor_id     uuid,
  verb         text NOT NULL,                    -- a message code, e.g. 'item.completed'
  change_set   jsonb NOT NULL DEFAULT '{}'::jsonb,
  occurred_at  timestamptz NOT NULL DEFAULT now(),
  correlation_id uuid,
  causation_id   uuid,
  -- The deletion path the data catalogue declares: the history goes with the item it is about.
  CONSTRAINT activity_entry_item_id_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
-- A page is read newest first within one item and continues after a boundary, so the tie-break
-- belongs in the index rather than in a sort.
CREATE INDEX activity_page_idx ON activity_entry (tenant_id, item_id, occurred_at DESC, id DESC);

CREATE TABLE media_object (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  storage_key text NOT NULL,
  mime_type   text NOT NULL,
  byte_size   bigint NOT NULL CHECK (byte_size >= 0),
  checksum    text,
  usage       text NOT NULL CHECK (usage IN ('COVER','ATTACHMENT','IMPORT','EXPORT')),
  ref_count   integer NOT NULL DEFAULT 0,
  -- NULL where nobody uploaded it: a mail attachment arrives over an intake that authenticates the
  -- tenant and no person, and an account here would be an uploader this system invented
  -- (migration 0061, G-11).
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  -- The upload life (C-06, migration 0013): PENDING between staging and confirmation, READY once
  -- the bytes were read back, judged and sealed. Fail-closed default.
  status      text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'READY')),
  file_name   text,
  -- Since when nothing has pointed at this object, NULL while something does (migration 0051).
  -- The recount maintains it, and the sweep waits it out before marking: an object is at
  -- ref_count = 0 between its confirmation and the first thing that uses it, and that window is
  -- not evidence of anything.
  unreferenced_since timestamptz
);
CREATE UNIQUE INDEX media_object_tenant_id_uq ON media_object (tenant_id, id);
CREATE INDEX media_object_reconcile_idx ON media_object (tenant_id, status, ref_count, created_at);
-- Declared here rather than in work_item's definition, because media_object is created after it
-- in this file; migration 0013 is where it arrived.
ALTER TABLE work_item ADD CONSTRAINT work_item_cover_media_fkey
  FOREIGN KEY (tenant_id, cover_media_id) REFERENCES media_object (tenant_id, id) ON DELETE RESTRICT;

CREATE TABLE item_attachment (
  tenant_id uuid NOT NULL,
  item_id   uuid NOT NULL,
  media_id  uuid NOT NULL,
  PRIMARY KEY (item_id, media_id),
  CONSTRAINT item_attachment_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT item_attachment_media_id_fkey
    FOREIGN KEY (tenant_id, media_id) REFERENCES media_object (tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX item_attachment_media_idx ON item_attachment (tenant_id, media_id);

-- ============================== Scheduling =================================

CREATE TABLE recurrence_rule (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  source_item_id uuid NOT NULL,
  rrule         text NOT NULL,                   -- RFC 5545
  time_zone     text NOT NULL,
  mode          text NOT NULL CHECK (mode IN ('ON_SCHEDULE','ON_COMPLETION')),
  horizon_days  integer NOT NULL DEFAULT 90,
  ends_at       timestamptz,
  max_count     integer,
  last_materialized_at timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz,
  version       integer NOT NULL DEFAULT 1,
  CONSTRAINT recurrence_rule_source_item_id_fkey
    FOREIGN KEY (tenant_id, source_item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
-- One series per entry: the entry points at its rule and the rule points back, and without this
-- the two could disagree about which rule an entry repeats by (D-04).
CREATE UNIQUE INDEX recurrence_rule_source_idx ON recurrence_rule (source_item_id);

CREATE TABLE reminder (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id      uuid NOT NULL,
  offset_spec  text NOT NULL,                    -- 'REL:-PT1H' | 'ABS:2026-09-01T08:00:00Z'
  channels     text[] NOT NULL DEFAULT '{EMAIL}',
  recipients   uuid[] NOT NULL DEFAULT '{}',     -- empty = assignee/members
  -- LAPSED is what a restore marks a reminder whose moment passed while the data was in an
  -- archive (migration 0037, backup-restore.md §8.4). Not CANCELLED: nobody cancelled it.
  state        text NOT NULL DEFAULT 'PENDING'
                 CHECK (state IN ('PENDING','SENT','CANCELLED','LAPSED')),
  fire_at      timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz,
  version      integer NOT NULL DEFAULT 1,
  CONSTRAINT reminder_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX reminder_due_idx ON reminder (tenant_id, state, fire_at);
CREATE INDEX reminder_item_idx ON reminder (item_id, created_at, id);

-- ======================== Views, Templates, Jumble =========================

CREATE TABLE saved_view (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  scope_type    text NOT NULL CHECK (scope_type IN ('TENANT','HUB','COLLECTION','ACCOUNT')),
  scope_id      uuid,
  owner_id      uuid,
  name          text NOT NULL,
  layout        text NOT NULL,                   -- LIST_COLLAPSED | LIST_EXPANDED | KANBAN | TIMELINE | …
  query         jsonb NOT NULL,
  grouping      jsonb NOT NULL DEFAULT '{}'::jsonb,
  visible_fields text[] NOT NULL DEFAULT '{}',
  sharing       text NOT NULL DEFAULT 'PRIVATE' CHECK (sharing IN ('PRIVATE','SCOPE','PUBLIC_LINK')),
  created_at    timestamptz NOT NULL DEFAULT now(),
  version       integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX saved_view_tenant_id_uq ON saved_view (tenant_id, id);

CREATE TABLE template (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  scope_type  text NOT NULL CHECK (scope_type IN ('TENANT','HUB','COLLECTION')),
  scope_id    uuid,
  name        text NOT NULL,
  description text,
  root_type   item_type NOT NULL DEFAULT 'TASK',
  nodes       jsonb NOT NULL,                    -- the tree including relative due dates (+P3D)
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz,
  deleted_at  timestamptz,
  version     integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX template_tenant_id_uq ON template (tenant_id, id);
-- Two live templates in one scope may not share a name; a deleted one frees it (D-06).
CREATE UNIQUE INDEX template_name_uq ON template (
    tenant_id, scope_type,
    coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid),
    name
  ) WHERE deleted_at IS NULL;
CREATE INDEX template_scope_idx ON template (tenant_id, scope_type, scope_id, created_at DESC)
  WHERE deleted_at IS NULL;

CREATE TABLE jumble_entry (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  channel      text NOT NULL CHECK (channel IN ('EMAIL','WEBHOOK','QUICK_CAPTURE','API')),
  sender       text,
  raw_subject  text,
  raw_body     text,
  attachments  uuid[] NOT NULL DEFAULT '{}',
  suggestion   jsonb,
  status       text NOT NULL DEFAULT 'NEW' CHECK (status IN ('NEW','PROCESSED','DISMISSED')),
  target_item_id uuid,
  received_at  timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz
);
CREATE INDEX jumble_status_idx ON jumble_entry (tenant_id, status, received_at DESC);

-- The jumble's webhook intake (G-10, migration 0060): one token-protected address per tenant,
-- stored as a hash under the intake's own purpose label. Rotating replaces it in one statement.
CREATE TABLE jumble_intake (
  tenant_id  uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL,
  rotated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX jumble_intake_token_uq ON jumble_intake (token_hash);

CREATE TABLE auto_assign_policy (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  scope_type  text NOT NULL CHECK (scope_type IN ('COLLECTION','HUB')),
  scope_id    uuid NOT NULL,
  strategy    text NOT NULL CHECK (strategy IN ('FIXED','RANDOM_MEMBER','RANDOM_GROUP_MEMBER','ROUND_ROBIN','LEAST_LOADED')),
  candidates  jsonb NOT NULL DEFAULT '[]'::jsonb,
  state       jsonb NOT NULL DEFAULT '{}'::jsonb,
  enabled     boolean NOT NULL DEFAULT true,
  version     integer NOT NULL DEFAULT 1
);
-- One policy per scope (C-02, migration 0011): the auto_assign key of one container's policies
-- document is one row, and the upsert that writes the key conflicts on this.
CREATE UNIQUE INDEX auto_assign_policy_scope_uq ON auto_assign_policy (tenant_id, scope_type, scope_id);

-- =============================== Automation ================================

CREATE TABLE automation_rule (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  scope_type  text NOT NULL CHECK (scope_type IN ('TENANT','HUB','COLLECTION')),
  scope_id    uuid,
  name        text NOT NULL,
  enabled     boolean NOT NULL DEFAULT true,
  run_as      uuid NOT NULL,
  trigger     jsonb NOT NULL,
  conditions  jsonb NOT NULL DEFAULT '[]'::jsonb,
  actions     jsonb NOT NULL,
  throttle    jsonb NOT NULL DEFAULT '{}'::jsonb,
  on_error    text NOT NULL DEFAULT 'STOP' CHECK (on_error IN ('STOP','CONTINUE','RETRY')),
  failure_count integer NOT NULL DEFAULT 0,
  created_by  uuid NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  version     integer NOT NULL DEFAULT 1,
  -- When a SCHEDULE rule next fires (G-08, migration 0055). NULL for the five triggers that are
  -- not a schedule, and for a schedule whose rule is exhausted.
  next_run_at timestamptz,
  -- The address an INBOUND_WEBHOOK rule answers on (G-08, migration 0057). D-08's discipline: the
  -- token is hashed, answered once, and revoked by rotating; the moment is the only thing about
  -- it a listing may show.
  inbound_token_hash bytea,
  inbound_rotated_at timestamptz,
  CONSTRAINT automation_rule_run_as_fkey
    FOREIGN KEY (tenant_id, run_as) REFERENCES account (tenant_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX automation_rule_tenant_id_uq ON automation_rule (tenant_id, id);
CREATE INDEX rule_trigger_idx ON automation_rule (tenant_id, enabled)
  WHERE deleted_at IS NULL;
-- What the dispatcher asks per event (G-07, migration 0053): the enabled rules whose trigger is
-- this event type. An expression index, because the trigger is a document - which is the right
-- shape, and this is what the shape costs.
CREATE INDEX rule_event_trigger_idx
  ON automation_rule (tenant_id, (trigger ->> 'kind'), (trigger ->> 'event_type'))
  WHERE deleted_at IS NULL AND enabled = true;
-- What the schedule pass asks: which of this tenant's rules are due. `backup_schedule_due_idx`'s
-- shape, because it is the same question - the tenant predicate is row level security's.
CREATE INDEX automation_rule_due_idx ON automation_rule (next_run_at)
  WHERE enabled AND deleted_at IS NULL AND next_run_at IS NOT NULL;
-- The lookup the unauthenticated inbound route makes. Unique across the installation, so a token
-- rewritten to quote another tenant matches nothing at all.
CREATE UNIQUE INDEX automation_rule_inbound_token_uq ON automation_rule (inbound_token_hash)
  WHERE inbound_token_hash IS NOT NULL;

CREATE TABLE rule_run (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  rule_id      uuid NOT NULL,
  event_id     uuid,
  -- What started the run (G-08, migration 0054). On the run rather than read back from the rule,
  -- because a rule can be edited from one kind into another and a log that resolved the kind at
  -- read time would rewrite its own history.
  trigger      text NOT NULL DEFAULT 'EVENT',
  -- Who pulled it, for the one kind a person pulls. No foreign key: a run has to outlive the
  -- account that started it, exactly as it outlives the rule it belongs to.
  triggered_by uuid,
  -- The entry the run is about when no event names it - a RELATIVE_DATE run measured from one
  -- entry's due date.
  subject_id   uuid,
  -- 'WAITING' is a run parked on a WAIT action (G-09, migration 0058): its results so far are
  -- written and a scheduled job holds the resume point. Its own status, because a row left in
  -- RUNNING is how a crash is recognised.
  status       text NOT NULL CHECK (status IN ('RUNNING','WAITING','SUCCEEDED','SKIPPED','FAILED','ABORTED_LOOP','THROTTLED')),
  condition_results jsonb NOT NULL DEFAULT '[]'::jsonb,
  action_results    jsonb NOT NULL DEFAULT '[]'::jsonb,
  -- What made this run one occurrence (G-09, migration 0059): the idempotency key's middle third,
  -- kept so a replay can complete a half-finished run around the keys its actions claimed. NULL
  -- on rows written before the column existed.
  occasion     text,
  error_code   text,
  started_at   timestamptz NOT NULL DEFAULT now(),
  finished_at  timestamptz,
  causation_depth integer NOT NULL DEFAULT 0,
  CONSTRAINT rule_run_rule_id_fkey
    FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT rule_run_trigger_check
    CHECK (trigger IN ('EVENT','SCHEDULE','RELATIVE_DATE','INBOUND_WEBHOOK','MANUAL','JUMBLE_ENTRY'))
);
CREATE INDEX rule_run_idx ON rule_run (tenant_id, rule_id, started_at DESC);
-- What the run listing's trigger filter asks: this tenant's runs of one kind, newest first.
CREATE INDEX rule_run_trigger_idx ON rule_run (tenant_id, trigger, id DESC);

-- What a RELATIVE_DATE rule owes for one entry, and when (G-08, migration 0056). D-02's shape
-- rather than a new one: `reminder` has carried "this entry, this moment" since phase 0, and a
-- relative-date rule is the same fact with a rule in place of a person.
CREATE TABLE rule_occurrence (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  rule_id    uuid NOT NULL,
  item_id    uuid NOT NULL,
  fire_at    timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT rule_occurrence_rule_id_fkey
    FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT rule_occurrence_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
-- One moment per rule per entry: a rule that owed two would fire twice for one deadline, and the
-- upsert that keeps the moment in step with the anchor conflicts on this.
CREATE UNIQUE INDEX rule_occurrence_uq ON rule_occurrence (tenant_id, rule_id, item_id);
CREATE INDEX rule_occurrence_due_idx ON rule_occurrence (tenant_id, fire_at);

-- ============================== Integration ================================

CREATE TABLE webhook_subscription (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  target_url    text NOT NULL,
  event_types   text[] NOT NULL,
  filter_expr   text,
  secret_enc    bytea NOT NULL,
  state         text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE','PAUSED','DISABLED')),
  failure_count integer NOT NULL DEFAULT 0,
  -- The key a sealed value opens under. An installation that has rotated its keyring holds
  -- several, so the ciphertext alone is not enough (E-02).
  secret_key_id         text,
  -- The rotation grace (G-03): one previous secret, verifying until this moment. A pair rather
  -- than a table, because a history of retired secrets is a history of values that must not be
  -- readable.
  previous_secret_enc    bytea,
  previous_secret_key_id text,
  previous_secret_until  timestamptz,
  -- The message code of the last failure, never a response body from the target (rule 10).
  last_error    text,
  -- When unreachability disabled it. Separate from the state, which goes back to ACTIVE on a
  -- re-enable and then cannot say when the trouble was.
  disabled_at   timestamptz,
  created_by    uuid NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  version       integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX webhook_subscription_tenant_id_uq ON webhook_subscription (tenant_id, id);

CREATE TABLE webhook_delivery (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  subscription_id uuid NOT NULL,
  event_id        uuid NOT NULL,
  attempt         integer NOT NULL DEFAULT 1,
  status          text NOT NULL CHECK (status IN ('PENDING','SUCCEEDED','FAILED','DEAD_LETTER')),
  response_status integer,
  error_code      text,
  next_attempt_at timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT webhook_delivery_subscription_id_fkey
    FOREIGN KEY (tenant_id, subscription_id) REFERENCES webhook_subscription (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX delivery_retry_idx ON webhook_delivery (tenant_id, status, next_attempt_at);
CREATE INDEX webhook_delivery_subscription_idx ON webhook_delivery (tenant_id, subscription_id, created_at DESC, id DESC);

CREATE TABLE calendar_feed (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id  uuid NOT NULL,
  view_id     uuid,
  token_hash  bytea NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  revoked_at  timestamptz,
  CONSTRAINT calendar_feed_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT calendar_feed_view_id_fkey
    FOREIGN KEY (tenant_id, view_id) REFERENCES saved_view (tenant_id, id) ON DELETE SET NULL (view_id)
);
CREATE UNIQUE INDEX calendar_feed_token_uq ON calendar_feed (token_hash);
CREATE INDEX calendar_feed_account_idx ON calendar_feed (tenant_id, account_id, created_at DESC, id DESC);

-- ============================== Notification ===============================

-- What somebody is to be told, and how far that got (C-09). References and no content: what an
-- email says is read from the entry when it is rendered, never copied here.
CREATE TABLE notification (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  recipient_id uuid NOT NULL,
  -- RETENTION since migration 0062: the advance warning of data-retention.md §6 (R-1).
  category     text NOT NULL CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION')),
  channel      text NOT NULL CHECK (channel IN ('EMAIL')),
  state        text NOT NULL CHECK (state IN ('PENDING','SENT','SUPPRESSED','FAILED')),
  reason       text,                             -- a detail code, never a sentence (rule 8)
  event_id     uuid,                             -- null for the invitation, which is not an event
  item_id      uuid,
  actor_id     uuid,
  created_at   timestamptz NOT NULL DEFAULT now(),
  sent_at      timestamptz,
  attempts     integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),

  CONSTRAINT notification_recipient_id_fkey
    FOREIGN KEY (tenant_id, recipient_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_actor_id_fkey
    FOREIGN KEY (tenant_id, actor_id) REFERENCES account (tenant_id, id)
      ON DELETE SET NULL (actor_id)
);
-- Partial: the invitation carries no event, and NULLs are all distinct to a unique index.
CREATE UNIQUE INDEX notification_event_recipient_idx
  ON notification (tenant_id, event_id, recipient_id, channel)
  WHERE event_id IS NOT NULL;
CREATE INDEX notification_pending_idx
  ON notification (tenant_id, created_at) WHERE state = 'PENDING';
CREATE INDEX notification_retention_idx ON notification (tenant_id, created_at);

-- What a person has said about being told. A row is an exception; its absence is the default, on.
CREATE TABLE notification_preference (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id    uuid NOT NULL,
  category      text NOT NULL CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION')),
  channel       text NOT NULL CHECK (channel IN ('EMAIL')),
  enabled       boolean NOT NULL DEFAULT true,
  include_title boolean NOT NULL DEFAULT true,   -- data-protection.md §9: the minimum is switchable
  updated_at    timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (tenant_id, account_id, category, channel),
  CONSTRAINT notification_preference_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- ========================= Events, jobs, idempotency =======================

CREATE TABLE outbox_event (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL,
  event_type      text NOT NULL,                 -- de.hubtask.work.item.created.v1
  subject         text,
  payload         jsonb NOT NULL,
  actor_type      text NOT NULL,
  actor_id        uuid,
  correlation_id  uuid,
  causation_id    uuid,
  causation_depth integer NOT NULL DEFAULT 0,
  occurred_at     timestamptz NOT NULL DEFAULT now(),
  dispatched_at   timestamptz,
  attempts        integer NOT NULL DEFAULT 0,
  locked_until    timestamptz,
  -- A change a restore wrote rather than one somebody made (backup-restore.md §8.4, migration
  -- 0033). Outward-facing subscribers are not given these: a restore would otherwise report last
  -- month's states to every webhook and every rule.
  replay          boolean NOT NULL DEFAULT false
);
CREATE INDEX outbox_pending_idx ON outbox_event (occurred_at)
  WHERE dispatched_at IS NULL;
-- The polling trigger's walk (G-04, migration 0052): one type, in the outbox's own order. The
-- tenant leads because row level security puts it in front of every predicate; the ordering pair
-- comes last so a page is a range read rather than a sort. Partial, because a poll never answers a
-- replayed event.
CREATE INDEX outbox_poll_idx ON outbox_event (tenant_id, event_type, occurred_at, id)
  WHERE replay = false;

CREATE TABLE job (
  id           uuid PRIMARY KEY,
  tenant_id    uuid,
  kind         text NOT NULL,
  payload      jsonb NOT NULL DEFAULT '{}'::jsonb,
  dedupe_key   text,
  run_at       timestamptz NOT NULL DEFAULT now(),
  state        text NOT NULL DEFAULT 'PENDING' CHECK (state IN ('PENDING','RUNNING','SUCCEEDED','FAILED','DEAD_LETTER','CANCELLED')),
  attempts     integer NOT NULL DEFAULT 0,
  max_attempts integer NOT NULL DEFAULT 8,
  locked_until timestamptz,
  last_error   text,
  priority     smallint NOT NULL DEFAULT 5,
  created_at   timestamptz NOT NULL DEFAULT now(),
  finished_at  timestamptz,
  -- How far along, between 0 and 1, or NULL from a job that cannot say (migration 0032).
  progress     real
);
CREATE INDEX job_pickup_idx ON job (state, run_at, priority);
CREATE UNIQUE INDEX job_dedupe_uq ON job (kind, dedupe_key)
  WHERE dedupe_key IS NOT NULL AND state IN ('PENDING','RUNNING');

-- What a subscriber has already seen. The outbox delivers at-least-once, so a consumer asks here
-- before it reacts; the insert is the question (ADR-0007, db/migrations/0003).
CREATE TABLE event_consumption (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  consumer    text NOT NULL,
  event_id    uuid NOT NULL,
  consumed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, consumer, event_id)
);
CREATE INDEX event_consumption_gc_idx ON event_consumption (consumed_at);

CREATE TABLE idempotency_key (
  tenant_id     uuid NOT NULL,
  key           text NOT NULL,
  endpoint      text NOT NULL,
  request_hash  bytea NOT NULL,
  response_code integer,
  response_body jsonb,
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, key, endpoint)
);
CREATE INDEX idempotency_gc_idx ON idempotency_key (created_at);

CREATE TABLE usage_record (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  period     date NOT NULL,
  metric     text NOT NULL,
  value      bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, period, metric)
);


-- ===================== Audit, data protection, compliance ===================
-- See docs/architecture/audit.md and docs/architecture/data-protection.md.

-- The audit trail: append-only, a hashed chain per tenant, no content.
-- Deliberately WITHOUT foreign keys to account/work_item: entries must survive deletions,
-- which is why actor_label/target_label are denormalised.
CREATE TABLE audit_log (
  id              uuid NOT NULL,
  tenant_id       uuid NOT NULL,
  seq             bigint NOT NULL,                  -- gapless per tenant
  occurred_at     timestamptz NOT NULL DEFAULT now(),
  action          text NOT NULL,                    -- 'item.deleted', 'auth.login_failed', ...
  outcome         text NOT NULL CHECK (outcome IN ('SUCCESS','DENIED','FAILED')),
  severity        text NOT NULL DEFAULT 'INFO'
                    CHECK (severity IN ('INFO','NOTICE','WARNING','CRITICAL')),
  actor_type      text NOT NULL
                    CHECK (actor_type IN ('USER','SERVICE_ACCOUNT','AUTOMATION','AI_AGENT','SYSTEM')),
  actor_id        uuid,
  actor_label     text,                             -- the label at the time of the event
  on_behalf_of_id uuid,                             -- the run_as principal for automation/agents
  target_type     text,
  target_id       uuid,
  target_label    text,
  context         jsonb NOT NULL DEFAULT '{}'::jsonb, -- request_id, trace_id, ip_truncated, ...
  changes         jsonb NOT NULL DEFAULT '{}'::jsonb, -- masked per the field classification
  legal_basis     text,                             -- e.g. 'dsr.erasure'
  prev_hash       bytea,
  hash            bytea NOT NULL,
  -- The partition key must be part of every unique constraint.
  PRIMARY KEY (tenant_id, occurred_at, seq)
) PARTITION BY RANGE (occurred_at);

-- The initial partitions; the retention/maintenance job creates further ones.
CREATE TABLE audit_log_2026_08 PARTITION OF audit_log
  FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE audit_log_default PARTITION OF audit_log DEFAULT;

CREATE UNIQUE INDEX audit_id_uq   ON audit_log (tenant_id, occurred_at, id);
-- The absence of gaps in the chain per tenant is additionally checked in the application
-- (audit:verify), because a global UNIQUE (tenant_id, seq) across partitions
-- cannot be enforced.
CREATE UNIQUE INDEX audit_seq_uq  ON audit_log (tenant_id, occurred_at, seq);
-- The chain's tail is read by sequence number and nothing else (0045), and every other index here
-- leads with occurred_at - so without this one that read is a scan of every partition.
CREATE INDEX audit_seq_idx        ON audit_log (tenant_id, seq DESC);
CREATE INDEX audit_time_idx       ON audit_log (tenant_id, occurred_at DESC);
CREATE INDEX audit_action_idx     ON audit_log (tenant_id, action, occurred_at DESC);
CREATE INDEX audit_actor_idx      ON audit_log (tenant_id, actor_id, occurred_at DESC);
CREATE INDEX audit_target_idx     ON audit_log (tenant_id, target_id, occurred_at DESC);

-- Immutability, level 2 (level 1 = the absent GRANTs, level 3 = the hash chain).
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only (attempted %)', TG_OP
    USING ERRCODE = 'insufficient_privilege';
END $$;

CREATE TRIGGER audit_log_no_update BEFORE UPDATE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
-- Note: the retention job removes whole partitions only (DETACH/DROP),
-- never individual rows. Creating and removing partitions happens with the
-- maintenance role, not with hubtask_app.

-- External anchoring of the hash chain (optional, see audit.md §3).
CREATE TABLE audit_anchor (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  anchored_at timestamptz NOT NULL DEFAULT now(),
  last_seq    bigint NOT NULL,
  chain_hash  bytea NOT NULL,
  destination text,                                 -- 'worm_bucket', 'mail', ...
  receipt     text,
  PRIMARY KEY (tenant_id, last_seq)
);

-- Retention periods are data, not code.
CREATE TABLE retention_policy (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  data_kind     text NOT NULL,                      -- 'trash','session','audit','rule_run', ...
  retain_days   integer NOT NULL CHECK (retain_days >= 0),
  min_days      integer NOT NULL DEFAULT 0,         -- the documented lower bound
  max_days      integer,                            -- the documented upper bound
  justification text,                               -- mandatory when extending beyond the default
  updated_at    timestamptz NOT NULL DEFAULT now(),
  updated_by    uuid,
  PRIMARY KEY (tenant_id, data_kind),
  CHECK (max_days IS NULL OR retain_days <= max_days),
  CHECK (retain_days >= min_days)
);

-- Data subject requests (GDPR Art. 15-21) as a tracked case with a deadline.
CREATE TABLE data_subject_request (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  subject_account_id uuid,                          -- may be NULL once fulfilled
  subject_email  text,                              -- for requests without an account
  kind           text NOT NULL
                   CHECK (kind IN ('ACCESS','ERASURE','PORTABILITY','RESTRICTION','OBJECTION','RECTIFICATION')),
  status         text NOT NULL DEFAULT 'RECEIVED'
                   CHECK (status IN ('RECEIVED','IN_PROGRESS','COMPLETED','REJECTED')),
  erasure_mode   text CHECK (erasure_mode IN ('ANONYMIZE','FULL_DELETE')),
  received_at    timestamptz NOT NULL DEFAULT now(),
  due_at         timestamptz NOT NULL,              -- the statutory deadline, +30 days by default
  completed_at   timestamptz,
  handled_by     uuid,
  rejection_reason text,
  result_media_id  uuid,                            -- unused; the export lands at a backup target
  scope          text NOT NULL DEFAULT 'TENANT'
                   CHECK (scope IN ('TENANT','INSTALLATION')),
  target_id      uuid,                              -- the backup target an export is written to
  result_archive text,                              -- where the archive landed there
  notes          text
);
CREATE INDEX dsr_open_idx ON data_subject_request (tenant_id, status, due_at)
  WHERE status IN ('RECEIVED','IN_PROGRESS');

-- The pseudonyms an erasure leaves behind for the audit trail. The trail cannot be edited in place
-- - the grants, the trigger and the hash chain all refuse it - so the substitution happens at the
-- boundary and this is what the boundary reads (audit.md §6, E-10).
CREATE TABLE audit_pseudonym (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  actor_id    uuid NOT NULL,
  pseudonym   text NOT NULL,
  reason      text NOT NULL DEFAULT 'DSR_ERASURE'
                CHECK (reason IN ('DSR_ERASURE','ADMIN')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, actor_id)
);

-- Consents for optional processing (AI, metering, notification channels).
CREATE TABLE consent_record (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id   uuid,
  purpose      text NOT NULL,                       -- 'ai_processing','metering','email_content'
  granted      boolean NOT NULL,
  granted_at   timestamptz NOT NULL DEFAULT now(),
  revoked_at   timestamptz,
  source       text,                                -- 'user','tenant_admin','config'
  UNIQUE (tenant_id, account_id, purpose, granted_at),
  CONSTRAINT consent_record_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- Documented data breaches (Art. 33/34) - evidence, not automated notification.
CREATE TABLE privacy_incident (
  id             uuid PRIMARY KEY,
  tenant_id      uuid REFERENCES tenant(id) ON DELETE SET NULL,  -- NULL = installation-wide
  detected_at    timestamptz NOT NULL,
  reported_at    timestamptz,
  severity       text NOT NULL CHECK (severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
  data_categories text[] NOT NULL DEFAULT '{}',
  affected_count integer,
  description    text NOT NULL,
  measures       text,
  authority_notified boolean NOT NULL DEFAULT false,
  subjects_notified  boolean NOT NULL DEFAULT false,
  closed_at      timestamptz
);


-- =========================== Backup & Restore ==============================
-- Targets are the operator's business (see ADR-0019); tenant-owned targets are optional.

CREATE TABLE backup_target (
  id            uuid PRIMARY KEY,
  tenant_id     uuid REFERENCES tenant(id) ON DELETE CASCADE,   -- NULL = instance-wide
  name          text NOT NULL,
  kind          text NOT NULL
                  CHECK (kind IN ('LOCAL','S3','SFTP','FTPS','FTP','WEBDAV','SMB',
                                  'AZURE_BLOB','GCS','RCLONE','HTTP_PUT')),
  config        jsonb NOT NULL DEFAULT '{}'::jsonb,   -- endpoint, bucket, path, region
  credential_enc bytea,                               -- AES-256-GCM, envelope (the key ID is in config)
  credential_key_id text,
  encryption_mode text NOT NULL DEFAULT 'AES256_GCM'
                  CHECK (encryption_mode IN ('AES256_GCM','NONE')),
  encryption_key_id text,
  region_note   text,                                 -- data residency, GDPR chapter V
  insecure_ack_by uuid,                               -- the confirmation for FTP/unencrypted
  insecure_ack_at timestamptz,
  enabled       boolean NOT NULL DEFAULT true,
  last_test_at  timestamptz,
  last_test_ok  boolean,
  last_test_error text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  created_by    uuid,
  version       integer NOT NULL DEFAULT 1,
  CHECK (encryption_mode <> 'NONE' OR insecure_ack_at IS NOT NULL),
  CHECK (kind <> 'FTP' OR insecure_ack_at IS NOT NULL)
);
CREATE UNIQUE INDEX backup_target_name_uq
  ON backup_target (coalesce(tenant_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));

CREATE TABLE backup_schedule (
  id            uuid PRIMARY KEY,
  target_id     uuid NOT NULL REFERENCES backup_target(id) ON DELETE CASCADE,
  tenant_id     uuid REFERENCES tenant(id) ON DELETE CASCADE,  -- NULL = a system backup
  scope_kind    text NOT NULL CHECK (scope_kind IN ('INSTANCE','TENANT','HUB','COLLECTION')),
  scope_id      uuid,
  rrule         text NOT NULL,                        -- RFC 5545, the same engine as recurrence
  time_zone     text NOT NULL DEFAULT 'UTC',
  mode          text NOT NULL DEFAULT 'INCREMENTAL' CHECK (mode IN ('FULL','INCREMENTAL')),
  full_rrule    text,                                 -- e.g. a weekly full backup
  include_media boolean NOT NULL DEFAULT true,
  include_audit boolean NOT NULL DEFAULT true,
  retention     jsonb NOT NULL
                  DEFAULT '{"keep_last":7,"keep_daily":14,"keep_weekly":8,
                            "keep_monthly":12,"keep_yearly":3,"min_keep":3}'::jsonb,
  notify_on     text[] NOT NULL DEFAULT ARRAY['FAILURE'],
  enabled       boolean NOT NULL DEFAULT true,
  next_run_at   timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  version       integer NOT NULL DEFAULT 1,
  CHECK ((scope_kind = 'INSTANCE') = (tenant_id IS NULL))
);
CREATE INDEX backup_schedule_due_idx ON backup_schedule (next_run_at) WHERE enabled;

-- One run. The authoritative truth about existing archives is the manifest at the target;
-- this table is a log and an accelerator, not a prerequisite for a restore.
CREATE TABLE backup_run (
  id            uuid PRIMARY KEY,
  schedule_id   uuid REFERENCES backup_schedule(id) ON DELETE SET NULL,
  target_id     uuid NOT NULL REFERENCES backup_target(id) ON DELETE CASCADE,
  tenant_id     uuid REFERENCES tenant(id) ON DELETE CASCADE,
  parent_run_id uuid REFERENCES backup_run(id) ON DELETE SET NULL,   -- the chain when incremental
  -- PRE_DELETE is the export a retention rule takes before it removes anything
  -- (data-retention.md §6, migration 0039).
  trigger       text NOT NULL
                  CHECK (trigger IN ('SCHEDULE','MANUAL','PRE_RESTORE','PRE_DELETE','API')),
  mode          text NOT NULL CHECK (mode IN ('FULL','INCREMENTAL')),
  status        text NOT NULL DEFAULT 'RUNNING'
                  CHECK (status IN ('RUNNING','SUCCEEDED','FAILED','CANCELLED','EXPIRED')),
  archive_path  text,                                 -- the path/key at the target
  manifest      jsonb,                                -- a copy of the manifest
  size_bytes    bigint,
  item_count    integer,
  media_count   integer,
  checksum      text,
  snapshot_at   timestamptz,                          -- the consistency point (REPEATABLE READ)
  started_at    timestamptz NOT NULL DEFAULT now(),
  finished_at   timestamptz,
  error_code    text,
  expires_at    timestamptz,                          -- from the retention plan
  verified_at   timestamptz,
  verify_ok     boolean
);
CREATE INDEX backup_run_target_idx ON backup_run (target_id, started_at DESC);
CREATE INDEX backup_run_expiry_idx ON backup_run (expires_at) WHERE status = 'SUCCEEDED';

CREATE TABLE restore_run (
  id             uuid PRIMARY KEY,
  target_id      uuid NOT NULL REFERENCES backup_target(id) ON DELETE RESTRICT,
  source_archive text NOT NULL,                       -- the path at the target, possible even without a backup_run
  -- The tenant that asked, and the one the row is visible in. The tenant being restored *into* is
  -- target_tenant_id below: they differ only for NEW_TENANT, whose target did not exist when the
  -- restore was asked for (migration 0034).
  tenant_id      uuid REFERENCES tenant(id) ON DELETE CASCADE,
  target_tenant_id uuid,
  mode           text NOT NULL
                   CHECK (mode IN ('INSPECT','SELECTIVE','MERGE','REPLACE_TENANT','NEW_TENANT','INSTANCE')),
  conflict_rule  text NOT NULL DEFAULT 'SKIP' CHECK (conflict_rule IN ('SKIP','OVERWRITE','DUPLICATE')),
  selection      jsonb,                               -- the container/item selection for SELECTIVE
  dry_run        boolean NOT NULL DEFAULT true,
  -- Whether §8.3 step 4's copy is wanted. Declinable, because the step's own parenthesis is "if
  -- there is room at the target" (migration 0035).
  create_safety_backup boolean NOT NULL DEFAULT true,
  safety_backup_run_id uuid REFERENCES backup_run(id),
  status         text NOT NULL DEFAULT 'PENDING'
                   CHECK (status IN ('PENDING','VALIDATING','RUNNING','SUCCEEDED','FAILED','CANCELLED')),
  report         jsonb,                               -- new/overwritten/skipped/conflicts
  -- How far the run got, per entity, written with each batch. A resumed attempt skips what has
  -- already been decided; the archive is immutable, so "the first N records" names the same N
  -- every time (migration 0036).
  progress       jsonb,
  requested_by   uuid NOT NULL,
  approved_by    uuid,
  started_at     timestamptz,
  finished_at    timestamptz,
  error_code     text
);
CREATE INDEX restore_run_tenant_idx ON restore_run (tenant_id, started_at DESC);

-- The deletion journal: prevents deleted objects returning through a restore.
CREATE TABLE deletion_journal (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entity       text NOT NULL,
  entity_id    uuid NOT NULL,
  deleted_at   timestamptz NOT NULL DEFAULT now(),
  reason       text NOT NULL CHECK (reason IN ('USER','RETENTION','DSR_ERASURE','ADMIN')),
  PRIMARY KEY (tenant_id, entity, entity_id)
);
CREATE INDEX deletion_journal_time_idx ON deletion_journal (tenant_id, deleted_at);

-- The marked objects are the few, and phase two reads this on every pass (migration 0038).
CREATE INDEX work_item_retention_idx ON work_item (tenant_id, retention_pending_until)
  WHERE retention_pending_until IS NOT NULL;

-- The rule model of data-retention.md §2 (migration 0038). It sits beside retention_policy rather
-- than replacing it: that table's key allows one period per kind per tenant, and a scoped model
-- needs two rows for one kind. The old rows are carried into this table by the first sweep after
-- the upgrade, and a later release contracts the old one away.
CREATE TABLE retention_rule (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  scope_kind      text NOT NULL CHECK (scope_kind IN ('TENANT','HUB','COLLECTION')),
  scope_id        uuid,                              -- NULL exactly when the scope is the tenant
  data_kind       text NOT NULL,                     -- the catalogue of §3; no constraint, see 0038
  condition       text,                              -- CEL, stored and not yet evaluated (0.5.0)
  retain_days     integer NOT NULL CHECK (retain_days >= 0),
  action          text NOT NULL
                    CHECK (action IN ('ARCHIVE','TRASH','ANONYMIZE','HARD_DELETE',
                                      'EXPORT_THEN_DELETE','NOTIFY_ONLY')),
  then_after_days integer CHECK (then_after_days IS NULL OR then_after_days >= 0),
  then_action     text CHECK (then_action IN ('ARCHIVE','TRASH','ANONYMIZE','HARD_DELETE',
                                              'EXPORT_THEN_DELETE')),
  grace_days      integer NOT NULL DEFAULT 14 CHECK (grace_days >= 0),
  notify          jsonb NOT NULL DEFAULT '{}'::jsonb,
  justification   text,                              -- mandatory beyond the kind's upper bound
  enabled         boolean NOT NULL DEFAULT true,
  export_target_id uuid REFERENCES backup_target(id) ON DELETE RESTRICT,
  created_by      uuid,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  version         integer NOT NULL DEFAULT 1,
  CONSTRAINT retention_rule_scope_check CHECK ((scope_kind = 'TENANT') = (scope_id IS NULL)),
  CONSTRAINT retention_rule_chain_check CHECK ((then_after_days IS NULL) = (then_action IS NULL)),
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX retention_rule_scope_idx ON retention_rule
  (tenant_id, data_kind, scope_kind,
   coalesce(scope_id, '00000000-0000-0000-0000-000000000000'::uuid));
CREATE INDEX retention_rule_lookup_idx
  ON retention_rule (tenant_id, data_kind) WHERE enabled;

-- The log of retention runs (the rules themselves: retention_rule, above).
CREATE TABLE retention_run (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  policy_id     uuid,
  data_kind     text NOT NULL,
  phase         text NOT NULL CHECK (phase IN ('MARK','EXECUTE','PREVIEW')),
  matched       integer NOT NULL DEFAULT 0,
  affected      integer NOT NULL DEFAULT 0,
  blocked       integer NOT NULL DEFAULT 0,
  blocked_reasons jsonb NOT NULL DEFAULT '{}'::jsonb,  -- legal_hold, restriction, tombstone_window
  started_at    timestamptz NOT NULL DEFAULT now(),
  finished_at   timestamptz,
  status        text NOT NULL DEFAULT 'RUNNING'
                  CHECK (status IN ('RUNNING','SUCCEEDED','FAILED'))
);
CREATE INDEX retention_run_idx ON retention_run (tenant_id, data_kind, started_at DESC);

CREATE TABLE legal_hold (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  scope_kind   text NOT NULL CHECK (scope_kind IN ('TENANT','CONTAINER','ITEM','ACCOUNT')),
  scope_id     uuid,
  reason       text NOT NULL,
  placed_by    uuid NOT NULL,
  placed_at    timestamptz NOT NULL DEFAULT now(),
  released_by  uuid,
  released_at  timestamptz,
  -- Why it was lifted (migration 0040). Both ends of a hold are decisions, and only the placing
  -- was recorded.
  released_reason text
);
CREATE INDEX legal_hold_active_idx ON legal_hold (tenant_id, scope_kind, scope_id)
  WHERE released_at IS NULL;

-- ============================ Sync (offline) ===============================

-- The monotonic change sequence per tenant. The basis for :pull; separate from outbox_event,
-- because the recipients, retention, and compatibility commitments differ (ADR-0021).
CREATE TABLE change_log (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  seq          bigint GENERATED ALWAYS AS IDENTITY,
  entity       text NOT NULL,
  entity_id    uuid NOT NULL,
  op           text NOT NULL CHECK (op IN ('UPSERT','DELETE','ACCESS_REVOKED')),
  container_id uuid,                                  -- the visibility filter on pull
  actor_id     uuid,
  device_id    uuid,
  hlc          text NOT NULL,                         -- physical:counter:device
  occurred_at  timestamptz NOT NULL DEFAULT now(),
  payload      jsonb,                                 -- the changed fields; NULL on DELETE
  -- The partition key belongs in the key; the cursor stays logical (tenant_id, seq).
  PRIMARY KEY (tenant_id, occurred_at, seq)
) PARTITION BY RANGE (occurred_at);

CREATE TABLE change_log_2026_08 PARTITION OF change_log
  FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE change_log_default PARTITION OF change_log DEFAULT;
CREATE INDEX change_log_pull_idx ON change_log (tenant_id, seq) INCLUDE (entity, entity_id, op);

-- The wake-up for the change stream (C-10, ADR-0007). A trigger rather than a NOTIFY in the
-- application: every path that records a change would otherwise have to remember to announce it,
-- and the one that forgets produces a change no connected client is told about. The payload is the
-- tenant and nothing else - a doorbell, not a letter (rule 10).
CREATE OR REPLACE FUNCTION hubtask_notify_change() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  PERFORM pg_notify('hubtask_change', NEW.tenant_id::text);
  RETURN NULL;
END $$;

-- On the parent, so that partitions created later carry it too.
CREATE TRIGGER change_log_notify
  AFTER INSERT ON change_log
  FOR EACH ROW EXECUTE FUNCTION hubtask_notify_change();

-- The wake-up for the dispatcher (G-02, ADR-0007). The queue rather than outbox_event: an event is
-- written together with its dispatch job in one transaction, so the row the worker waits for is the
-- job. No payload - `job` has no tenant column and none is needed, and an empty payload is what
-- lets PostgreSQL collapse a transaction that enqueued five jobs into one ring of the bell.
CREATE OR REPLACE FUNCTION hubtask_notify_job() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  PERFORM pg_notify('hubtask_job', '');
  RETURN NULL;
END $$;

CREATE TRIGGER job_notify
  AFTER INSERT ON job
  FOR EACH ROW EXECUTE FUNCTION hubtask_notify_job();
CREATE INDEX change_log_container_idx ON change_log (tenant_id, container_id, seq);

-- Deletion markers with a minimum lifetime: a hard delete is only allowed after it elapses,
-- otherwise devices that were offline for a long time recreate deleted objects.
CREATE TABLE tombstone (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entity       text NOT NULL,
  entity_id    uuid NOT NULL,
  deleted_at   timestamptz NOT NULL DEFAULT now(),
  purge_after  timestamptz NOT NULL,                  -- deleted_at + the offline window
  PRIMARY KEY (tenant_id, entity, entity_id)
);
CREATE INDEX tombstone_purge_idx ON tombstone (purge_after);

CREATE TABLE sync_device (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id    uuid NOT NULL,
  platform      text,
  display_name  text,
  last_cursor   bigint,
  last_seen_at  timestamptz,
  scopes        jsonb NOT NULL DEFAULT '[]'::jsonb,   -- the subscribed containers
  push_token    text,
  blocked       boolean NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sync_device_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX sync_device_account_idx ON sync_device (tenant_id, account_id);

-- Idempotency of the push mutations (30 days).
CREATE TABLE sync_op_log (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  op_id        uuid NOT NULL,
  device_id    uuid,
  result       text NOT NULL CHECK (result IN ('APPLIED','MERGED','REJECTED','CONFLICT')),
  entity_id    uuid,
  applied_at   timestamptz NOT NULL DEFAULT now(),
  response     jsonb,
  PRIMARY KEY (tenant_id, op_id)
);
CREATE INDEX sync_op_log_ttl_idx ON sync_op_log (applied_at);

-- OR-set tags for set fields (labels, members, attachments): additions and removals each carry
-- their own tag, so that a concurrent add/remove does not discard whole lists through LWW.
CREATE TABLE set_element (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id      uuid NOT NULL,
  set_name     text NOT NULL CONSTRAINT set_element_set_name_known
    CHECK (set_name IN ('labels','members','watchers','attachments')),
  element_id   uuid NOT NULL,
  add_tag      text,                                  -- the HLC of the addition
  remove_tag   text,                                  -- the HLC of the removal
  PRIMARY KEY (tenant_id, item_id, set_name, element_id)
);

-- ============================ Row Level Security ===========================
-- For every tenant-scoped table: a policy on current_tenant_id().
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'account','account_group','account_group_member','membership','access_token',
    'session','session_refresh_token','auth_attempt',
    'account_mfa','account_recovery_code','auth_pending',
    'container','bucket','label','work_item','item_label','item_member',
    'custom_field_definition','comment','activity_entry','media_object','item_attachment',
    'recurrence_rule','reminder','saved_view','template','jumble_entry','auto_assign_policy',
    'automation_rule','rule_run','rule_occurrence','jumble_intake',
    'webhook_subscription','webhook_delivery','calendar_feed',
    'outbox_event','event_consumption','idempotency_key','usage_record',
    'notification','notification_preference',
    'audit_anchor','audit_pseudonym','retention_policy','data_subject_request','consent_record',
    'backup_schedule','backup_run','restore_run','deletion_journal','retention_run',
    'retention_rule',
    'legal_hold','tombstone','sync_device','sync_op_log','set_element'
  ]
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format($f$
      CREATE POLICY tenant_isolation ON %I
        USING (tenant_id = current_tenant_id())
        WITH CHECK (tenant_id = current_tenant_id())
    $f$, t);
  END LOOP;
END $$;

-- tenant itself: access only to its own row (the admin role deliberately does not bypass this;
-- tenant administration runs through the control plane role).
ALTER TABLE tenant ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_self ON tenant
  USING (id = current_tenant_id())
  WITH CHECK (id = current_tenant_id());

-- audit_log: RLS on the partitioned table. A partition addressed directly is NOT covered by
-- the parent policy - see the partition block further down.
ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_log
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- privacy_incident can be installation-wide (tenant_id IS NULL) and is then visible only to
-- the instance administration.
ALTER TABLE privacy_incident ENABLE ROW LEVEL SECURITY;
ALTER TABLE privacy_incident FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON privacy_incident
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());


-- change_log: RLS on the partitioned table; the partitions carry the policy themselves.
ALTER TABLE change_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE change_log FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON change_log
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- backup_target can be instance-wide (tenant_id IS NULL) and is then visible only to
-- the instance administration - not to tenant users.
ALTER TABLE backup_target ENABLE ROW LEVEL SECURITY;
ALTER TABLE backup_target FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON backup_target
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- item_capability_profile: the system defaults (tenant_id IS NULL) are readable by everyone,
-- overrides only for the tenant concerned. Never write to the system defaults.
ALTER TABLE item_capability_profile ENABLE ROW LEVEL SECURITY;
ALTER TABLE item_capability_profile FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON item_capability_profile
  USING (tenant_id IS NULL OR tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());

-- job is partly tenant-less (system jobs) and is read only by worker roles:
-- deliberately without RLS, with access restricted through role privileges.

-- A policy on the parent is NOT inherited when a partition is addressed directly: PostgreSQL
-- applies the policies of the relation named in the query. Measured on PostgreSQL 16 - through
-- audit_log one tenant's row, through audit_log_2026_08 both. Every partition created later
-- (retention and maintenance jobs) has to carry the policy as well.
DO $partitions$
DECLARE p record;
BEGIN
  FOR p IN
    SELECT c.relname
    FROM pg_class c
    JOIN pg_inherits i ON i.inhrelid = c.oid
    JOIN pg_class parent ON parent.oid = i.inhparent
    WHERE parent.relname IN ('audit_log', 'change_log')
  LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', p.relname);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', p.relname);
    EXECUTE format($p$
      CREATE POLICY tenant_isolation ON %I
        USING (tenant_id = current_tenant_id())
        WITH CHECK (tenant_id = current_tenant_id())
    $p$, p.relname);
  END LOOP;
END $partitions$;

-- ======================= Grants for the application role ====================
-- hubtask_app works through the tables, never around them: no ownership, no BYPASSRLS, and
-- therefore subject to every policy above.
GRANT USAGE ON SCHEMA public TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO hubtask_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO hubtask_app;
GRANT EXECUTE ON FUNCTION current_tenant_id() TO hubtask_app;
GRANT EXECUTE ON FUNCTION imm_unaccent(text) TO hubtask_app;
ALTER DEFAULT PRIVILEGES FOR ROLE hubtask_migrator IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO hubtask_app;
ALTER DEFAULT PRIVILEGES FOR ROLE hubtask_migrator IN SCHEMA public
  GRANT USAGE, SELECT ON SEQUENCES TO hubtask_app;

-- ===================== Grants: immutability, level 1 ========================
-- The application role may only create and read audit entries.
-- Checked by gate SG-4 / test AT-1.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM hubtask_app;
GRANT  SELECT, INSERT ON audit_log TO hubtask_app;

-- The pseudonyms an erasure leaves for the trail are append-only for the same reason the trail is:
-- one that could be updated is a name that could come back, and one that could be deleted is an
-- erasure that could be undone (E-10, audit.md §6).
REVOKE UPDATE, DELETE, TRUNCATE ON audit_pseudonym FROM hubtask_app;
GRANT  SELECT, INSERT ON audit_pseudonym TO hubtask_app;

-- The same for the partitions: a partition addressed directly is a table of its own.
DO $audit_partitions$
DECLARE p record;
BEGIN
  FOR p IN
    SELECT c.relname
    FROM pg_class c
    JOIN pg_inherits i ON i.inhrelid = c.oid
    JOIN pg_class parent ON parent.oid = i.inhparent
    WHERE parent.relname = 'audit_log'
  LOOP
    EXECUTE format('REVOKE UPDATE, DELETE, TRUNCATE ON %I FROM hubtask_app', p.relname);
  END LOOP;
END $audit_partitions$;

-- ============== The partition duty (audit.md §3, E-09) ======================
-- Every partition created later has to carry the policy and the revokes above, because neither is
-- inherited when a partition is addressed directly - a measured finding, recorded where the
-- partitions are created. This function is what a scheduled duty calls; it creates the month's
-- partition when it is missing and repairs one that is missing either.
--
-- SECURITY DEFINER because creating a partition and revoking on it are the owner's rights. Narrow
-- for two reasons: `search_path` is pinned, and the only parameter is a date from which every
-- identifier is derived. See db/migrations/0043_audit_partition_duty.sql.
CREATE OR REPLACE FUNCTION ensure_audit_partition(month date) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  starts date := date_trunc('month', month)::date;
  ends   date := (date_trunc('month', month) + interval '1 month')::date;
  name   text := 'audit_log_' || to_char(date_trunc('month', month), 'YYYY_MM');
  target regclass;
BEGIN
  target := to_regclass(format('public.%I', name));

  IF target IS NULL THEN
    BEGIN
      EXECUTE format(
        'CREATE TABLE %I PARTITION OF audit_log FOR VALUES FROM (%L) TO (%L)', name, starts, ends);
    EXCEPTION WHEN check_violation OR invalid_table_definition THEN
      -- The month's entries are already in the default partition, which PostgreSQL will not split.
      RETURN NULL;
    END;
    target := to_regclass(format('public.%I', name));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE oid = target AND relrowsecurity) THEN
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', name);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_class WHERE oid = target AND relforcerowsecurity) THEN
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', name);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_policy WHERE polrelid = target AND polname = 'tenant_isolation'
  ) THEN
    EXECUTE format($policy$
      CREATE POLICY tenant_isolation ON %I
        USING (tenant_id = current_tenant_id())
        WITH CHECK (tenant_id = current_tenant_id())
    $policy$, name);
  END IF;

  IF has_table_privilege('hubtask_app', target, 'UPDATE')
     OR has_table_privilege('hubtask_app', target, 'DELETE')
     OR has_table_privilege('hubtask_app', target, 'TRUNCATE') THEN
    EXECUTE format('REVOKE UPDATE, DELETE, TRUNCATE ON %I FROM hubtask_app', name);
  END IF;
  IF NOT has_table_privilege('hubtask_app', target, 'INSERT')
     OR NOT has_table_privilege('hubtask_app', target, 'SELECT') THEN
    EXECUTE format('GRANT SELECT, INSERT ON %I TO hubtask_app', name);
  END IF;

  RETURN name;
END $$;

REVOKE ALL ON FUNCTION ensure_audit_partition(date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_audit_partition(date) TO hubtask_app;

-- This month and the next, so that a fresh installation never writes into the default partition.
SELECT ensure_audit_partition(date_trunc('month', now())::date);
SELECT ensure_audit_partition((date_trunc('month', now()) + interval '1 month')::date);

-- ============ The one cross-tenant question (data-protection.md §4, E-10) ===
-- Which workspaces a person is a member of. It answers tenant identifiers and nothing else; the
-- collection that follows opens one ordinary transaction per tenant under that tenant's own
-- context. See db/migrations/0044_privacy_requests.sql for the whole reasoning.
CREATE OR REPLACE FUNCTION subject_tenants(subject_email text) RETURNS SETOF uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT DISTINCT tenant_id
  FROM account
  WHERE subject_email IS NOT NULL
    AND lower(email) = lower(subject_email)
    AND deleted_at IS NULL
  ORDER BY 1
$$;

REVOKE ALL ON FUNCTION subject_tenants(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION subject_tenants(text) TO hubtask_app;

-- ============ Tenant resolution before a credential exists (H-01) ==========
-- Sign-in needs a tenant before it can check a password (0.6.0 decision 3). One identifier or
-- none, never a listing: a slug names its tenant, NULL answers the single-mode installation's
-- only row. See db/migrations/0063_auth_sessions.sql for the whole reasoning.
CREATE OR REPLACE FUNCTION resolve_tenant(tenant_slug text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT id FROM tenant
  WHERE deleted_at IS NULL
    AND (
      (tenant_slug IS NOT NULL AND slug = lower(tenant_slug))
      OR (tenant_slug IS NULL
          AND (SELECT count(*) FROM tenant WHERE deleted_at IS NULL) = 1)
    )
  LIMIT 1
$$;

REVOKE ALL ON FUNCTION resolve_tenant(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_tenant(text) TO hubtask_app;

COMMIT;
