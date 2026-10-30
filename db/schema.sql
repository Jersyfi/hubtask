-- Hubtask - the reference schema.
-- The migrations are the source; this file is the readable reference of the same state.
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
CREATE EXTENSION IF NOT EXISTS vector;   -- optional, semantic search (ADR-0050): installed by
                                         -- migration 0075 only where it is available

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

-- The text search configuration one item's language is indexed and searched under (ADR-0034).
-- STABLE rather than IMMUTABLE - which is what a trigger permits and a generated column
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
CREATE OR REPLACE FUNCTION hubtask_search_recipe(language text) RETURNS text
  LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT CASE WHEN cfg = 'simple'::regconfig THEN 'simple' ELSE cfg::text || '+simple' END
    FROM hubtask_text_config(language) AS cfg
$$;

-- The document: the title weighted A and the notes B under the entry's own configuration, and the
-- word forms beside them under `simple` at the same weights (ADR-0066). Without the second copy a
-- reader whose configuration differs from the entry's is answered "nothing matches" about an entry
-- that is plainly there, which is what the language picker used to work around.
CREATE OR REPLACE FUNCTION hubtask_search_document(language text, title text, notes text)
  RETURNS tsvector LANGUAGE sql STABLE PARALLEL SAFE AS
$$
  SELECT setweight(to_tsvector(cfg, coalesce(title, '')), 'A')
      || setweight(to_tsvector(cfg, coalesce(notes, '')), 'B')
      || CASE WHEN cfg = 'simple'::regconfig THEN ''::tsvector
              ELSE setweight(to_tsvector('simple', coalesce(title, '')), 'A')
                || setweight(to_tsvector('simple', coalesce(notes, '')), 'B')
         END
    FROM hubtask_text_config(language) AS cfg
$$;

-- The collation names sort under (i18n-l10n.md §5, migration 0080): the ICU root collation
-- where PostgreSQL has it, the database's own locale where it does not. The queries say
-- `COLLATE hubtask_name` either way; /meta/capabilities answers which of the two an installation
-- got. No index is built on it, so its ICU version is nobody's to track.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_collation WHERE collname = 'hubtask_name') THEN
    RETURN;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_collation WHERE collname = 'und-x-icu') THEN
    EXECUTE 'CREATE COLLATION hubtask_name FROM "und-x-icu"';
  ELSE
    EXECUTE format('CREATE COLLATION hubtask_name (provider = libc, locale = %L)',
                   (SELECT datcollate FROM pg_database WHERE datname = current_database()));
  END IF;
END $$;

CREATE OR REPLACE FUNCTION work_item_search_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  NEW.search_document := hubtask_search_document(NEW.content_language, NEW.title, NEW.notes);
  -- Which configuration built it: a row whose stored name differs from what
  -- hubtask_search_recipe() answers today is stale, and the reindex rewrites exactly those.
  NEW.search_configuration := hubtask_search_recipe(NEW.content_language);
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
  version            integer NOT NULL DEFAULT 1,
  -- When the 30-day grace after a deletion request runs out; the hard-delete job waits
  -- for this moment.
  purge_after        timestamptz,
  -- The synchronisation epoch (0087): every cursor carries the epoch it was minted under,
  -- and a restore into the workspace advances it, so that a cursor minted before is refused.
  sync_epoch         bigint NOT NULL DEFAULT 0,
  -- The plan this workspace is on (ADR-0070 §3, migration 0099), NULL for none; the reference
  -- (tenant_plan_fk) is added with the plan table below (0164).
  plan_id            uuid,
  -- A purchase platform's own reference for the workspace (multi-tenancy.md §4.3, 0165), unique
  -- among the workspaces that have one (tenant_external_reference_key, 0166).
  external_reference text CONSTRAINT tenant_external_reference_length
                     CHECK (external_reference IS NULL OR char_length(external_reference) BETWEEN 1 AND 200),
  -- An operator's opening of the password for this workspace (0118, ADR-0078 §3): when it
  -- ends, who asked and why. Honoured where it is read - past the end it is over, whatever the row
  -- still says - and cleared with the opening. Not in `settings`, so the workspace's own PATCH cannot
  -- reach it.
  password_opened_until     timestamptz,
  password_opened_requester text CHECK (length(password_opened_requester) BETWEEN 1 AND 200),
  password_opened_reason    text CHECK (length(password_opened_reason) BETWEEN 1 AND 500),
  -- A pending deletion can be called off (0130, multi-tenancy.md §5): where the workspace stood
  -- when it was requested, to return it there, and the grace job's mark, written before the first
  -- byte goes, after which no cancel lands.
  status_before_deletion    tenant_status,
  purge_started_at          timestamptz,
  -- The first start's question was answered or skipped (0197, UC-WRK-30 check 5): nobody is asked
  -- again.
  starter_answered_at       timestamptz,
  CONSTRAINT tenant_password_opening_whole_check
    CHECK (num_nulls(password_opened_until, password_opened_requester, password_opened_reason) IN (0, 3))
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
  -- The account's moments: NULL is "the default, on" for the first and "the tour has not
  -- been taken" for the second (migration 0093).
  celebrations      boolean,
  onboarding_completed_at timestamptz,
  -- The redemption token the invitation mints: hashed under its own purpose label, shown
  -- once, dead on redemption. One open invitation per invited account, so it lives on the row.
  redemption_token_hash bytea,
  redemption_expires_at timestamptz,
  -- When the password in password_hash was set (ADR-0068 §3, migration 0098). What max_age_days
  -- and rotation_from are compared against. NULL for an account that has no password.
  password_set_at   timestamptz,
  -- What a managed account signs in with instead of an address (ADR-0074, migration 0127): folded,
  -- unique in the workspace, never containing `@`.
  sign_in_name      text CONSTRAINT account_sign_in_name_check
                    CHECK (char_length(sign_in_name) BETWEEN 3 AND 32 AND position('@' IN sign_in_name) = 0),
  -- When Hubtask drew the password the account holds; NULL once the person chose their own.
  start_password_at timestamptz,
  -- The latest agreement to the terms of use: the link and the version in force then, and when
  -- (UC-ID-19, migration 0140). Current while both equal the terms in force.
  terms_url         text CONSTRAINT account_terms_url_check CHECK (char_length(terms_url) <= 2000),
  terms_version     text CONSTRAINT account_terms_version_check CHECK (char_length(terms_version) <= 64),
  terms_agreed_at   timestamptz,
  -- The picture the person shows beside their name (migration 0199); NULL means the initials. Its
  -- foreign key is declared after media_object.
  avatar_media_id   uuid,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz,
  version           integer NOT NULL DEFAULT 1
);
CREATE UNIQUE INDEX account_tenant_id_uq ON account (tenant_id, id);
CREATE UNIQUE INDEX account_email_uq ON account (tenant_id, lower(email))
  WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX account_sign_in_name_uq ON account (tenant_id, sign_in_name)
  WHERE sign_in_name IS NOT NULL AND deleted_at IS NULL;
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
  -- The target of a notification's key (migration 0144).
  CONSTRAINT membership_tenant_id_id_key UNIQUE (tenant_id, id),
  CONSTRAINT membership_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT membership_group_id_fkey
    FOREIGN KEY (tenant_id, group_id) REFERENCES account_group (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX membership_lookup_idx ON membership (tenant_id, account_id, scope_type, scope_id);
CREATE INDEX membership_scope_idx  ON membership (tenant_id, scope_type, scope_id);
-- A subject holds a role at a scope once (0149). NULLS NOT DISTINCT: one of account_id and group_id
-- is NULL in every row, and scope_id at the tenant.
CREATE UNIQUE INDEX membership_grant_uq
  ON membership (tenant_id, account_id, group_id, scope_type, scope_id, role) NULLS NOT DISTINCT;

-- Custom roles from the permission catalogue and where they are held (migration 0219, UC-ID-26). The
-- seven built-in roles are the enum above and have no row.
CREATE TABLE custom_role (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{1,62}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  permissions text[] NOT NULL CHECK (cardinality(permissions) BETWEEN 1 AND 256),
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT custom_role_key_uq UNIQUE (tenant_id, key)
);
CREATE TABLE custom_role_grant (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id  uuid,
  group_id    uuid,
  scope_type  membership_scope NOT NULL CHECK (scope_type <> 'ITEM'),
  scope_id    uuid,
  role_id     uuid NOT NULL,
  created_at  timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  CHECK ((account_id IS NULL) <> (group_id IS NULL)),
  CHECK ((scope_type = 'TENANT') = (scope_id IS NULL)),
  CONSTRAINT custom_role_grant_account_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT custom_role_grant_group_fkey
    FOREIGN KEY (tenant_id, group_id) REFERENCES account_group (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT custom_role_grant_role_fkey
    FOREIGN KEY (tenant_id, role_id) REFERENCES custom_role (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX custom_role_grant_uq
  ON custom_role_grant (tenant_id, account_id, group_id, scope_type, scope_id, role_id) NULLS NOT DISTINCT;
CREATE INDEX custom_role_grant_lookup_idx ON custom_role_grant (tenant_id, account_id, scope_type, scope_id);
CREATE INDEX custom_role_grant_group_idx ON custom_role_grant (tenant_id, group_id) WHERE group_id IS NOT NULL;
CREATE INDEX custom_role_grant_role_idx ON custom_role_grant (tenant_id, role_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON custom_role TO hubtask_app;
GRANT SELECT, INSERT, DELETE ON custom_role_grant TO hubtask_app;

-- The configuration document's versions, agents' plans waiting for a person, and templates
-- (migration 0221, F-6, D-16, D-17). Configuration and counts only, never content (rule 10).
CREATE TABLE config_revision (
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  version        integer NOT NULL CHECK (version > 0),
  document       jsonb NOT NULL,
  hash           text NOT NULL,
  plan_hash      text NOT NULL,
  actor_kind     text NOT NULL,
  actor_id       uuid,
  proposed_by    uuid,
  rolled_back_to integer,
  template       text,
  added          integer NOT NULL DEFAULT 0,
  changed        integer NOT NULL DEFAULT 0,
  removed        integer NOT NULL DEFAULT 0,
  created_at     timestamptz NOT NULL,
  -- The hub whose document this version is, NULL for the workspace's (migration 0223).
  hub_id         uuid,
  PRIMARY KEY (tenant_id, version)
);
CREATE TABLE config_proposal (
  id                  uuid PRIMARY KEY,
  tenant_id           uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  state               text NOT NULL CHECK (state IN ('WAITING', 'APPLIED', 'DECLINED')),
  document            jsonb NOT NULL,
  mode                text NOT NULL CHECK (mode IN ('MERGE', 'REPLACE')),
  base_hash           text NOT NULL,
  plan_hash           text NOT NULL,
  template            text,
  added               integer NOT NULL DEFAULT 0,
  changed             integer NOT NULL DEFAULT 0,
  removed             integer NOT NULL DEFAULT 0,
  hides_data          boolean NOT NULL DEFAULT false,
  changes_permissions boolean NOT NULL DEFAULT false,
  proposed_by         uuid,
  decided_by          uuid,
  created_at          timestamptz NOT NULL,
  decided_at          timestamptz,
  hub_id              uuid,
  UNIQUE (tenant_id, id)
);
CREATE INDEX config_proposal_waiting_idx ON config_proposal (tenant_id, created_at DESC) WHERE state = 'WAITING';
CREATE TABLE config_template (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$'),
  version    integer NOT NULL CHECK (version > 0),
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  origin     text NOT NULL CHECK (origin IN ('WORKSPACE', 'OWN')),
  document   jsonb NOT NULL,
  created_by uuid,
  created_at timestamptz NOT NULL,
  UNIQUE (tenant_id, id),
  CONSTRAINT config_template_version_uq UNIQUE (tenant_id, key, version)
);
CREATE TABLE config_template_use (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  template_key text NOT NULL,
  version      integer NOT NULL,
  document     jsonb NOT NULL,
  applied_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, template_key)
);
-- DELETE only for a replacing restore (migration 0222, tenant-export.md §6).
GRANT SELECT, INSERT, DELETE ON config_revision TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON config_proposal TO hubtask_app;
GRANT SELECT, INSERT, DELETE ON config_template TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON config_template_use TO hubtask_app;
-- The sandbox: a copy of the configuration without content, and the one it was copied from
-- (migration 0224, config-document.md §3).
CREATE TABLE config_sandbox (
  tenant_id     uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  document      jsonb NOT NULL,
  base_document jsonb NOT NULL,
  base_hash     text NOT NULL,
  base_version  integer NOT NULL DEFAULT 0,
  writes        integer NOT NULL DEFAULT 0,
  updated_at    timestamptz NOT NULL
);
GRANT SELECT, INSERT, UPDATE, DELETE ON config_sandbox TO hubtask_app;

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

-- A sign-in (security.md §5): the row /auth/sessions lists and revocation stamps. Both
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
  -- The step-up: a fresh re-authentication recorded on the session, one live proof at a
  -- time, consumed by the one privileged action it is presented to.
  step_up_token_hash  bytea,
  step_up_at          timestamptz,
  step_up_method      text,
  step_up_consumed_at timestamptz,
  -- The leash: a session issued through an OAuth exchange names its grant and carries
  -- the grant's scopes. NULL scopes is a person's own session. The foreign key is added after
  -- oauth_grant below, the order the migrations built it in.
  grant_id uuid,
  scopes   text[],
  -- The two bounds this session answers to, written when it opened (migration 0100): the moment it
  -- may not outlive however often it is refreshed, and how long it may sit unused. NULL is "no
  -- bound". Written at sign-in rather than resolved per request, because resolving would put a round
  -- trip on the hot path of the whole API for two numbers that change once a year - and the cost of
  -- that trade is that a bound tightened later reaches new sessions only, which is what
  -- `rotation_from` exists to fix in one write.
  hard_expires_at timestamptz,
  idle_minutes    integer,
  -- The elevated session (ADR-0070 §4, migration 0102): until when this session carries the control
  -- plane's scope. On the session rather than in a table of its own, which is the whole design - the
  -- elevation ends with the session because it is a column of it. NULL is "not elevated", which is
  -- what every row means again an hour later.
  elevated_until  timestamptz,
  -- How this session was opened (ADR-0068 §3, migration 0098): PASSWORD, PASSWORD_TOTP,
  -- PASSWORD_RECOVERY, PASSWORD_PASSKEY, OIDC, INVITATION, RESET, PASSKEY. Answered in the
  -- session list, so that a person reading their own sessions can tell them apart. NULL is a
  -- session opened before the column existed.
  signed_in_with text,
  -- Which provider opened it, for a session opened through one (migration 0111): named in the
  -- session list. No foreign key - a removed provider is no reason to touch the sessions it opened.
  signed_in_provider_id uuid,
  CONSTRAINT session_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX session_grant_idx ON session (grant_id) WHERE grant_id IS NOT NULL;
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

-- One TOTP enrolment per account (security.md §5): the secret sealed through the envelope
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
  -- The authenticator's replacement (migration 0113): a second, unconfirmed secret beside
  -- the armed one, the session that began it and the end of its window. All four or none.
  replacement_secret_enc    bytea,
  replacement_secret_key_id text,
  replacement_session_id    uuid,
  replacement_expires_at    timestamptz,
  CONSTRAINT account_mfa_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT account_mfa_replacement_whole CHECK (
    (replacement_secret_enc IS NULL) = (replacement_secret_key_id IS NULL)
    AND (replacement_secret_enc IS NULL) = (replacement_session_id IS NULL)
    AND (replacement_secret_enc IS NULL) = (replacement_expires_at IS NULL)
  )
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

-- The passwords this account has had before this one (ADR-0068 §1, migration 0098). Hashes only:
-- no plaintext, and nothing that says anything about the shape of one. The cap is the policy's
-- `history_count` rather than a constraint, because the cap is a value an installation changes and
-- a constraint would have to move with it.
CREATE TABLE account_password_history (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id    uuid NOT NULL,
  password_hash text NOT NULL,
  set_at        timestamptz NOT NULL,
  CONSTRAINT account_password_history_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX account_password_history_account_idx
  ON account_password_history (tenant_id, account_id, set_at DESC);

-- Passkeys and security keys: an account's WebAuthn credentials, up to ten (identity.md §23).
-- The name is user content (rule 10); the public key is the COSE bytes the authenticator sent.
CREATE TABLE webauthn_credential (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id      uuid NOT NULL,
  credential_id   bytea NOT NULL CHECK (octet_length(credential_id) BETWEEN 1 AND 1023),
  public_key      bytea NOT NULL CHECK (octet_length(public_key) BETWEEN 1 AND 2048),
  algorithm       integer NOT NULL CHECK (algorithm IN (-7, -8, -257)),
  sign_count      bigint NOT NULL DEFAULT 0 CHECK (sign_count BETWEEN 0 AND 4294967295),
  user_handle     bytea NOT NULL CHECK (octet_length(user_handle) = 32),
  backup_eligible boolean NOT NULL,
  backed_up       boolean NOT NULL,
  transports      text[] NOT NULL DEFAULT '{}' CHECK (cardinality(transports) <= 6),
  name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
  created_at      timestamptz NOT NULL,
  last_used_at    timestamptz,
  -- The host it was made for; NULL for one made before the column (migration 0163).
  rp_id           text CHECK (rp_id IS NULL OR char_length(rp_id) BETWEEN 1 AND 253),
  CONSTRAINT webauthn_credential_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT webauthn_credential_backup_state CHECK (backup_eligible OR NOT backed_up)
);
CREATE UNIQUE INDEX webauthn_credential_credential_uq ON webauthn_credential (tenant_id, credential_id);
CREATE INDEX webauthn_credential_account_idx ON webauthn_credential (tenant_id, account_id, created_at);

-- The pending credential of a two-step sign-in: short-lived, single-use, hashed under
-- its own purpose label - a row with the session machinery's discipline, not a session.
CREATE TABLE auth_pending (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- NULL only on a TERMS row carrying a provider's arrival, which has no account yet (migration 0142).
  account_id  uuid,
  -- NULL only on an ADDRESS row whose mail has not gone out yet: the token is minted at delivery
  -- (migration 0131), and a NULL never matches a presented token.
  token_hash  bytea,
  -- What the credential may complete. TOTP presents a code, ENROLL is the second factor's
  -- enforcement route, RESET is the token a reset mail carries, PASSWORD is the credential
  -- the PASSWORD_CHANGE step hands out (migration 0098), LINK is a provider arrival waiting
  -- for the account's own proof (migration 0110), CONNECT is the link a workspace without
  -- the password mails to connect its provider (migration 0117), ADDRESS is the link that
  -- confirms a managed account's new address (migration 0131), and TERMS is the step a
  -- sign-in ends in while the account has not agreed to the terms of use in force (migration 0140),
  -- PASSKEY_REGISTER is a passkey registration's challenge (migration 0161), PASSKEY_SIGN_IN a
  -- passkey sign-in's (migration 0168), and PASSKEY_FACTOR, PASSKEY_ENROLL and PASSKEY_STEP_UP the
  -- challenges of a key as the second factor, its setup during sign-in and a fresh proof
  -- (migration 0176).
  purpose     text NOT NULL,
  user_agent  text,
  ip_class    text,
  created_at  timestamptz NOT NULL,
  expires_at  timestamptz NOT NULL,
  consumed_at timestamptz,
  -- The provider identity this sign-in connects once it completes (migration 0110, E2).
  -- Its foreign key is declared after identity_provider, which this file defines further down.
  link_provider_id uuid,
  link_subject     text CHECK (length(link_subject) BETWEEN 1 AND 255),
  -- What proved the account when the link is connected: the password at the LINK step, or the
  -- mailbox with a fresh sign-in at the provider (migration 0117). NULL reads as the password.
  link_proof       text CHECK (link_proof IN ('PASSWORD', 'MAILBOX')),
  -- A password change still owed once this step completes: a password the rule refused, or a
  -- start password, met after a factor step (migration 0127).
  password_change_owed boolean NOT NULL DEFAULT false,
  -- The address an ADDRESS row confirms; it becomes the account's only when the link is followed
  -- (migration 0131).
  address     text CONSTRAINT auth_pending_address_length CHECK (char_length(address) BETWEEN 3 AND 254),
  -- The session a TERMS step opens once agreed: how the sign-in proved the account, and the
  -- provider that did (migration 0140).
  session_method text CONSTRAINT auth_pending_session_method_check CHECK (char_length(session_method) <= 32),
  session_via    uuid,
  -- How the steps before a change step were proven, for the session it opens (migration 0139).
  signed_in_with text,
  -- A provider's newcomer waiting for the terms of use on a row without an account (migration 0142).
  arrival_email        text CONSTRAINT auth_pending_arrival_email_length
    CHECK (char_length(arrival_email) BETWEEN 3 AND 254),
  arrival_display_name text CONSTRAINT auth_pending_arrival_display_name_length
    CHECK (char_length(arrival_display_name) <= 200),
  arrival_directory    text CONSTRAINT auth_pending_arrival_directory_length
    CHECK (char_length(arrival_directory) <= 255),
  -- A passkey registration's session, which alone may finish it, and the user handle the new
  -- credential will carry (migration 0161).
  session_id  uuid,
  user_handle bytea,
  CONSTRAINT auth_pending_session_fkey FOREIGN KEY (tenant_id, session_id)
    REFERENCES session (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT auth_pending_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT auth_pending_purpose_check
    CHECK (purpose IN ('TOTP', 'ENROLL', 'RESET', 'PASSWORD', 'LINK', 'CONNECT', 'ADDRESS', 'TERMS',
                       'PASSKEY_REGISTER', 'PASSKEY_SIGN_IN', 'PASSKEY_FACTOR', 'PASSKEY_ENROLL',
                       'PASSKEY_STEP_UP')),
  CONSTRAINT auth_pending_address_purpose
    CHECK ((purpose = 'ADDRESS') = (address IS NOT NULL)
           AND (purpose = 'ADDRESS' OR token_hash IS NOT NULL)),
  CONSTRAINT auth_pending_link_whole CHECK ((link_provider_id IS NULL) = (link_subject IS NULL)),
  CONSTRAINT auth_pending_link_purpose CHECK (purpose <> 'LINK' OR link_provider_id IS NOT NULL),
  -- No account exactly where a TERMS arrival or a passkey sign-in's challenge stands (migration 0174).
  CONSTRAINT auth_pending_arrival_whole
    CHECK (CASE WHEN purpose = 'PASSKEY_SIGN_IN'
                THEN account_id IS NULL AND arrival_email IS NULL
                ELSE (account_id IS NULL) = (arrival_email IS NOT NULL) END),
  CONSTRAINT auth_pending_arrival_terms
    CHECK (account_id IS NOT NULL OR purpose = 'PASSKEY_SIGN_IN'
           OR (purpose = 'TERMS' AND link_provider_id IS NOT NULL)),
  CONSTRAINT auth_pending_passkey_whole
    CHECK ((session_id IS NOT NULL) = (purpose IN ('PASSKEY_REGISTER', 'PASSKEY_STEP_UP'))
           AND (user_handle IS NOT NULL) = (purpose IN ('PASSKEY_REGISTER', 'PASSKEY_ENROLL'))
           AND (user_handle IS NULL OR octet_length(user_handle) = 32))
);
CREATE UNIQUE INDEX auth_pending_token_uq ON auth_pending (token_hash);
-- What a sign-in flow's key to its CONNECT link points at (migration 0117).
CREATE UNIQUE INDEX auth_pending_tenant_id_uq ON auth_pending (tenant_id, id);

-- A registered third-party app. Redirect URIs match exactly, byte for byte.
CREATE TABLE oauth_client (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
  confidential  boolean NOT NULL,
  secret_hash   bytea,
  redirect_uris text[] NOT NULL,
  created_at    timestamptz NOT NULL,
  created_by    uuid,
  version       integer NOT NULL DEFAULT 1,
  CHECK (confidential = (secret_hash IS NOT NULL))
);
CREATE UNIQUE INDEX oauth_client_tenant_id_uq ON oauth_client (tenant_id, id);

-- What a person allowed one app: the row they see and revoke beside their sessions.
CREATE TABLE oauth_grant (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  client_id  uuid NOT NULL,
  scopes     text[] NOT NULL,
  created_at timestamptz NOT NULL,
  revoked_at timestamptz,
  CONSTRAINT oauth_grant_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT oauth_grant_client_fkey FOREIGN KEY (tenant_id, client_id)
    REFERENCES oauth_client (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX oauth_grant_tenant_id_uq ON oauth_grant (tenant_id, id);
CREATE UNIQUE INDEX oauth_grant_live_uq ON oauth_grant (account_id, client_id)
  WHERE revoked_at IS NULL;
CREATE INDEX oauth_grant_account_idx ON oauth_grant (account_id, created_at DESC);

-- A single-use authorization code, kept briefly after consumption as the replay signal.
CREATE TABLE oauth_code (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  client_id      uuid NOT NULL,
  account_id     uuid NOT NULL,
  grant_id       uuid NOT NULL,
  code_hash      bytea NOT NULL,
  code_challenge text NOT NULL,
  redirect_uri   text NOT NULL,
  created_at     timestamptz NOT NULL,
  expires_at     timestamptz NOT NULL,
  consumed_at    timestamptz,
  CONSTRAINT oauth_code_grant_fkey FOREIGN KEY (tenant_id, grant_id)
    REFERENCES oauth_grant (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX oauth_code_hash_uq ON oauth_code (code_hash);

-- The hosts a workspace answers at (migration 0104). In multi mode a request's host names its
-- workspace through this table (`resolve_tenant_host`, migration 0207), and the canonical row is the
-- workspace's main address. The subdomain row is derived from the slug under the installation's own
-- domain and is verified by construction; the verification mark of any other row is published in a DNS
-- record, which is why it is the one presented value here that is not a digest.
CREATE TABLE tenant_host (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  host         text NOT NULL CHECK (host = lower(host) AND length(host) BETWEEN 4 AND 253),
  -- PENDING claimed · VERIFIED the zone carried the mark · ACTIVE serving, and the canonical one
  -- is ACTIVE · BROKEN it was ACTIVE and stopped, which is why the row stays: it is the way back.
  state        text NOT NULL DEFAULT 'PENDING'
                 CHECK (state IN ('PENDING', 'VERIFIED', 'ACTIVE', 'BROKEN')),
  verification text NOT NULL CHECK (length(verification) BETWEEN 8 AND 200),
  verified_at  timestamptz,
  is_canonical boolean NOT NULL DEFAULT false,
  created_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, host)
);
-- One host, one workspace, installation-wide: two workspaces claiming the same host is the failure
-- this table exists to make impossible.
CREATE UNIQUE INDEX tenant_host_host_uq ON tenant_host (host);
-- One canonical host per workspace. A mail, a redirect and an invitation link have to name one.
CREATE UNIQUE INDEX tenant_host_canonical_uq ON tenant_host (tenant_id) WHERE is_canonical;

-- The providers a workspace signs its people in through (migration 0103). Plural,
-- and `tenant_id` is nullable: NULL is the installation's own, which every workspace reads and
-- none writes - see the two policies below. The client secret is sealed under the envelope
-- encryption of security.md §8 - a
-- token exchange needs the plaintext, which is why this is not a hash.
CREATE TABLE identity_provider (
  id                    uuid PRIMARY KEY,
  tenant_id             uuid REFERENCES tenant(id) ON DELETE CASCADE,
  issuer                text NOT NULL CHECK (length(issuer) BETWEEN 1 AND 500),
  client_id             text NOT NULL CHECK (length(client_id) BETWEEN 1 AND 500),
  client_secret_enc     bytea NOT NULL,
  client_secret_key_id  text NOT NULL,
  display_name          text NOT NULL DEFAULT '' CHECK (length(display_name) <= 200),
  -- The preset it was configured from, which decides the mark that is drawn (ADR-0069). GENERIC
  -- is the letter tile.
  kind                  text NOT NULL DEFAULT 'GENERIC'
                          CHECK (kind IN ('GENERIC', 'GOOGLE', 'MICROSOFT')),
  -- Who gets an account on a first arrival. A public provider may only be INVITED_ONLY, which the
  -- application enforces: "public" is a property of the preset, not of anything a CHECK can see.
  provisioning          text NOT NULL DEFAULT 'DOMAINS'
                          CHECK (provisioning IN ('INVITED_ONLY', 'DOMAINS', 'ANY')),
  position              integer NOT NULL DEFAULT 0,
  allowed_email_domains text[] NOT NULL DEFAULT '{}',
  allowed_directories text[] NOT NULL DEFAULT '{}',
  enabled               boolean NOT NULL DEFAULT true,
  created_at            timestamptz NOT NULL,
  updated_at            timestamptz,
  version               integer NOT NULL DEFAULT 1,
  -- An installation row's count of workspaces that switched it on, moved only through
  -- move_provider_offer, and when its offer ends (ADR-0076, migration 0114). A workspace's own row
  -- keeps zero and NULL.
  offered_workspaces    integer NOT NULL DEFAULT 0,
  withdraw_at           timestamptz,
  -- What somebody the provider brings in new gets (migration 0132): one role on the workspace, or
  -- one group of it, or nothing (both NULL). Never on the installation's rows, never OWNER.
  newcomer_role         membership_role,
  newcomer_group_id     uuid,
  CONSTRAINT identity_provider_newcomers_check CHECK (
    (newcomer_role IS NULL OR newcomer_group_id IS NULL)
    AND (tenant_id IS NOT NULL OR (newcomer_role IS NULL AND newcomer_group_id IS NULL))
    AND newcomer_role IS DISTINCT FROM 'OWNER'
  ),
  CONSTRAINT identity_provider_newcomer_group_fk
    FOREIGN KEY (tenant_id, newcomer_group_id) REFERENCES account_group (tenant_id, id)
    ON DELETE SET NULL (newcomer_group_id)
);
-- One registration per issuer per level. NULLS NOT DISTINCT is what makes that true of the
-- installation's rows: without it every NULL tenant is its own.
CREATE UNIQUE INDEX identity_provider_issuer_uq
  ON identity_provider (tenant_id, issuer) NULLS NOT DISTINCT;
CREATE INDEX identity_provider_tenant_idx
  ON identity_provider (tenant_id, position, created_at);

-- Which provider vouched for a subject, and which account it became (migration 0103).
-- `account.external_subject` held one subject per account and could not say which provider; this
-- can. One account holds at most one subject per provider, and one subject names at most one
-- account per provider per workspace - the workspace is in that key because an installation-wide
-- provider is one row for everybody and a person may work in two of them.
CREATE TABLE account_identity (
  tenant_id   uuid NOT NULL,
  account_id  uuid NOT NULL,
  provider_id uuid NOT NULL REFERENCES identity_provider(id) ON DELETE CASCADE,
  subject     text NOT NULL CHECK (length(subject) BETWEEN 1 AND 255),
  linked_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id, provider_id),
  CONSTRAINT account_identity_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX account_identity_subject_uq
  ON account_identity (tenant_id, provider_id, subject);

-- auth_pending's link to a provider (migration 0110), declared here because identity_provider is.
ALTER TABLE auth_pending ADD CONSTRAINT auth_pending_link_provider_id_fkey
  FOREIGN KEY (link_provider_id) REFERENCES identity_provider(id) ON DELETE CASCADE;

-- One browser round trip of authorization code + PKCE. The state is hashed because the caller
-- presents it back; the verifier and the nonce are kept as they are because one travels to the
-- provider and the other is compared with a claim - and neither completes a flow without the
-- sealed client secret.
-- The workspace's AI provider (ADR-0012, ADR-0049): which provider it would ask, under
-- which models, in which jurisdiction, and whether it consents to being asked at all. One row per
-- workspace. The API key is sealed under the envelope encryption (security.md §8), never hashed - the adapter needs the
-- plaintext to sign a request - and the two envelope columns are all-or-nothing.
CREATE TABLE ai_provider (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  -- NULL is a model the installation offers to every workspace (ADR-0072, migration 0129).
  tenant_id        uuid REFERENCES tenant(id) ON DELETE CASCADE,
  kind             text NOT NULL CHECK (kind IN ('NOOP', 'OPENAI_COMPATIBLE', 'OLLAMA')),
  base_url         text NOT NULL DEFAULT '' CHECK (length(base_url) <= 2000),
  completion_model text NOT NULL DEFAULT '' CHECK (length(completion_model) <= 200),
  embedding_model  text NOT NULL DEFAULT '' CHECK (length(embedding_model) <= 200),
  api_key_enc      bytea,
  api_key_key_id   text,
  -- A declaration rather than something this software can verify; its purpose is that the
  -- decision is documented rather than made by accident (ADR-0018 decision 7).
  jurisdiction     text NOT NULL
    CHECK (jurisdiction IN ('SELF_HOSTED', 'EEA', 'ADEQUACY', 'THIRD_COUNTRY')),
  -- ai-first.md §2's switch, checked before every call. Consent is given, never inherited.
  processing_allowed boolean NOT NULL DEFAULT false,
  created_at       timestamptz NOT NULL,
  updated_at       timestamptz,
  version          integer NOT NULL DEFAULT 1,
  -- What a workspace is shown of an offered model, as the operator declares it.
  display_name     text NOT NULL DEFAULT '' CHECK (length(display_name) <= 200),
  processor        text NOT NULL DEFAULT '' CHECK (length(processor) <= 200),
  CONSTRAINT ai_provider_key_envelope CHECK (
    (api_key_enc IS NULL AND api_key_key_id IS NULL) OR
    (api_key_enc IS NOT NULL AND api_key_key_id IS NOT NULL)
  ),
  -- An offered row is named, says who processes, calls something, and never carries consent.
  CONSTRAINT ai_provider_offered_shape CHECK (
    tenant_id IS NOT NULL OR (
      display_name <> '' AND processor <> '' AND kind <> 'NOOP' AND NOT processing_allowed
    )
  )
);
-- One own row per workspace; NULLs are distinct, so any number of offered rows.
CREATE UNIQUE INDEX ai_provider_tenant_uq ON ai_provider (tenant_id);
CREATE UNIQUE INDEX ai_provider_offered_name_uq ON ai_provider (display_name) WHERE tenant_id IS NULL;

-- The workspace's choice of AI source (ADR-0072 §2, §3). Choosing an offered model is the
-- workspace's consent to it; a withdrawn model leaves the choice pointing at nothing.
CREATE TABLE ai_source (
  tenant_id           uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  source              text NOT NULL CHECK (source IN ('OWN', 'OFFERED', 'NONE')),
  offered_provider_id uuid REFERENCES ai_provider(id) ON DELETE SET NULL,
  updated_at          timestamptz NOT NULL,
  version             integer NOT NULL DEFAULT 1,
  CONSTRAINT ai_source_offered_only CHECK (source = 'OFFERED' OR offered_provider_id IS NULL)
);

CREATE TABLE oidc_flow (
  id            uuid PRIMARY KEY,
  -- Which provider this sign-in left through (migration 0103). The state names the
  -- workspace and a workspace has several ways in, so the flow remembers the one it used - or the
  -- exchange would be signed with the wrong client secret. Nullable for the rolling window: a flow
  -- opened by the previous binary carries none.
  provider_id   uuid,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  state_hash    bytea NOT NULL,
  code_verifier text NOT NULL,
  nonce         text NOT NULL,
  created_at    timestamptz NOT NULL,
  expires_at    timestamptz NOT NULL,
  consumed_at   timestamptz,
  -- The session a step-up at the provider belongs to (ADR-0075 §2, migration 0112). NULL is a
  -- sign-in flow. Each callback consumes only its own kind.
  session_id    uuid,
  -- The invited account the flow accepts, when it started from the invitation's own link
  -- (ADR-0078 §1, migration 0116). NULL is every other sign-in.
  invited_account_id uuid,
  -- The CONNECT link a sign-in started from, in a workspace without the password (ADR-0078 §1,
  -- migration 0117). NULL is every other sign-in.
  pending_id    uuid,
  -- A flow bound to a session that connects a provider to the account rather than stepping up
  -- (ADR-0078 §1, migration 0152). Each callback consumes only its own kind.
  connect       boolean NOT NULL DEFAULT false,
  -- The terms of use agreed to on the invitation card before the provider was chosen (UC-ID-19,
  -- migration 0140); the arrival that activates the invited account stores them.
  terms_url     text CONSTRAINT oidc_flow_terms_url_check CHECK (char_length(terms_url) <= 2000),
  terms_version text CONSTRAINT oidc_flow_terms_version_check CHECK (char_length(terms_version) <= 64),
  CONSTRAINT oidc_flow_session_fkey FOREIGN KEY (tenant_id, session_id)
    REFERENCES session (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT oidc_flow_invited_account_fkey FOREIGN KEY (tenant_id, invited_account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT oidc_flow_pending_fkey FOREIGN KEY (tenant_id, pending_id)
    REFERENCES auth_pending (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX oidc_flow_state_uq ON oidc_flow (state_hash);
CREATE INDEX oidc_flow_expiry_idx ON oidc_flow (expires_at);

-- The leash's key, addable only once oauth_grant exists.
ALTER TABLE session
  ADD CONSTRAINT session_grant_fkey FOREIGN KEY (tenant_id, grant_id)
    REFERENCES oauth_grant (tenant_id, id) ON DELETE CASCADE;

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
  -- policies keys (domain-model.md §3.3): completion_policy (MANUAL | ROLLUP), structure (the
  -- container's own layer of its settings, domain-model.md §2), auto_assign. An absent key means the
  -- default - for completion_policy and structure, what the layer above says; the column starts as {}
  -- and UpdateContainerPolicies and SetContainerStructure write into it, each its own keys.
  --
  -- auto_assign is a key of the policies *document*, not of this column: it is stored in
  -- auto_assign_policy, because ROUND_ROBIN's cursor has to be lockable, and the container queries
  -- join it back so every read still carries the whole document.
  --
  -- default_bucket_id has no writer and is not read. A deleted bucket's items need a bucket to fall
  -- back to, and it is derived instead - the collection's leftmost remaining bucket -
  -- because a stored default is a value nothing keeps up to date: a column deleted while the key
  -- still named it would send items into a bucket that is no longer on the board.
  policies     jsonb NOT NULL DEFAULT '{}'::jsonb,
  archived_at  timestamptz,
  deleted_at   timestamptz,
  trash_batch_id uuid,
  -- Who deleted it, in activity_entry's shape: a kind and a nullable identifier, because an
  -- automation and the system act without an account. Permanently nullable - a row deleted before
  -- 0070 will never learn who deleted it, and inventing it from the audit trail would be reading a
  -- fact out of a source with a different permission.
  deleted_by_type text,
  deleted_by_id uuid,
  created_by   uuid NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  version      integer NOT NULL DEFAULT 1,
  -- A private hub is reached only through a membership on it or below it (ADR-0073 §1, 0124).
  private      boolean NOT NULL DEFAULT false,
  -- A collection's short name and the count its entries' numbers come from (migration 0204,
  -- ADR-0094): "einkauf", and EINKAUF-42 for its 42nd entry.
  handle       text CHECK (handle IS NULL OR handle ~ '^[a-z][a-z0-9]{1,19}$'),
  item_counter bigint NOT NULL DEFAULT 0,
  CHECK ((type = 'HUB') = (parent_id IS NULL)),
  CONSTRAINT container_private_hub_check CHECK (NOT private OR type = 'HUB')
);
CREATE UNIQUE INDEX container_tenant_id_uq ON container (tenant_id, id);
CREATE UNIQUE INDEX container_handle_uq ON container (tenant_id, handle) WHERE handle IS NOT NULL;
ALTER TABLE container ADD CONSTRAINT container_parent_id_fkey
    FOREIGN KEY (tenant_id, parent_id) REFERENCES container (tenant_id, id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX container_name_uq
  ON container (tenant_id, coalesce(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(imm_unaccent(name)))
  WHERE deleted_at IS NULL;
CREATE INDEX container_parent_idx ON container (tenant_id, parent_id, order_key)
  WHERE deleted_at IS NULL;
-- The trash view and the way back out of it. Every other index on this table is partial on
-- `deleted_at IS NULL` and therefore describes exactly what the trash is not.
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

-- The settings catalogue's entries of the workspace, its hubs, collections and people (migration
-- 0208): a dotted key, its value as JSON, how the level holds it, and what it leaves the levels
-- below. `scope_id` is the tenant, the container or the account, by `level`; no foreign key, since it
-- points into three tables.
CREATE TABLE setting_value (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  level      text NOT NULL CHECK (level IN ('WORKSPACE', 'HUB', 'COLLECTION', 'PERSON')),
  scope_id   uuid NOT NULL,
  key        text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
  value      jsonb,
  hold       text NOT NULL DEFAULT 'SET' CHECK (hold IN ('SET', 'LOCK', 'RESTRICT')),
  allowed    jsonb,
  set_by     uuid,
  updated_at timestamptz NOT NULL,
  UNIQUE (tenant_id, level, scope_id, key),
  CHECK (value IS NOT NULL OR hold <> 'SET'),
  CHECK ((allowed IS NOT NULL) = (hold = 'RESTRICT'))
);

-- The type registry (migrations 0209, 0210; UC-FLW-01): a workspace's own kinds of entries, each on
-- one level. The three system types are declared in the domain and gain a row only once changed.
CREATE TABLE entry_type (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key              text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  level            text NOT NULL CHECK (level IN ('TASK', 'WORK_PACKAGE', 'ACTIVITY')),
  system           boolean NOT NULL DEFAULT false,
  -- Empty only on a system type nobody named: a client shows its own word for the level (rule 8).
  name             text NOT NULL CHECK (char_length(name) <= 120 AND (system OR btrim(name) <> '')),
  icon             text NOT NULL CHECK (icon ~ '^[a-z][a-z0-9-]{0,39}$'),
  color_token      text NOT NULL CHECK (color_token ~ '^[a-z][a-z0-9-]{0,39}$'),
  completable      boolean NOT NULL DEFAULT true,
  workflow         boolean NOT NULL DEFAULT false,
  record           boolean NOT NULL DEFAULT false,
  schedulable      boolean NOT NULL DEFAULT true,
  estimable        boolean NOT NULL DEFAULT false,
  allowed_children text[] NOT NULL DEFAULT '{}',
  -- The type's form (0214): one document, read and written whole.
  form             jsonb NOT NULL DEFAULT '{}'::jsonb
                     CHECK (jsonb_typeof(form) = 'object' AND pg_column_size(form) <= 262144),
  position         integer NOT NULL DEFAULT 0,
  archived_at      timestamptz,
  created_at       timestamptz NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT entry_type_key_uq UNIQUE (tenant_id, key),
  CHECK (NOT (record AND completable)),
  CHECK (NOT system OR (key = lower(level) AND NOT record AND archived_at IS NULL))
);
CREATE UNIQUE INDEX entry_type_system_uq ON entry_type (tenant_id, level) WHERE system;

-- Workflows and which hub or collection runs which per type (migration 0216, UC-FLW-02 … 05).
CREATE TABLE workflow (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  system       boolean NOT NULL DEFAULT false,
  name         text NOT NULL CHECK (char_length(name) <= 120 AND (system OR btrim(name) <> '')),
  published    jsonb NOT NULL
    CHECK (jsonb_typeof(published) = 'object' AND pg_column_size(published) <= 262144),
  draft        jsonb
    CHECK (draft IS NULL OR (jsonb_typeof(draft) = 'object' AND pg_column_size(draft) <= 262144)),
  published_at timestamptz,
  published_by uuid,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT workflow_key_uq UNIQUE (tenant_id, key),
  CHECK (NOT system OR key = 'default')
);

CREATE TABLE workflow_scheme (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  container_id uuid NOT NULL,
  entry_type   text NOT NULL DEFAULT ''
    CHECK (entry_type = '' OR entry_type ~ '^[a-z][a-z0-9_]{0,49}$'),
  workflow_key text NOT NULL,
  created_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, container_id, entry_type),
  CONSTRAINT workflow_scheme_container_fkey
    FOREIGN KEY (tenant_id, container_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT workflow_scheme_workflow_fkey
    FOREIGN KEY (tenant_id, workflow_key) REFERENCES workflow (tenant_id, key)
);

CREATE TABLE work_item (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id      uuid NOT NULL,
  type               item_type NOT NULL,
  parent_id          uuid,
  path               text NOT NULL,              -- '/<uuid>/<uuid>/…', materialised
  depth              integer NOT NULL CHECK (depth >= 0),
  title              text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 500),
  -- How much of it ("2", "1 kg"), capability AMOUNT (migration 0188); NULL for none.
  amount             text CHECK (amount IS NULL OR char_length(amount) BETWEEN 1 AND 40),
  notes              text,
  is_completed       boolean NOT NULL DEFAULT false,
  completed_at       timestamptz,
  completed_by       uuid,
  -- How often it went from open to done; kept by a reopen (migration 0189).
  completion_count   integer NOT NULL DEFAULT 0 CHECK (completion_count >= 0),
  -- When it first entered its collection's first column and last entered a done column, kept by
  -- the trigger work_item_flow_moments (migration 0194; UC-WRK-31 check 4).
  flow_started_at    timestamptz,
  flow_finished_at   timestamptz,
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
  -- key from resurrecting the old value (migration 0018).
  custom_field_refs  jsonb NOT NULL DEFAULT '{}'::jsonb,
  recurrence_rule_id uuid,
  recurrence_source_id uuid,
  origin_jumble_id   uuid,
  content_language   text,
  calendar_uid       text,
  search_vector      tsvector GENERATED ALWAYS AS
                       (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(notes, ''))) STORED,
  -- The language-dependent document the search reads, maintained by the trigger below and dropping
  -- the generated column above in a later migration (migration 0019, ADR-0034).
  search_document    tsvector,
  search_configuration text,             -- the configuration that built the document
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
  -- The same pair the container carries, and for the same reason (0070).
  deleted_by_type    text,
  deleted_by_id      uuid,
  created_by         uuid NOT NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  version            integer NOT NULL DEFAULT 1,
  CHECK (is_completed = (completed_at IS NOT NULL)),
  CHECK ((type = 'TASK') = (parent_id IS NULL)),
  CHECK (bucket_id IS NULL OR type = 'TASK'),
  -- The cover's integrity (migration 0013). No TASK tie: which types carry COVER is the
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
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  -- The entry's number in its collection (migration 0204, ADR-0094).
  number       bigint,
  -- The entry's type by key, null for its level's system type (migration 0209).
  entry_type   text,
  -- Its status in the workflow in force for it, null where none runs (migration 0216).
  status       text,
  CONSTRAINT work_item_entry_type_fkey
    FOREIGN KEY (tenant_id, entry_type) REFERENCES entry_type (tenant_id, key)
);
CREATE INDEX wi_entry_type_idx ON work_item (tenant_id, entry_type) WHERE entry_type IS NOT NULL;
CREATE INDEX wi_status_idx ON work_item (tenant_id, collection_id, status) WHERE status IS NOT NULL;
CREATE UNIQUE INDEX work_item_tenant_id_uq ON work_item (tenant_id, id);
CREATE UNIQUE INDEX wi_calendar_uid_uq ON work_item (tenant_id, calendar_uid)
  WHERE calendar_uid IS NOT NULL;
ALTER TABLE work_item ADD CONSTRAINT work_item_parent_id_fkey
    FOREIGN KEY (tenant_id, parent_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE;
CREATE INDEX wi_board_idx    ON work_item (tenant_id, collection_id, bucket_id, order_key)
  WHERE deleted_at IS NULL AND archived_at IS NULL;
CREATE INDEX wi_due_idx      ON work_item (tenant_id, collection_id, due_at)
  WHERE deleted_at IS NULL AND archived_at IS NULL AND is_completed = false;
-- A collection's figure: its open rows directly under it (OPEN, FIELD_SUM), and its completions
-- from a moment on (RHYTHM). Migration 0187.
CREATE INDEX wi_figure_open_idx ON work_item (tenant_id, collection_id)
  WHERE parent_id IS NULL AND is_completed = false AND archived_at IS NULL AND deleted_at IS NULL;
CREATE INDEX wi_figure_done_idx ON work_item (tenant_id, collection_id, completed_at)
  WHERE is_completed = true AND deleted_at IS NULL;
-- The occurrences of one series: what an ON_COMPLETION series is waiting for, and when it was
-- last done.
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
-- because the cursor is a keyset over (order_key, id) (api-guidelines.md §4).
CREATE INDEX wi_level_order_idx
  ON work_item (tenant_id, collection_id, parent_id, order_key COLLATE "C", id)
  WHERE deleted_at IS NULL;
-- The query language's default order: one whole collection, in its manual order. Without the
-- `parent_id` column of the index above, which a query spanning a collection does not constrain
-- (ADR-0026).
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
-- The moments a flow's figure measures by, set wherever a row's column or collection changes
-- (migration 0194).
CREATE OR REPLACE FUNCTION work_item_flow_moments() RETURNS trigger LANGUAGE plpgsql AS
$$
DECLARE
  moment timestamptz;
  first_column uuid;
  completes boolean;
BEGIN
  IF TG_OP = 'UPDATE' THEN
    IF NEW.flow_started_at IS DISTINCT FROM OLD.flow_started_at
       OR NEW.flow_finished_at IS DISTINCT FROM OLD.flow_finished_at THEN
      -- The statement wrote the moments itself: a restore overwriting the row.
      RETURN NEW;
    ELSIF NEW.collection_id IS DISTINCT FROM OLD.collection_id THEN
      NEW.flow_started_at := NULL;
      NEW.flow_finished_at := NULL;
    ELSIF NEW.bucket_id IS NOT DISTINCT FROM OLD.bucket_id THEN
      RETURN NEW;
    END IF;
    moment := NEW.updated_at;
  ELSE
    moment := NEW.created_at;
  END IF;
  IF NEW.bucket_id IS NULL THEN
    RETURN NEW;
  END IF;

  -- The board's first column as a board is answered: by rank, then identifier (ListBuckets).
  SELECT b.id INTO first_column
    FROM bucket b
   WHERE b.tenant_id = NEW.tenant_id AND b.collection_id = NEW.collection_id AND b.deleted_at IS NULL
   ORDER BY b.order_key COLLATE "C", b.id
   LIMIT 1;
  SELECT b.is_done_bucket INTO completes
    FROM bucket b
   WHERE b.tenant_id = NEW.tenant_id AND b.id = NEW.bucket_id;

  IF NEW.bucket_id = first_column AND NEW.flow_started_at IS NULL THEN
    NEW.flow_started_at := moment;
  END IF;
  IF completes AND (TG_OP = 'UPDATE' OR NEW.flow_finished_at IS NULL) THEN
    NEW.flow_finished_at := moment;
  END IF;
  RETURN NEW;
END
$$;
CREATE TRIGGER work_item_flow_moments
  BEFORE INSERT OR UPDATE OF bucket_id, collection_id ON work_item
  FOR EACH ROW EXECUTE FUNCTION work_item_flow_moments();
-- The supplement for the scripts a tsquery cannot serve: CJK and Thai have no word boundaries, so
-- one run of characters is one token and a substring of it is unfindable (i18n-l10n.md §5).
CREATE INDEX wi_search_trgm_idx ON work_item
  USING gin ((coalesce(title, '') || ' ' || coalesce(notes, '')) gin_trgm_ops);
CREATE INDEX wi_custom_idx   ON work_item USING gin (custom_fields jsonb_path_ops);
CREATE INDEX wi_trash_idx    ON work_item (tenant_id, deleted_at) WHERE deleted_at IS NOT NULL;
-- Restoring a deletion is one statement keyed on the batch every row of it shares (I-C2).
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
  kind          text NOT NULL CONSTRAINT custom_field_definition_kind_check
                  CHECK (kind IN ('TEXT','NUMBER','DATE','SELECT','MULTI_SELECT','BOOL','USER','URL',
                                  'MONEY','DURATION','REFERENCE','USER_SET','RICH_TEXT')),
  options       jsonb NOT NULL DEFAULT '[]'::jsonb,
  -- The ISO 4217 code of a MONEY field's amounts, null for every other kind (migration 0211).
  currency      text,
  is_required   boolean NOT NULL DEFAULT false,
  -- Written only by holders of FIELD_WRITE:<key> (migration 0219, UC-ID-27).
  restricted    boolean NOT NULL DEFAULT false,
  applies_to    item_type[] NOT NULL DEFAULT '{TASK}',
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  version       integer     NOT NULL DEFAULT 1,
  deleted_at    timestamptz,
  CONSTRAINT custom_field_definition_collection_id_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT custom_field_definition_currency_check
    CHECK ((kind = 'MONEY') = (currency IS NOT NULL) AND (currency IS NULL OR currency ~ '^[A-Z]{3}$'))
);
CREATE UNIQUE INDEX custom_field_definition_tenant_id_uq ON custom_field_definition (tenant_id, id);
CREATE INDEX cfd_scope_idx ON custom_field_definition (tenant_id, collection_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX cfd_key_uq
  ON custom_field_definition (tenant_id, coalesce(collection_id, '00000000-0000-0000-0000-000000000000'::uuid), key)
  WHERE deleted_at IS NULL;

-- The typed projection of `work_item.custom_fields` the query language ranges, sorts and groups
-- over (migrations 0211, 0212; F-2): one row per value, per element of a set, in the one column its
-- kind compares in. Written by the trigger below in the transaction that writes the document.
CREATE TABLE entry_field_value (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entry_id      uuid NOT NULL,
  definition_id uuid NOT NULL,
  ordinal       integer NOT NULL DEFAULT 0 CHECK (ordinal >= 0),
  num           numeric,          -- NUMBER, MONEY, DURATION, BOOL (1 or 0)
  ts            timestamptz,      -- DATE, midnight UTC
  text          text CHECK (text IS NULL OR char_length(text) <= 256),  -- TEXT, SELECT, MULTI_SELECT, URL
  ref           uuid,             -- USER, USER_SET, REFERENCE
  PRIMARY KEY (tenant_id, entry_id, definition_id, ordinal),
  CHECK (num_nonnulls(num, ts, text, ref) = 1),
  CONSTRAINT entry_field_value_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT entry_field_value_definition_fkey
    FOREIGN KEY (tenant_id, definition_id) REFERENCES custom_field_definition (tenant_id, id)
    ON DELETE CASCADE
);
CREATE INDEX efv_num_idx  ON entry_field_value (tenant_id, definition_id, num)  WHERE num IS NOT NULL;
CREATE INDEX efv_ts_idx   ON entry_field_value (tenant_id, definition_id, ts)   WHERE ts IS NOT NULL;
CREATE INDEX efv_text_idx ON entry_field_value (tenant_id, definition_id, text) WHERE text IS NOT NULL;
CREATE INDEX efv_ref_idx  ON entry_field_value (tenant_id, definition_id, ref)  WHERE ref IS NOT NULL;
CREATE INDEX efv_search_idx ON entry_field_value USING gin (to_tsvector('simple'::regconfig, text)) WHERE text IS NOT NULL;  -- migration 0225
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_field_value TO hubtask_app;
-- entry_field_values_of(work_item) and the trigger work_item_field_values: migration 0211.

-- The slim entry's side tables (migrations 0227, 0228; F-10): what background loops keep on an
-- entry, mirrored from its columns by the trigger work_item_side_rows until the contract release
-- moves the readers here and drops the columns. A row exists only while one of its values does.
CREATE TABLE work_item_announcement (
  tenant_id             uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id               uuid NOT NULL,
  due_soon_announced_at timestamptz,
  overdue_announced_at  timestamptz,
  PRIMARY KEY (tenant_id, item_id),
  CHECK (num_nonnulls(due_soon_announced_at, overdue_announced_at) > 0),
  CONSTRAINT work_item_announcement_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE TABLE work_item_flow (
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id          uuid NOT NULL,
  flow_started_at  timestamptz,
  flow_finished_at timestamptz,
  PRIMARY KEY (tenant_id, item_id),
  CHECK (num_nonnulls(flow_started_at, flow_finished_at) > 0),
  CONSTRAINT work_item_flow_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE TABLE work_item_retention (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id       uuid NOT NULL,
  pending_until timestamptz,
  rule_id       uuid,
  action        text,
  blocked_by    text,
  PRIMARY KEY (tenant_id, item_id),
  CHECK (num_nonnulls(pending_until, rule_id, action, blocked_by) > 0),
  CONSTRAINT work_item_retention_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX work_item_retention_due_idx ON work_item_retention (tenant_id, pending_until)
  WHERE pending_until IS NOT NULL;
GRANT SELECT, INSERT, UPDATE, DELETE ON work_item_announcement, work_item_flow, work_item_retention
  TO hubtask_app;
-- work_item_mirror_side_rows(work_item) and the trigger work_item_side_rows: migration 0227.

-- Durable timers on business calendars (migration 0229, F-10, domain-model.md §3.7).
CREATE TABLE durable_timer (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  purpose          text NOT NULL CHECK (purpose IN ('SLA', 'APPROVAL', 'EFFECTIVENESS_CHECK', 'PAGE_REVIEW')),
  subject_id       uuid NOT NULL,
  name             text NOT NULL CHECK (name ~ '^[a-z][a-z0-9_]{0,49}$'),
  calendar         jsonb NOT NULL,   -- the calendar it was armed on
  budget_seconds   bigint NOT NULL CHECK (budget_seconds > 0),
  consumed_seconds bigint NOT NULL DEFAULT 0 CHECK (consumed_seconds >= 0),
  running_since    timestamptz,
  due_at           timestamptz,
  state            text NOT NULL CHECK (state IN ('RUNNING', 'PAUSED', 'FIRED', 'STOPPED')),
  fired_at         timestamptz,
  created_at       timestamptz NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  CHECK ((state = 'RUNNING') = (running_since IS NOT NULL)),
  CHECK (state <> 'RUNNING' OR due_at IS NOT NULL),
  CHECK ((state = 'FIRED') = (fired_at IS NOT NULL))
);
CREATE UNIQUE INDEX durable_timer_live_uq ON durable_timer (tenant_id, purpose, subject_id, name)
  WHERE state IN ('RUNNING', 'PAUSED');
CREATE INDEX durable_timer_due_idx ON durable_timer (tenant_id, due_at) WHERE state = 'RUNNING';
GRANT SELECT, INSERT, UPDATE, DELETE ON durable_timer TO hubtask_app;

-- Live documents (migration 0230, F-8): a document's CRDT update log, its snapshots and its head.
-- The updates are the editor's bytes and never read here; what a snapshot covers is marked, and
-- removed by the DOCUMENT_HISTORY retention kind.
CREATE TABLE realtime_document (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  kind         text NOT NULL CHECK (kind ~ '^[a-z][a-z_]{1,31}$'),
  document_id  uuid NOT NULL,
  head_seq     bigint NOT NULL DEFAULT 0 CHECK (head_seq >= 0),
  snapshot_seq bigint NOT NULL DEFAULT 0 CHECK (snapshot_seq >= 0 AND snapshot_seq <= head_seq),
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, kind, document_id)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON realtime_document TO hubtask_app;

CREATE TABLE realtime_update (
  tenant_id    uuid NOT NULL,
  kind         text NOT NULL,
  document_id  uuid NOT NULL,
  seq          bigint NOT NULL CHECK (seq > 0),
  data         bytea NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 1048576),
  author_id    uuid,
  created_at   timestamptz NOT NULL,
  compacted_at timestamptz,
  PRIMARY KEY (tenant_id, kind, document_id, seq),
  CONSTRAINT realtime_update_document_fkey FOREIGN KEY (tenant_id, kind, document_id)
    REFERENCES realtime_document (tenant_id, kind, document_id) ON DELETE CASCADE,
  CONSTRAINT realtime_update_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);
CREATE INDEX realtime_update_compacted_idx ON realtime_update (tenant_id, compacted_at)
  WHERE compacted_at IS NOT NULL;
GRANT SELECT, INSERT, UPDATE, DELETE ON realtime_update TO hubtask_app;

CREATE TABLE realtime_snapshot (
  tenant_id     uuid NOT NULL,
  kind          text NOT NULL,
  document_id   uuid NOT NULL,
  up_to_seq     bigint NOT NULL CHECK (up_to_seq > 0),
  state         bytea NOT NULL CHECK (octet_length(state) BETWEEN 1 AND 16777216),
  created_at    timestamptz NOT NULL,
  superseded_at timestamptz,
  PRIMARY KEY (tenant_id, kind, document_id, up_to_seq),
  CONSTRAINT realtime_snapshot_document_fkey FOREIGN KEY (tenant_id, kind, document_id)
    REFERENCES realtime_document (tenant_id, kind, document_id) ON DELETE CASCADE
);
CREATE INDEX realtime_snapshot_superseded_idx ON realtime_snapshot (tenant_id, superseded_at)
  WHERE superseded_at IS NOT NULL;
GRANT SELECT, INSERT, UPDATE, DELETE ON realtime_snapshot TO hubtask_app;

-- Pages of the knowledge module (migration 0231, M3).
CREATE TABLE page (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id           uuid NOT NULL,
  parent_id        uuid,
  title            text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  body             text NOT NULL DEFAULT '' CHECK (octet_length(body) <= 1048576),
  icon             text NOT NULL DEFAULT '' CHECK (char_length(icon) <= 16),
  order_key        text NOT NULL,
  template         boolean NOT NULL DEFAULT false,
  content_language text,
  owner_id         uuid,
  verified_until   timestamptz,
  verified_at      timestamptz,
  verified_by      uuid,
  archived_at      timestamptz,
  deleted_at       timestamptz,
  created_by       uuid NOT NULL,
  created_at       timestamptz NOT NULL,
  updated_by       uuid NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  search_document  tsvector,
  UNIQUE (tenant_id, id),
  CHECK (parent_id IS NULL OR parent_id <> id),
  CONSTRAINT page_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  -- Deferred to the commit: a restore writes pages in identifier order, and a page moved under a
  -- newer one comes before its parent.
  CONSTRAINT page_parent_fkey FOREIGN KEY (tenant_id, parent_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT page_owner_fkey FOREIGN KEY (tenant_id, owner_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (owner_id)
);
-- The tree a space reads: its pages in order under each parent.
CREATE INDEX page_tree_idx ON page (tenant_id, hub_id, parent_id, order_key);
CREATE INDEX page_search_idx ON page USING gin (search_document);
-- The review list: the pages whose verification runs out.
CREATE INDEX page_verified_until_idx ON page (tenant_id, verified_until)
  WHERE verified_until IS NOT NULL AND deleted_at IS NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON page TO hubtask_app;

CREATE OR REPLACE FUNCTION page_search_document() RETURNS trigger
  LANGUAGE plpgsql AS
$$
BEGIN
  NEW.search_document := hubtask_search_document(NEW.content_language, NEW.title, NEW.body);
  RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS page_search_document ON page;
CREATE TRIGGER page_search_document
  BEFORE INSERT OR UPDATE OF title, body, content_language ON page
  FOR EACH ROW EXECUTE FUNCTION page_search_document();

CREATE TABLE page_version (
  tenant_id  uuid NOT NULL,
  page_id    uuid NOT NULL,
  seq        integer NOT NULL CHECK (seq > 0),
  title      text NOT NULL,
  body       text NOT NULL,
  author_id  uuid,
  name       text NOT NULL DEFAULT '' CHECK (char_length(name) <= 120),
  restored   integer NOT NULL DEFAULT 0 CHECK (restored >= 0),
  created_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, page_id, seq),
  CONSTRAINT page_version_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_version_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON page_version TO hubtask_app;

CREATE TABLE page_comment (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  page_id     uuid NOT NULL,
  parent_id   uuid,
  author_id   uuid,
  body        text NOT NULL CHECK (octet_length(body) <= 65536),
  anchor      text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  quote       text NOT NULL DEFAULT '' CHECK (char_length(quote) <= 500),
  resolved_at timestamptz,
  resolved_by uuid,
  created_at  timestamptz NOT NULL,
  edited_at   timestamptz,
  deleted_at  timestamptz,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT page_comment_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT page_comment_parent_fkey FOREIGN KEY (tenant_id, parent_id)
    REFERENCES page_comment (tenant_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT page_comment_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);
CREATE INDEX page_comment_page_idx ON page_comment (tenant_id, page_id, created_at);

GRANT SELECT, INSERT, UPDATE, DELETE ON page_comment TO hubtask_app;

CREATE TABLE page_reference (
  tenant_id   uuid NOT NULL,
  page_id     uuid NOT NULL,
  target_kind text NOT NULL CHECK (target_kind IN
                ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT')),
  target_id   uuid NOT NULL,
  anchor      text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  strength    text NOT NULL CHECK (strength IN ('MENTION', 'CARD', 'EMBED')),
  PRIMARY KEY (tenant_id, page_id, target_kind, target_id, anchor),
  CONSTRAINT page_reference_page_fkey FOREIGN KEY (tenant_id, page_id)
    REFERENCES page (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX page_reference_target_idx ON page_reference (tenant_id, target_kind, target_id);

GRANT SELECT, INSERT, DELETE ON page_reference TO hubtask_app;

CREATE OR REPLACE FUNCTION page_reference_of_body() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM page_reference WHERE tenant_id = NEW.tenant_id AND page_id = NEW.id;
  -- A trashed page points nowhere until it is restored.
  IF NEW.deleted_at IS NULL AND position('hubtask:' IN NEW.body) > 0 THEN
    INSERT INTO page_reference (tenant_id, page_id, target_kind, target_id, anchor, strength)
    SELECT NEW.tenant_id, NEW.id, r.target_kind, r.target_id, r.anchor, r.strength
      FROM entry_references_of(NEW.body) AS r
     WHERE NOT (r.target_kind = 'PAGE' AND r.target_id = NEW.id);
  END IF;
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS page_references_insert ON page;
CREATE TRIGGER page_references_insert AFTER INSERT ON page
  FOR EACH ROW EXECUTE FUNCTION page_reference_of_body();
DROP TRIGGER IF EXISTS page_references_update ON page;
CREATE TRIGGER page_references_update AFTER UPDATE OF body, deleted_at ON page
  FOR EACH ROW EXECUTE FUNCTION page_reference_of_body();

-- Boards of the whiteboard module (migration 0233, M4).
CREATE TABLE whiteboard (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id     uuid NOT NULL,
  title      text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
  scene      jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(scene) = 'array' AND jsonb_array_length(scene) <= 5000),
  deleted_at timestamptz,
  created_by uuid NOT NULL,
  created_at timestamptz NOT NULL,
  updated_by uuid NOT NULL,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT whiteboard_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX whiteboard_hub_idx ON whiteboard (tenant_id, hub_id, updated_at DESC);

ALTER TABLE whiteboard ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON whiteboard;
CREATE POLICY tenant_isolation ON whiteboard
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON whiteboard TO hubtask_app;

CREATE TABLE whiteboard_reference (
  tenant_id     uuid NOT NULL,
  whiteboard_id uuid NOT NULL,
  target_kind   text NOT NULL CHECK (target_kind IN
                  ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT')),
  target_id     uuid NOT NULL,
  anchor        text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  strength      text NOT NULL CHECK (strength IN ('MENTION', 'CARD', 'EMBED')),
  PRIMARY KEY (tenant_id, whiteboard_id, target_kind, target_id, anchor),
  CONSTRAINT whiteboard_reference_board_fkey FOREIGN KEY (tenant_id, whiteboard_id)
    REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX whiteboard_reference_target_idx
  ON whiteboard_reference (tenant_id, target_kind, target_id);

ALTER TABLE whiteboard_reference ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard_reference FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON whiteboard_reference;
CREATE POLICY tenant_isolation ON whiteboard_reference
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON whiteboard_reference TO hubtask_app;

CREATE OR REPLACE FUNCTION whiteboard_reference_of_scene() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM whiteboard_reference WHERE tenant_id = NEW.tenant_id AND whiteboard_id = NEW.id;
  -- A trashed board points nowhere until it is restored.
  IF NEW.deleted_at IS NULL THEN
    INSERT INTO whiteboard_reference (tenant_id, whiteboard_id, target_kind, target_id, anchor, strength)
    SELECT NEW.tenant_id, NEW.id, found.target_kind, found.target_id, found.anchor,
           (array_agg(found.strength ORDER BY found.rank DESC))[1]
    FROM (
      SELECT e #>> '{target,kind}' AS target_kind, (e #>> '{target,id}')::uuid AS target_id,
             '' AS anchor, 'CARD' AS strength, 1 AS rank
        FROM jsonb_array_elements(NEW.scene) AS e
       WHERE e ->> 'kind' = 'CARD' AND e #>> '{target,id}' IS NOT NULL
      UNION ALL
      SELECT r.target_kind, r.target_id, r.anchor, r.strength,
             CASE r.strength WHEN 'EMBED' THEN 2 WHEN 'CARD' THEN 1 ELSE 0 END
        FROM jsonb_array_elements(NEW.scene) AS e, entry_references_of(e ->> 'text') AS r
       WHERE position('hubtask:' IN coalesce(e ->> 'text', '')) > 0
    ) AS found
    WHERE NOT (found.target_kind = 'WHITEBOARD' AND found.target_id = NEW.id)
    GROUP BY found.target_kind, found.target_id, found.anchor;
  END IF;
  RETURN NULL;
END $$;

DROP TRIGGER IF EXISTS whiteboard_references_insert ON whiteboard;
CREATE TRIGGER whiteboard_references_insert AFTER INSERT ON whiteboard
  FOR EACH ROW EXECUTE FUNCTION whiteboard_reference_of_scene();
DROP TRIGGER IF EXISTS whiteboard_references_update ON whiteboard;
CREATE TRIGGER whiteboard_references_update AFTER UPDATE OF scene, deleted_at ON whiteboard
  FOR EACH ROW EXECUTE FUNCTION whiteboard_reference_of_scene();

-- A whiteboard's history (migration 0234, UC-CNV-06): one row per drawing session, a named
-- version its own row.
CREATE TABLE whiteboard_version (
  tenant_id     uuid NOT NULL,
  whiteboard_id uuid NOT NULL,
  seq           integer NOT NULL CHECK (seq > 0),
  title         text NOT NULL,
  scene         jsonb NOT NULL CHECK (jsonb_typeof(scene) = 'array' AND jsonb_array_length(scene) <= 5000),
  author_id     uuid,
  name          text NOT NULL DEFAULT '' CHECK (char_length(name) <= 120),
  created_at    timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, whiteboard_id, seq),
  CONSTRAINT whiteboard_version_board_fkey FOREIGN KEY (tenant_id, whiteboard_id)
    REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT whiteboard_version_author_fkey FOREIGN KEY (tenant_id, author_id)
    REFERENCES account (tenant_id, id) ON DELETE SET NULL (author_id)
);
ALTER TABLE whiteboard_version ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard_version FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON whiteboard_version
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON whiteboard_version TO hubtask_app;

-- Links and references (migration 0213, F-3). The six shipped link types have no row.
CREATE TABLE link_type (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'
                 AND key NOT IN ('blocks', 'depends_on', 'duplicates', 'runs_on', 'affects', 'implements')),
  name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  inverse_name text NOT NULL CHECK (char_length(inverse_name) <= 120),
  direction    text NOT NULL CHECK (direction IN ('DIRECTED', 'SYMMETRIC')),
  cardinality  text NOT NULL CHECK (cardinality IN ('MANY', 'ONE')),
  cycles       text NOT NULL CHECK (cycles IN ('FORBIDDEN', 'ALLOWED')),
  waits        text NOT NULL CHECK (waits IN ('NONE', 'SOURCE', 'TARGET')),
  position     integer NOT NULL DEFAULT 0,
  archived_at  timestamptz,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT link_type_key_uq UNIQUE (tenant_id, key),
  CHECK (direction = 'DIRECTED' OR (waits = 'NONE' AND cardinality = 'MANY')),
  CHECK (direction = 'SYMMETRIC' OR inverse_name <> ''),
  CHECK (waits = 'NONE' OR cycles = 'FORBIDDEN')
);
GRANT SELECT, INSERT, UPDATE, DELETE ON link_type TO hubtask_app;

CREATE TABLE entry_link (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  link_type   text NOT NULL CHECK (link_type ~ '^[a-z][a-z0-9_]{0,49}$'),
  own_type    text GENERATED ALWAYS AS (
                CASE WHEN link_type IN ('blocks', 'depends_on', 'duplicates', 'runs_on', 'affects', 'implements')
                     THEN NULL ELSE link_type END) STORED,
  source_id   uuid NOT NULL,
  target_id   uuid NOT NULL,
  created_by  uuid,
  created_at  timestamptz NOT NULL,
  -- A waiting link's timing (migration 0244, M2).
  dependency_kind text NOT NULL DEFAULT 'FS' CHECK (dependency_kind IN ('FS', 'SS', 'FF', 'SF')),
  lag_days    integer NOT NULL DEFAULT 0 CHECK (lag_days BETWEEN -999 AND 999),
  UNIQUE (tenant_id, id),
  CONSTRAINT entry_link_uq UNIQUE (tenant_id, link_type, source_id, target_id),
  CHECK (source_id <> target_id),
  CONSTRAINT entry_link_source_fkey
    FOREIGN KEY (tenant_id, source_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT entry_link_target_fkey
    FOREIGN KEY (tenant_id, target_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT entry_link_type_fkey
    FOREIGN KEY (tenant_id, own_type) REFERENCES link_type (tenant_id, key)
);
CREATE INDEX entry_link_source_idx ON entry_link (tenant_id, source_id, link_type);
CREATE INDEX entry_link_target_idx ON entry_link (tenant_id, target_id, link_type);
CREATE INDEX entry_link_own_type_idx ON entry_link (tenant_id, own_type) WHERE own_type IS NOT NULL;
GRANT SELECT, INSERT, DELETE ON entry_link TO hubtask_app;
GRANT UPDATE (dependency_kind, lag_days) ON entry_link TO hubtask_app;

CREATE TABLE entry_reference (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  source_kind text NOT NULL CHECK (source_kind IN ('ENTRY', 'COMMENT')),
  source_id   uuid NOT NULL,
  entry_id    uuid NOT NULL,
  target_kind text NOT NULL CHECK (target_kind IN
                ('ENTRY', 'HUB', 'COLLECTION', 'VIEW', 'PAGE', 'WHITEBOARD', 'DASHBOARD', 'GOAL', 'SPRINT')),
  target_id   uuid NOT NULL,
  anchor      text NOT NULL DEFAULT '' CHECK (anchor ~ '^[A-Za-z0-9_-]{0,64}$'),
  strength    text NOT NULL CHECK (strength IN ('MENTION', 'CARD', 'EMBED')),
  PRIMARY KEY (tenant_id, source_kind, source_id, target_kind, target_id, anchor),
  CONSTRAINT entry_reference_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX entry_reference_target_idx ON entry_reference (tenant_id, target_kind, target_id);
GRANT SELECT, INSERT, DELETE ON entry_reference TO hubtask_app;
-- entry_references_of(text) and the triggers work_item_references_* and comment_references_*,
-- which keep entry_reference from an entry's notes and its comments: migration 0213.

CREATE TABLE comment (
  id                uuid PRIMARY KEY,
  tenant_id         uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id           uuid NOT NULL,
  author_id         uuid NOT NULL,
  parent_comment_id uuid,
  -- Conditional since migration 0012: a tombstone's body is empty - deletion clears the text
  -- rather than hiding it - while a living comment carries 1 to 20000 characters. A tombstone a
  -- legal hold covers keeps its text until the retention pass clears it once no hold covers it
  -- (data-retention.md §4); no read serves it (Comment.sql).
  body              text NOT NULL CHECK (deleted_at IS NOT NULL OR length(body) BETWEEN 1 AND 20000),
  created_at        timestamptz NOT NULL DEFAULT now(),
  edited_at         timestamptz,
  deleted_at        timestamptz,
  version           integer NOT NULL DEFAULT 1,
  -- What somebody wrote, or what the server filed on their behalf: the displaced version of a
  -- free-text field that lost a merge (migration 0084, offline-sync.md §5). A system comment's
  -- heading is a message code with parameters, never a sentence the server composed.
  kind              text NOT NULL DEFAULT 'USER' CONSTRAINT comment_kind_known CHECK (kind IN ('USER', 'SYSTEM')),
  system_code       text,
  system_params     jsonb,
  CONSTRAINT comment_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX comment_tenant_id_uq ON comment (tenant_id, id);
-- A search finds an entry by its comments (migration 0225, F-9).
CREATE INDEX comment_search_idx ON comment USING gin (to_tsvector('simple'::regconfig, body)) WHERE deleted_at IS NULL;
ALTER TABLE comment ADD CONSTRAINT comment_parent_comment_id_fkey
    FOREIGN KEY (tenant_id, parent_comment_id) REFERENCES comment (tenant_id, id) ON DELETE CASCADE;
CREATE INDEX comment_item_idx ON comment (tenant_id, item_id, created_at);
-- The tombstones whose text a legal hold keeps, which the retention pass reads (migration 0122).
CREATE INDEX comment_kept_text_idx ON comment (tenant_id, id)
  WHERE deleted_at IS NOT NULL AND body <> '';

-- The last moment a collection, or every collection of a hub, was left with nothing open
-- (UC-WRK-29 check 4). Names nobody. Migration 0190.
CREATE TABLE container_milestone (
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  container_id uuid NOT NULL,
  reached_at   timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, container_id),
  CONSTRAINT container_milestone_container_fkey FOREIGN KEY (tenant_id, container_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);

-- One person's acknowledgement of another's completion (UC-WRK-29 check 5): read only by whom it
-- was given to, never exported, audited or announced. Migration 0190.
CREATE TABLE kudos (
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id   uuid NOT NULL,
  given_by  uuid NOT NULL,
  given_to  uuid NOT NULL,
  given_at  timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, item_id, given_by, given_to),
  CONSTRAINT kudos_not_to_oneself CHECK (given_by <> given_to),
  CONSTRAINT kudos_item_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT kudos_given_by_fkey FOREIGN KEY (tenant_id, given_by)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT kudos_given_to_fkey FOREIGN KEY (tenant_id, given_to)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX kudos_given_to_idx ON kudos (tenant_id, given_to, item_id);
CREATE INDEX kudos_given_by_idx ON kudos (tenant_id, given_by);

-- A preset saved from a hub's or collection's structure (UC-WRK-30): the workspace's, or one
-- person's own, gone with their account. `settings` is a layer's settings document without
-- categories; the name is the person's content (rule 10). Migration 0197.
CREATE TABLE structure_preset (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  id         uuid NOT NULL,
  origin     text NOT NULL,
  owner_id   uuid,
  name       text NOT NULL,
  settings   jsonb NOT NULL,
  created_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  PRIMARY KEY (tenant_id, id),
  CONSTRAINT structure_preset_origin_check CHECK (origin IN ('WORKSPACE', 'OWN')),
  CONSTRAINT structure_preset_owner_check CHECK ((origin = 'OWN') = (owner_id IS NOT NULL)),
  CONSTRAINT structure_preset_name_check CHECK (char_length(name) BETWEEN 1 AND 100),
  CONSTRAINT structure_preset_owner_fkey FOREIGN KEY (tenant_id, owner_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX structure_preset_origin_owner_idx ON structure_preset (tenant_id, origin, owner_id);

-- Partitioned by month (multi-tenancy.md §7), constructed exactly as migration 0068
-- leaves a converted installation: a standalone history partition carrying the old shapes, a
-- partitioned parent under the working name, a default catch-all, and the coming month through
-- ensure_stream_partition below. The history is empty on a fresh install and holds everything
-- older than the conversion on a migrated one - one construction, so the schema reference and
-- the migrations agree object for object.
CREATE TABLE activity_entry_history (
  id           uuid NOT NULL,
  tenant_id    uuid NOT NULL CONSTRAINT activity_entry_tenant_id_fkey REFERENCES tenant(id) ON DELETE CASCADE,
  -- nullable, and the key below is MATCH SIMPLE: container_id is there for a container's own
  -- history, which has no reader yet. An entry without an item is refused by the domain.
  item_id      uuid,
  container_id uuid,
  -- Named like the parent's: the attach matches check constraints by name.
  actor_type   text NOT NULL CONSTRAINT activity_entry_actor_type_check CHECK (actor_type IN ('USER','SERVICE_ACCOUNT','AUTOMATION','AI_AGENT','SYSTEM')),
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
CREATE INDEX activity_page_idx ON activity_entry_history (tenant_id, item_id, occurred_at DESC, id DESC);
CREATE UNIQUE INDEX activity_entry_part_pkey ON activity_entry_history (tenant_id, occurred_at, id);
CREATE UNIQUE INDEX activity_entry_history_id_idx ON activity_entry_history (id);

CREATE TABLE activity_entry (
  id           uuid NOT NULL,
  tenant_id    uuid NOT NULL CONSTRAINT activity_entry_tenant_id_fkey REFERENCES tenant(id) ON DELETE CASCADE,
  item_id      uuid,
  container_id uuid,
  actor_type   text NOT NULL CONSTRAINT activity_entry_actor_type_check CHECK (actor_type IN ('USER','SERVICE_ACCOUNT','AUTOMATION','AI_AGENT','SYSTEM')),
  actor_id     uuid,
  verb         text NOT NULL,
  change_set   jsonb NOT NULL DEFAULT '{}'::jsonb,
  occurred_at  timestamptz NOT NULL DEFAULT now(),
  correlation_id uuid,
  causation_id   uuid,
  -- The partition key must be part of every unique constraint (the audit_log lesson).
  PRIMARY KEY (tenant_id, occurred_at, id),
  CONSTRAINT activity_entry_item_id_fkey FOREIGN KEY (tenant_id, item_id)
    REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
) PARTITION BY RANGE (occurred_at);
CREATE INDEX activity_entry_page_idx ON activity_entry (tenant_id, item_id, occurred_at DESC, id DESC);

DO $activity_attach$
BEGIN
  EXECUTE format(
    'ALTER TABLE activity_entry ATTACH PARTITION activity_entry_history FOR VALUES FROM (MINVALUE) TO (%L)',
    (date_trunc('month', now()) + interval '1 month')::date);
END $activity_attach$;
CREATE TABLE activity_entry_default PARTITION OF activity_entry DEFAULT;

CREATE TABLE media_object (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  storage_key text NOT NULL,
  mime_type   text NOT NULL,
  byte_size   bigint NOT NULL CHECK (byte_size >= 0),
  checksum    text,
  usage       text NOT NULL CONSTRAINT media_object_usage_check
              CHECK (usage IN ('COVER', 'ATTACHMENT', 'IMPORT', 'EXPORT', 'AVATAR')),
  ref_count   integer NOT NULL DEFAULT 0,
  -- NULL where nobody uploaded it: a mail attachment arrives over an intake that authenticates the
  -- tenant and no person, and an account here would be an uploader this system invented
  -- (migration 0061).
  created_by  uuid,
  created_at  timestamptz NOT NULL DEFAULT now(),
  deleted_at  timestamptz,
  -- The upload life (migration 0013): PENDING between staging and confirmation, READY once
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
-- A search finds an entry by the name of a file attached to it (migration 0226, F-9).
CREATE INDEX media_object_name_search_idx ON media_object USING gin (to_tsvector('simple'::regconfig, translate(file_name, '._-', '   '))) WHERE file_name IS NOT NULL AND deleted_at IS NULL;
-- Declared here rather than in work_item's definition, because media_object is created after it
-- in this file; migration 0013 is where it arrived.
ALTER TABLE work_item ADD CONSTRAINT work_item_cover_media_fkey
  FOREIGN KEY (tenant_id, cover_media_id) REFERENCES media_object (tenant_id, id) ON DELETE RESTRICT;

-- A whiteboard's pictures and files (migration 0235, UC-CNV-08), placed by AttachWhiteboardMedia.
CREATE TABLE whiteboard_media (
  tenant_id     uuid NOT NULL,
  whiteboard_id uuid NOT NULL,
  media_id      uuid NOT NULL,
  PRIMARY KEY (tenant_id, whiteboard_id, media_id),
  CONSTRAINT whiteboard_media_board_fkey FOREIGN KEY (tenant_id, whiteboard_id)
    REFERENCES whiteboard (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT whiteboard_media_object_fkey FOREIGN KEY (tenant_id, media_id)
    REFERENCES media_object (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX whiteboard_media_object_idx ON whiteboard_media (tenant_id, media_id);
ALTER TABLE whiteboard_media ENABLE ROW LEVEL SECURITY;
ALTER TABLE whiteboard_media FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON whiteboard_media
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON whiteboard_media TO hubtask_app;
-- Cleared rather than refused when the picture goes: a replace-restore and an erasure both remove
-- media before the account row is done with (migration 0199).
ALTER TABLE account ADD CONSTRAINT account_avatar_media_fkey
  FOREIGN KEY (tenant_id, avatar_media_id) REFERENCES media_object (tenant_id, id)
  ON DELETE SET NULL (avatar_media_id);

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
-- the two could disagree about which rule an entry repeats by.
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
-- Two live templates in one scope may not share a name; a deleted one frees it.
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
  -- Superseded by `ai_suggestion`. A proposal is a record with provenance, a decision
  -- and a period, and none of that fits a column; nothing has ever written this one. Kept for
  -- one release because a rolling update's old pods still select it, and dropped by the
  -- contract half of expand/contract once they are gone (rule 12).
  suggestion   jsonb,
  status       text NOT NULL DEFAULT 'NEW' CHECK (status IN ('NEW','PROCESSED','DISMISSED')),
  target_item_id uuid,
  received_at  timestamptz NOT NULL DEFAULT now(),
  processed_at timestamptz,
  -- The account that submitted the entry, where an account did; empty for a mail or a webhook
  -- (migration 0196). What keeps a kept-out person's own capture out of AI (data-protection.md §4.1).
  created_by   uuid,
  -- The place a mail was addressed to where it waits here instead (migration 0201, ADR-0092).
  meant_for_kind text,
  meant_for_id   uuid,
  -- Why the mail intake put a message here (migration 0205, ADR-0094).
  intake_reason  text
);
CREATE INDEX jumble_status_idx ON jumble_entry (tenant_id, status, received_at DESC);

-- The jumble's webhook intake (migration 0060): one token-protected address per tenant,
-- stored as a hash under the intake's own purpose label. Rotating replaces it in one statement.
CREATE TABLE jumble_intake (
  tenant_id  uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  token_hash bytea NOT NULL,
  rotated_at timestamptz NOT NULL
);
CREATE UNIQUE INDEX jumble_intake_token_uq ON jumble_intake (token_hash);

-- The mail intake (migration 0205, ADR-0094): a workspace's settings - its name on the operator's
-- receiving domain unique on the installation - and a receipt per message, one per Message-ID.
CREATE TABLE mail_setting (
  tenant_id             uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  enabled               boolean NOT NULL DEFAULT false,
  name                  text CHECK (name IS NULL OR name ~ '^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$'),
  default_collection_id uuid,
  sender_level          text NOT NULL DEFAULT 'MEMBERS'
                        CHECK (sender_level IN ('MEMBERS', 'MEMBERS_AND_ALLOWED', 'ANYONE')),
  allowed               text[] NOT NULL DEFAULT '{}',
  blocked               text[] NOT NULL DEFAULT '{}',
  sender_routes         jsonb NOT NULL DEFAULT '[]'::jsonb,
  opened_by             uuid,
  updated_at            timestamptz NOT NULL,
  updated_by            uuid
);
CREATE UNIQUE INDEX mail_setting_name_uq ON mail_setting (name) WHERE name IS NOT NULL;

-- The inbound route's one question before it has a tenant (multi-tenancy.md §2.1).
CREATE OR REPLACE FUNCTION resolve_mail_workspace(workspace_name text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT tenant_id FROM mail_setting
  WHERE name = lower(workspace_name) AND enabled
  LIMIT 1
$$;

CREATE OR REPLACE FUNCTION mail_workspace_name_taken(workspace_name text, self uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT EXISTS (SELECT 1 FROM mail_setting WHERE name = lower(workspace_name) AND tenant_id <> self)
$$;

CREATE TABLE mail_receipt (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  message_id  text NOT NULL,
  received_at timestamptz NOT NULL,
  sender      text NOT NULL,
  subject     text NOT NULL,
  outcome     text NOT NULL CHECK (outcome IN ('ENTRY', 'COMMENT', 'JUMBLE', 'DISCARDED')),
  reason      text NOT NULL DEFAULT '',
  item_id     uuid,
  entry_id    uuid,
  door        text NOT NULL,
  UNIQUE (tenant_id, message_id)
);
CREATE INDEX mail_receipt_recent_idx ON mail_receipt (tenant_id, received_at DESC);

-- Connected mailboxes (migration 0206, ADR-0094): the secret sealed under the installation's key.
CREATE TABLE mailbox (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  protocol      text NOT NULL CHECK (protocol IN ('IMAP', 'JMAP')),
  address       text NOT NULL,
  host          text,
  port          integer CHECK (port IS NULL OR port BETWEEN 1 AND 65535),
  session_url   text,
  username      text NOT NULL,
  folder        text NOT NULL,
  done_folder   text NOT NULL,
  preset        text,
  enabled       boolean NOT NULL DEFAULT true,
  secret_enc    bytea NOT NULL,
  secret_key_id text NOT NULL,
  checked_at    timestamptz,
  failure       text,
  failing_since timestamptz,
  -- PASSWORD, or OAUTH: then the sealed secret is the refresh token, and the row names the app.
  auth          text NOT NULL DEFAULT 'PASSWORD' CHECK (auth IN ('PASSWORD', 'OAUTH')),
  oauth_provider      text,
  oauth_authorize_url text,
  oauth_token_url     text,
  oauth_scopes        text[],
  oauth_client_id     text,
  oauth_installation  boolean NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL,
  created_by    uuid,
  UNIQUE (tenant_id, id),
  CHECK ((auth = 'OAUTH') = (oauth_provider IS NOT NULL AND oauth_token_url IS NOT NULL)),
  CHECK ((protocol = 'IMAP') = (host IS NOT NULL AND port IS NOT NULL)),
  CHECK ((protocol = 'JMAP') = (session_url IS NOT NULL))
);
CREATE UNIQUE INDEX mailbox_address_uq ON mailbox (tenant_id, lower(address));

-- A mailbox connection waiting for its provider's answer (migration 0206): used once, ten
-- minutes, bound to the administrator who asked; the state kept only as its hash.
CREATE TABLE mailbox_authorization (
  id                   uuid PRIMARY KEY,
  tenant_id            uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id           uuid NOT NULL,
  state_hash           bytea NOT NULL,
  provider             text NOT NULL,
  authorize_url        text NOT NULL,
  token_url            text NOT NULL,
  scopes               text[] NOT NULL,
  client_id            text NOT NULL,
  installation         boolean NOT NULL,
  host                 text NOT NULL,
  port                 integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
  login_hint           text,
  verifier_enc         bytea NOT NULL,
  verifier_key_id      text NOT NULL,
  client_secret_enc    bytea,
  client_secret_key_id text,
  expires_at           timestamptz NOT NULL,
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX mailbox_authorization_state_uq ON mailbox_authorization (tenant_id, state_hash);

-- Voice assistants (migration 0202, ADR-0093): a workspace's settings and the names people may
-- say for a place.
CREATE TABLE voice_setting (
  tenant_id             uuid PRIMARY KEY REFERENCES tenant(id) ON DELETE CASCADE,
  enabled               boolean NOT NULL DEFAULT false,
  default_collection_id uuid,
  skill_id              text,
  -- The name the skill is spoken to by; empty for the language's default (migration 0203).
  invocation_name       text CHECK (invocation_name IS NULL OR length(invocation_name) BETWEEN 2 AND 50),
  updated_at            timestamptz NOT NULL,
  updated_by            uuid
);

CREATE TABLE voice_alias (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  phrase        text NOT NULL,
  folded        text NOT NULL,
  collection_id uuid NOT NULL,
  label_id      uuid,
  created_at    timestamptz NOT NULL,
  UNIQUE (tenant_id, id)
);
CREATE UNIQUE INDEX voice_alias_phrase_uq ON voice_alias (tenant_id, folded);

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
-- One policy per scope (migration 0011): the auto_assign key of one container's policies
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
  -- When a SCHEDULE rule next fires (migration 0055). NULL for the five triggers that are
  -- not a schedule, and for a schedule whose rule is exhausted.
  next_run_at timestamptz,
  -- The address an INBOUND_WEBHOOK rule answers on (migration 0057, automation.md §1.1): the
  -- token is hashed, answered once, and revoked by rotating; the moment is the only thing about
  -- it a listing may show.
  inbound_token_hash bytea,
  inbound_rotated_at timestamptz,
  -- What the check found (ADR-0060, migration 0095): the findings as a document, empty for a
  -- rule with nothing wrong and for one never checked, and the moment that tells the two apart.
  findings   jsonb NOT NULL DEFAULT '[]'::jsonb,
  checked_at timestamptz,
  -- Switched off by the workspace's deletion request (0130), so that a cancel switches exactly
  -- these back on.
  paused_for_deletion boolean NOT NULL DEFAULT false,
  CONSTRAINT automation_rule_run_as_fkey
    FOREIGN KEY (tenant_id, run_as) REFERENCES account (tenant_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX automation_rule_tenant_id_uq ON automation_rule (tenant_id, id);
CREATE INDEX rule_trigger_idx ON automation_rule (tenant_id, enabled)
  WHERE deleted_at IS NULL;
-- What the dispatcher asks per event (migration 0053): the enabled rules whose trigger is
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

-- Partitioned by month (range on started_at), activity_entry's construction.
CREATE TABLE rule_run_history (
  id           uuid NOT NULL,
  tenant_id    uuid NOT NULL CONSTRAINT rule_run_tenant_id_fkey REFERENCES tenant(id) ON DELETE CASCADE,
  rule_id      uuid NOT NULL,
  event_id     uuid,
  -- What started the run (migration 0054). On the run rather than read back from the rule,
  -- because a rule can be edited from one kind into another and a log that resolved the kind at
  -- read time would rewrite its own history.
  trigger      text NOT NULL DEFAULT 'EVENT',
  -- Who pulled it, for the one kind a person pulls. No foreign key: a run has to outlive the
  -- account that started it, exactly as it outlives the rule it belongs to.
  triggered_by uuid,
  -- The entry the run is about when no event names it - a RELATIVE_DATE run measured from one
  -- entry's due date.
  subject_id   uuid,
  -- 'WAITING' is a run parked on a WAIT action (migration 0058): its results so far are
  -- written and a scheduled job holds the resume point. Its own status, because a row left in
  -- RUNNING is how a crash is recognised.
  status       text NOT NULL CONSTRAINT rule_run_status_check CHECK (status IN ('RUNNING','WAITING','SUCCEEDED','SKIPPED','FAILED','ABORTED_LOOP','THROTTLED')),
  condition_results jsonb NOT NULL DEFAULT '[]'::jsonb,
  action_results    jsonb NOT NULL DEFAULT '[]'::jsonb,
  -- What made this run one occurrence (migration 0059): the idempotency key's middle third,
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
CREATE INDEX rule_run_idx ON rule_run_history (tenant_id, rule_id, started_at DESC);
CREATE INDEX rule_run_trigger_idx ON rule_run_history (tenant_id, trigger, id DESC);
CREATE UNIQUE INDEX rule_run_part_pkey ON rule_run_history (tenant_id, started_at, id);
CREATE UNIQUE INDEX rule_run_history_id_idx ON rule_run_history (id);

CREATE TABLE rule_run (
  id           uuid NOT NULL,
  tenant_id    uuid NOT NULL CONSTRAINT rule_run_tenant_id_fkey REFERENCES tenant(id) ON DELETE CASCADE,
  rule_id      uuid NOT NULL,
  event_id     uuid,
  trigger      text NOT NULL DEFAULT 'EVENT',
  triggered_by uuid,
  subject_id   uuid,
  status       text NOT NULL CONSTRAINT rule_run_status_check CHECK (status IN ('RUNNING','WAITING','SUCCEEDED','SKIPPED','FAILED','ABORTED_LOOP','THROTTLED')),
  condition_results jsonb NOT NULL DEFAULT '[]'::jsonb,
  action_results    jsonb NOT NULL DEFAULT '[]'::jsonb,
  occasion     text,
  error_code   text,
  started_at   timestamptz NOT NULL DEFAULT now(),
  finished_at  timestamptz,
  causation_depth integer NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, started_at, id),
  CONSTRAINT rule_run_rule_id_fkey
    FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT rule_run_trigger_check
    CHECK (trigger IN ('EVENT','SCHEDULE','RELATIVE_DATE','INBOUND_WEBHOOK','MANUAL','JUMBLE_ENTRY'))
) PARTITION BY RANGE (started_at);
CREATE INDEX rule_run_rule_idx ON rule_run (tenant_id, rule_id, started_at DESC);
-- What the run listing's trigger filter asks: this tenant's runs of one kind, newest first.
CREATE INDEX rule_run_page_idx ON rule_run (tenant_id, trigger, id DESC);

DO $rule_run_attach$
BEGIN
  EXECUTE format(
    'ALTER TABLE rule_run ATTACH PARTITION rule_run_history FOR VALUES FROM (MINVALUE) TO (%L)',
    (date_trunc('month', now()) + interval '1 month')::date);
END $rule_run_attach$;
CREATE TABLE rule_run_default PARTITION OF rule_run DEFAULT;

-- What a RELATIVE_DATE rule owes for one entry, and when (migration 0056). The reminder's shape
-- rather than a new one: `reminder` carries "this entry, this moment", and a relative-date rule is the same fact with a rule in place of a person.
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
  -- Where it listens (migration 0156): a hub or a collection, both NULL for the whole workspace.
  scope_type    text,
  scope_id      uuid,
  secret_enc    bytea NOT NULL,
  state         text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE','PAUSED','DISABLED')),
  failure_count integer NOT NULL DEFAULT 0,
  -- The key a sealed value opens under. An installation that has rotated its keyring holds
  -- several, so the ciphertext alone is not enough.
  secret_key_id         text,
  -- The rotation grace: one previous secret, verifying until this moment. A pair rather
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
  version       integer NOT NULL DEFAULT 1,
  CONSTRAINT webhook_subscription_scope_check CHECK (
    (scope_type IS NULL AND scope_id IS NULL)
    OR (scope_type IN ('HUB','COLLECTION') AND scope_id IS NOT NULL)
  )
);
CREATE UNIQUE INDEX webhook_subscription_tenant_id_uq ON webhook_subscription (tenant_id, id);

CREATE TABLE webhook_delivery (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  subscription_id uuid NOT NULL,
  event_id        uuid NOT NULL,
  attempt         integer NOT NULL DEFAULT 1,
  status          text NOT NULL CONSTRAINT webhook_delivery_status_check
                    CHECK (status IN ('PENDING','SUCCEEDED','FAILED','DEAD_LETTER','FILTER_FAILED')),
  response_status integer,
  error_code      text,
  next_attempt_at timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  push_id         uuid,
  subject         text,
  event_type      text,
  CONSTRAINT webhook_delivery_subscription_id_fkey
    FOREIGN KEY (tenant_id, subscription_id) REFERENCES webhook_subscription (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX delivery_retry_idx ON webhook_delivery (tenant_id, status, next_attempt_at);
CREATE INDEX webhook_delivery_collapse_idx
  ON webhook_delivery (tenant_id, subscription_id, push_id, subject, event_type)
  WHERE status = 'PENDING' AND push_id IS NOT NULL;
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

-- ======================= The emergency access to a private hub =======================
-- One row per opening by the workspace owner (ADR-0073 §4, migration 0138): the hub, who, why and
-- the hour. The reason is the members' record of who opened their hub, kept with the hub and gone
-- with it.
CREATE TABLE private_hub_access (
  id        uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id    uuid NOT NULL,
  opened_by uuid,
  reason    text NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 500),
  opened_at timestamptz NOT NULL,
  ends_at   timestamptz NOT NULL,
  CONSTRAINT private_hub_access_hour_check CHECK (ends_at > opened_at),
  UNIQUE (tenant_id, id),
  CONSTRAINT private_hub_access_hub_id_fkey
    FOREIGN KEY (tenant_id, hub_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT private_hub_access_opened_by_fkey
    FOREIGN KEY (tenant_id, opened_by) REFERENCES account (tenant_id, id) ON DELETE SET NULL (opened_by)
);
CREATE INDEX private_hub_access_hub_idx ON private_hub_access (tenant_id, hub_id, opened_at DESC);

-- ============================== Notification ===============================

-- What somebody is to be told, and how far that got. References and no content: what an
-- email says is read from the entry when it is rendered, never copied here.
CREATE TABLE notification (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  recipient_id uuid NOT NULL,
  -- RETENTION since migration 0062: the advance warning of data-retention.md §6 (R-1).
  category     text NOT NULL CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS')),
  channel      text NOT NULL CHECK (channel IN ('EMAIL')),
  state        text NOT NULL CHECK (state IN ('PENDING','SENT','SUPPRESSED','FAILED')),
  reason       text,                             -- a detail code, never a sentence (rule 8)
  event_id     uuid,                             -- null for the invitation, which is not an event
  item_id      uuid,
  actor_id     uuid,
  created_at   timestamptz NOT NULL DEFAULT now(),
  sent_at      timestamptz,
  attempts     integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  -- The subjects that are not an entry (migration 0096): the rule that was switched
  -- off, the subscription that stopped being called. At most one of the three is set.
  rule_id         uuid,
  subscription_id uuid,
  -- The emergency access a member is told of (migration 0138, ADR-0073 §4): the mail reads the
  -- opener, the time and the reason from it at delivery.
  private_hub_access_id uuid,
  -- The role somebody was given (migrations 0144, 0145, UC-NOT-01 check 9): the mail reads the role,
  -- the scope and the place's name from the grant at delivery, and a revoked grant takes it along.
  membership_id uuid,
  -- The page whose review fell due (migration 0232, UC-DOC-12 check 4): the mail reads its title at
  -- delivery, and a purged page takes it along.
  page_id uuid,

  CONSTRAINT notification_recipient_id_fkey
    FOREIGN KEY (tenant_id, recipient_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_item_id_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_actor_id_fkey
    FOREIGN KEY (tenant_id, actor_id) REFERENCES account (tenant_id, id)
      ON DELETE SET NULL (actor_id),
  CONSTRAINT notification_rule_id_fkey
    FOREIGN KEY (tenant_id, rule_id) REFERENCES automation_rule (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_subscription_id_fkey
    FOREIGN KEY (tenant_id, subscription_id) REFERENCES webhook_subscription (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_private_hub_access_id_fkey
    FOREIGN KEY (tenant_id, private_hub_access_id) REFERENCES private_hub_access (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_one_subject_check
    CHECK (num_nonnulls(item_id, rule_id, subscription_id, private_hub_access_id, membership_id, page_id) <= 1),
  CONSTRAINT notification_membership_id_fkey
    FOREIGN KEY (tenant_id, membership_id) REFERENCES membership (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT notification_page_id_fkey
    FOREIGN KEY (tenant_id, page_id) REFERENCES page (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX notification_page_idx ON notification (tenant_id, page_id) WHERE page_id IS NOT NULL;
-- Partial: the invitation carries no event, and NULLs are all distinct to a unique index.
CREATE UNIQUE INDEX notification_event_recipient_idx
  ON notification (tenant_id, event_id, recipient_id, channel)
  WHERE event_id IS NOT NULL;
CREATE INDEX notification_pending_idx
  ON notification (tenant_id, created_at) WHERE state = 'PENDING';
CREATE INDEX notification_retention_idx ON notification (tenant_id, created_at);
-- What a revocation's cascade finds its records by.
CREATE INDEX notification_membership_idx
  ON notification (tenant_id, membership_id) WHERE membership_id IS NOT NULL;

-- What a person has said about being told. A row is an exception; its absence is the default, on.
CREATE TABLE notification_preference (
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id    uuid NOT NULL,
  category      text NOT NULL CHECK (category IN ('ASSIGNMENT','MEMBERSHIP','COMMENT','INVITATION','REMINDER','INTEGRATION','RETENTION','EMERGENCY_ACCESS')),
  channel       text NOT NULL CHECK (channel IN ('EMAIL')),
  enabled       boolean NOT NULL DEFAULT true,
  include_title boolean NOT NULL DEFAULT true,   -- data-protection.md §9: the minimum is switchable
  updated_at    timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (tenant_id, account_id, category, channel),
  CONSTRAINT notification_preference_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- ========================= Events, jobs, idempotency =======================

-- Partitioned by month, activity_entry's construction: history + parent + default,
-- one shape for the schema reference and the migrations.
CREATE TABLE outbox_event_history (
  id              uuid NOT NULL,
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
  replay          boolean NOT NULL DEFAULT false,
  -- The two clocks of an event a device brought in, and the push it arrived with (0086).
  received_at     timestamptz,
  push_id         uuid
);
CREATE INDEX outbox_pending_idx ON outbox_event_history (occurred_at)
  WHERE dispatched_at IS NULL;
CREATE INDEX outbox_poll_idx ON outbox_event_history (tenant_id, event_type, occurred_at, id)
  WHERE replay = false;
CREATE UNIQUE INDEX outbox_event_part_pkey ON outbox_event_history (tenant_id, occurred_at, id);
CREATE UNIQUE INDEX outbox_event_history_id_idx ON outbox_event_history (id);

CREATE TABLE outbox_event (
  id              uuid NOT NULL,
  tenant_id       uuid NOT NULL,
  event_type      text NOT NULL,
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
  replay          boolean NOT NULL DEFAULT false,
  received_at     timestamptz,
  push_id         uuid,
  PRIMARY KEY (tenant_id, occurred_at, id)
) PARTITION BY RANGE (occurred_at);
CREATE INDEX outbox_event_pending_idx ON outbox_event (occurred_at)
  WHERE dispatched_at IS NULL;
-- The polling trigger's walk (migration 0052): one type, in the outbox's own order. The
-- tenant leads because row level security puts it in front of every predicate; the ordering pair
-- comes last so a page is a range read rather than a sort. Partial, because a poll never answers a
-- replayed event.
CREATE INDEX outbox_event_poll_idx ON outbox_event (tenant_id, event_type, occurred_at, id)
  WHERE replay = false;

DO $outbox_attach$
BEGIN
  EXECUTE format(
    'ALTER TABLE outbox_event ATTACH PARTITION outbox_event_history FOR VALUES FROM (MINVALUE) TO (%L)',
    (date_trunc('month', now()) + interval '1 month')::date);
END $outbox_attach$;
CREATE TABLE outbox_event_default PARTITION OF outbox_event DEFAULT;

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
  progress     real,
  -- The parameters of the refusal a job ended with, beside its code (migration 0151).
  last_error_params jsonb
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
  -- The day's closing count for the counts kept as the day's highest (`value`): entries, stored
  -- bytes, people (migration 0183). NULL for the sums.
  closing    bigint,
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
-- The one exception (migration 0067): a row may fall only while the transaction-scoped
-- purge marker names exactly its tenant - and the only writer of that marker is
-- purge_tenant_trail, which closes the window before it returns. UPDATE stays impossible
-- unconditionally.
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE'
     AND current_setting('hubtask.trail_purge', true) = OLD.tenant_id::text THEN
    RETURN OLD;
  END IF;
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
  notes          text,
  -- An extension (Art. 12(3), migration 0119): due_at takes the extended deadline, these keep the
  -- original, the reason and the day the person was informed - all three or none.
  original_due_at  timestamptz,
  extension_reason text CHECK (extension_reason IN ('COMPLEXITY','NUMBER_OF_REQUESTS')),
  informed_on      date,
  CONSTRAINT dsr_extension_whole_check
    CHECK (num_nulls(original_due_at, extension_reason, informed_on) IN (0, 3))
);
CREATE INDEX dsr_open_idx ON data_subject_request (tenant_id, status, due_at)
  WHERE status IN ('RECEIVED','IN_PROGRESS');

-- What an erasure kept, per legal hold (migration 0120, data-protection.md §4.1): counts, never a
-- name or content. `hold_id` references no row, so a restore that rewrites `legal_hold` leaves the
-- record of what a case kept in place.
CREATE UNIQUE INDEX dsr_tenant_id_uq ON data_subject_request (tenant_id, id);
CREATE TABLE erasure_kept (
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  request_id     uuid NOT NULL,
  hold_id        uuid NOT NULL,
  hold_scope     text NOT NULL CHECK (hold_scope IN ('TENANT','CONTAINER','ITEM','ACCOUNT')),
  hold_scope_id  uuid,
  account        boolean NOT NULL DEFAULT false,
  entries        integer NOT NULL DEFAULT 0 CHECK (entries >= 0),
  comments       integer NOT NULL DEFAULT 0 CHECK (comments >= 0),
  assignments    integer NOT NULL DEFAULT 0 CHECK (assignments >= 0),
  intake         integer NOT NULL DEFAULT 0 CHECK (intake >= 0),
  recorded_at    timestamptz NOT NULL,
  erased_at      timestamptz,
  blocked_code   text,
  blocked_params jsonb,
  PRIMARY KEY (tenant_id, request_id, hold_id),
  CONSTRAINT erasure_kept_request_id_fkey FOREIGN KEY (tenant_id, request_id)
    REFERENCES data_subject_request (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX erasure_kept_pending_idx ON erasure_kept (tenant_id, hold_id)
  WHERE erased_at IS NULL;

-- The pseudonyms an erasure leaves behind for the audit trail. The trail cannot be edited in place
-- - the grants, the trigger and the hash chain all refuse it - so the substitution happens at the
-- boundary and this is what the boundary reads (audit.md §6).
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

-- A person told once that their objection to AI stopped taking effect (migration 0137): the
-- notice is pending while noticed_at is older than the workspace's position.
CREATE TABLE ai_objection_notice (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  account_id uuid NOT NULL,
  noticed_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, account_id),
  CONSTRAINT ai_objection_notice_account_id_fkey
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
  -- Whether a FULL run is followed by an INSPECT restore of its own archive (B-4, migration
  -- 0091). On for a new schedule; the migration left the schedules that existed off.
  trial_restore boolean NOT NULL DEFAULT true,
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
  verify_ok     boolean,
  -- What the trial restore found, and when (B-4, migration 0091): the INSPECT report of the
  -- archive this run wrote, read back in the same job.
  trial_report  jsonb,
  trial_at      timestamptz
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
  condition       text,                              -- CEL, stored and not evaluated
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
  -- Why a pass set the rule aside: its condition could not be evaluated on a candidate. The code
  -- and the moment, until a pass evaluates it cleanly (migration 0170, data-retention.md §2).
  last_failure_code text,
  last_failure_at   timestamptz,
  CONSTRAINT retention_rule_scope_check CHECK ((scope_kind = 'TENANT') = (scope_id IS NULL)),
  CONSTRAINT retention_rule_chain_check CHECK ((then_after_days IS NULL) = (then_action IS NULL)),
  CONSTRAINT retention_rule_last_failure_whole
    CHECK ((last_failure_code IS NULL) = (last_failure_at IS NULL)),
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

-- The key a tenant-scoped reference to a rule points at (ADR-0024, migration 0172).
CREATE UNIQUE INDEX retention_rule_tenant_id_uq ON retention_rule (tenant_id, id);

-- Where each read of the rule sweep stopped, so the next pass continues there (data-retention.md §5,
-- migration 0173): phase one of a kind, a chain's second stage by its rule, phase two. A position
-- in the candidates' order and nothing else (rule 10); not in an archive.
CREATE TABLE retention_cursor (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  reader     text NOT NULL CHECK (reader IN ('ANNOUNCE','CHAIN','ACT')),
  data_kind  text,
  rule_id    uuid,
  after_at   timestamptz NOT NULL,
  after_id   uuid NOT NULL,
  CONSTRAINT retention_cursor_reader_shape CHECK (
    (reader = 'ANNOUNCE' AND data_kind IS NOT NULL AND rule_id IS NULL)
    OR (reader = 'CHAIN' AND data_kind IS NULL AND rule_id IS NOT NULL)
    OR (reader = 'ACT' AND data_kind IS NULL AND rule_id IS NULL)
  ),
  -- On the workspace and the rule together, so no row can name another workspace's rule (ADR-0024;
  -- the key is 0172's index).
  CONSTRAINT retention_cursor_rule_fkey FOREIGN KEY (tenant_id, rule_id)
    REFERENCES retention_rule (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX retention_cursor_reader_uq
  ON retention_cursor (tenant_id, reader, data_kind, rule_id) NULLS NOT DISTINCT;
CREATE INDEX retention_cursor_rule_idx ON retention_cursor (tenant_id, rule_id) WHERE rule_id IS NOT NULL;

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

-- The wake-up for the change stream (ADR-0007). A trigger rather than a NOTIFY in the
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

-- A live document's update wakes the connections on every process (migration 0230).
CREATE TRIGGER realtime_update_notify
  AFTER INSERT ON realtime_update
  FOR EACH ROW EXECUTE FUNCTION hubtask_notify_change();

-- The wake-up for the dispatcher (ADR-0007). The queue rather than outbox_event: an event is
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

-- The server's clock per field (migration 0083, offline-sync.md §4.2, §10): the reading of
-- the write that landed, which a push's reading is compared against per field. Not backfilled -
-- a field written before the migration has no row and loses to the first device that writes it.
-- Keyed by entity rather than tied to one table, because the rule applies to more than entries;
-- an entry's rows go when the entry is purged (the purge removes them beside the entry).
CREATE TABLE field_clock (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entity     text NOT NULL,
  entity_id  uuid NOT NULL,
  field      text NOT NULL,
  hlc        text NOT NULL,                        -- physical:counter:device, sorts as a clock
  PRIMARY KEY (tenant_id, entity, entity_id, field)
);

-- An import from another system (backup-restore.md §9): what was asked, where it stands,
-- and the report the applier wrote in the restore's shape. Not a restore_run, which names a
-- target and an archive at it; an import's file is a media object deleted when the job ends.
CREATE TABLE import_run (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  requested_by  uuid NOT NULL,
  kind          text NOT NULL CHECK (kind IN ('CSV','TRELLO','GOOGLE_TASKS','MICROSOFT_TODO')),
  media_id      uuid NOT NULL,
  hub_id        uuid NOT NULL,
  mapping       jsonb,
  -- The requester's zone and language at the request, for a source that writes dates without a
  -- zone and entries without a language: what the rows are read in.
  time_zone     text,
  language      text,
  status        text NOT NULL DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING','RUNNING','SUCCEEDED','FAILED')),
  report        jsonb,
  refused       jsonb,
  progress      jsonb,
  error_code    text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  started_at    timestamptz,
  finished_at   timestamptz
);
CREATE INDEX import_run_tenant_idx ON import_run (tenant_id, created_at DESC);

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
  -- The credential of the request that last touched the device (migration 0082): what
  -- forgetting the device revokes. A token's identifier matches no session and revokes nothing.
  credential_id uuid,
  CONSTRAINT sync_device_account_id_fkey
    FOREIGN KEY (tenant_id, account_id) REFERENCES account (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX sync_device_account_idx ON sync_device (tenant_id, account_id);
CREATE INDEX sync_device_last_seen_idx ON sync_device (tenant_id, last_seen_at);

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

-- What AI proposed, and did not do (ADR-0012). A suggestion is a record: it becomes a change
-- when somebody accepts it, as their own write with their own rights. The provenance columns are
-- why the table exists rather than the answer being applied and forgotten - "why does this task say
-- that" has to be answerable a year later.
CREATE TABLE ai_suggestion (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  -- No foreign key: the two target kinds live in two tables, and a polymorphic reference cannot
  -- be one. The application checks the target, which it must do anyway to authorise the read.
  target_type   text NOT NULL CHECK (target_type IN ('WORK_ITEM', 'JUMBLE_ENTRY', 'CONTAINER', 'ACCOUNT')),
  target_id     uuid NOT NULL,
  kind          text NOT NULL CHECK (kind IN ('FIELDS', 'DECOMPOSITION', 'DUPLICATES', 'TEMPLATE', 'RULE', 'ANSWER', 'ENTRIES')),
  status        text NOT NULL DEFAULT 'PROPOSED'
                  CHECK (status IN ('PROPOSED', 'ACCEPTED', 'DISMISSED')),
  payload       jsonb NOT NULL,
  source        text NOT NULL DEFAULT 'AI' CHECK (source IN ('AI')),
  model         text NOT NULL CHECK (length(model) BETWEEN 1 AND 200),
  prompt_id     text NOT NULL CHECK (length(prompt_id) BETWEEN 0 AND 200),
  prompt_version text NOT NULL CHECK (length(prompt_version) BETWEEN 0 AND 50),
  -- When the provider answered, which is not when this row was written.
  produced_at   timestamptz NOT NULL,
  -- What the suggestion was made from. Acceptance compares it and refuses a stale one.
  input_digest  bytea NOT NULL,
  created_at    timestamptz NOT NULL,
  decided_at    timestamptz,
  decided_by    uuid,
  version       integer NOT NULL DEFAULT 1,
  -- How many parts of the provider's answer the narrowing dropped before the payload was stored:
  -- for TEMPLATE the nodes the profile refused, each with its subtree (migration 0094).
  dropped_nodes integer NOT NULL DEFAULT 0 CHECK (dropped_nodes >= 0),
  CONSTRAINT ai_suggestion_decision CHECK (
    (status = 'PROPOSED' AND decided_at IS NULL AND decided_by IS NULL) OR
    (status <> 'PROPOSED' AND decided_at IS NOT NULL)
  )
);
CREATE INDEX ai_suggestion_target_idx
  ON ai_suggestion (tenant_id, target_type, target_id, status, created_at DESC, id DESC);
CREATE INDEX ai_suggestion_age_idx ON ai_suggestion (tenant_id, created_at);

-- The words a person asked a template to be drafted from (migration 0089). Held under row
-- level security for the minutes between the asking and the answer, because every other question
-- reads its material from a row the workspace already holds and the queue's payloads carry
-- identifiers only; the job that reads it deletes it when it ends.
CREATE TABLE ai_request (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  asked_by    uuid NOT NULL,
  text        text NOT NULL CHECK (length(text) BETWEEN 1 AND 2000),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ai_request_age_idx ON ai_request (tenant_id, created_at);

-- Semantic search's store (ADR-0050), where the database can carry it.
--
-- **Conditional.** Migration 0075 creates this table only where `pg_available_extensions` offers
-- `vector`; an installation without pgvector has no such table and a search that is lexical,
-- complete, and honest about itself in `/meta/capabilities`. It is mirrored here because every
-- gate that starts a PostgreSQL runs one that has the extension, which is what lets
-- `support-matrix.md` call semantic search supported at all.
--
-- Its own table rather than a column of `work_item`: a conditional column would give one table two
-- shapes, and a vector has no business in the row every write of an entry touches. The index is
-- built in the same migration and not CONCURRENTLY, because the table it indexes was created empty
-- one statement earlier - the lock is over a table no pod has ever read.
CREATE TABLE item_embedding (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id    uuid NOT NULL,
  -- Vectors are only comparable within one model's space, so a row that does not name its model is
  -- one nothing can decide about after a reconfiguration.
  -- An offered model's vectors are named `offered:<id>/<model>` (migration 0129), hence 300.
  model      text NOT NULL CHECK (length(model) BETWEEN 1 AND 300),
  embedding  vector(1536) NOT NULL,
  source_digest bytea NOT NULL,
  updated_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, item_id),
  FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX item_embedding_vector_idx ON item_embedding USING hnsw (embedding vector_cosine_ops);

-- The fingerprint of an entry's text, computed where the entries are (migration 0076).
--
-- The same function as `core/domain/model/suggestion.Digest` - the parts joined by a byte no UTF-8
-- text contains, then SHA-256 - because "the same text" has to mean the same thing to whatever
-- computes a digest and whatever compares one. Two implementations of one rule is how a staleness
-- check comes to pass for a suggestion made from something else, and an integration test asserts
-- the two agree rather than trusting this comment.
--
-- It exists at all because the embedding pass has to ask "whose text has moved" over a whole
-- workspace: in Go that is every entry's title and notes crossing the wire to be thrown away.
CREATE OR REPLACE FUNCTION digest_of(VARIADIC parts text[])
  RETURNS bytea LANGUAGE sql IMMUTABLE PARALLEL SAFE AS
$$
  SELECT sha256(
    coalesce(
      (SELECT string_agg(convert_to(coalesce(part, ''), 'UTF8') || '\xff'::bytea, ''::bytea
               ORDER BY ordinality)
         FROM unnest(parts) WITH ORDINALITY AS t(part, ordinality)),
      ''::bytea))
$$;

-- ============ A limit reached, once per crossing ====================
-- The mark that makes `limit_reached` and `ai_budget_spent` told once per crossing (UC-INS-14
-- check 2): one per quota at one period - the limit's value, or the AI budget's day. See
-- db/migrations/0181_quota_reached.sql.
CREATE TABLE quota_reached (
  tenant_id  uuid NOT NULL REFERENCES tenant (id) ON DELETE CASCADE,
  quota      text NOT NULL CHECK (quota ~ '^[a-z][a-z0-9_]*$'),
  period     text NOT NULL DEFAULT '',
  reached_at timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, quota, period)
);
GRANT SELECT, INSERT, DELETE ON quota_reached TO hubtask_app;

-- ============================ Row Level Security ===========================
-- For every tenant-scoped table: a policy on current_tenant_id().
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'account','account_group','account_group_member','membership','access_token',
    'session','session_refresh_token','auth_attempt',
    'account_mfa','account_recovery_code','account_password_history','webauthn_credential',
    'auth_pending',
    'oauth_client','oauth_grant','oauth_code',
    'account_identity','tenant_host','oidc_flow','ai_source','ai_suggestion','ai_request',
    'item_embedding',
    'container','bucket','label','work_item','item_label','item_member',
    'custom_field_definition','comment','container_milestone','kudos','structure_preset',
    'activity_entry',
    'media_object','item_attachment',
    'recurrence_rule','reminder','saved_view','template','jumble_entry','auto_assign_policy',
    'automation_rule','rule_run','rule_occurrence','jumble_intake','mail_setting','mail_receipt','mailbox','mailbox_authorization','voice_setting','voice_alias',
    'webhook_subscription','webhook_delivery','calendar_feed',
    'outbox_event','event_consumption','idempotency_key','usage_record',
    'notification','notification_preference','private_hub_access',
    'audit_anchor','audit_pseudonym','retention_policy','data_subject_request','consent_record',
    'ai_objection_notice',
    'erasure_kept',
    'backup_schedule','backup_run','restore_run','deletion_journal','retention_run',
    'retention_rule','retention_cursor',
    'legal_hold','tombstone','sync_device','sync_op_log','set_element','field_clock',
    'import_run','quota_reached','setting_value','entry_type','entry_field_value',
    'link_type','entry_link','entry_reference','workflow','workflow_scheme',
    'custom_role','custom_role_grant',
    'config_revision','config_proposal','config_template','config_template_use',
    'config_sandbox',
    'work_item_announcement','work_item_flow','work_item_retention','durable_timer',
    'realtime_document','realtime_update','realtime_snapshot',
    'page','page_version','page_comment','page_reference'
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

-- identity_provider is read by every workspace and written by one level only (migration 0103).
-- A NULL row is the installation's: every workspace has to be able to draw its button, and
-- no workspace may change it. The standard policy would make such a row invisible to everybody,
-- so the read admits it and the write does not - and the third policy is the installation's own
-- scope, where `app.tenant_id` is the empty string and `current_tenant_id()` is therefore NULL.
ALTER TABLE identity_provider ENABLE ROW LEVEL SECURITY;
ALTER TABLE identity_provider FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON identity_provider FOR SELECT
  USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);
CREATE POLICY tenant_write ON identity_provider FOR ALL
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY installation_write ON identity_provider FOR ALL
  USING (tenant_id IS NULL AND current_tenant_id() IS NULL)
  WITH CHECK (tenant_id IS NULL AND current_tenant_id() IS NULL);

-- ai_provider follows identity_provider (migration 0129): a NULL row is a model the installation
-- offers. Every workspace that chose one reads it for the call; only the installation writes it.
ALTER TABLE ai_provider ENABLE ROW LEVEL SECURITY;
ALTER TABLE ai_provider FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_read ON ai_provider FOR SELECT
  USING (tenant_id = current_tenant_id() OR tenant_id IS NULL);
CREATE POLICY tenant_write ON ai_provider FOR ALL
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY installation_write ON ai_provider FOR ALL
  USING (tenant_id IS NULL AND current_tenant_id() IS NULL)
  WITH CHECK (tenant_id IS NULL AND current_tenant_id() IS NULL);

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
-- the instance administration - not to tenant users: a session with no tenant set reaches those
-- rows and only those (migration 0157).
ALTER TABLE backup_target ENABLE ROW LEVEL SECURITY;
ALTER TABLE backup_target FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON backup_target
  USING (tenant_id = current_tenant_id())
  WITH CHECK (tenant_id = current_tenant_id());
CREATE POLICY installation_targets ON backup_target FOR ALL
  USING (tenant_id IS NULL AND current_tenant_id() IS NULL)
  WITH CHECK (tenant_id IS NULL AND current_tenant_id() IS NULL);

-- item_capability_profile: the system defaults (tenant_id IS NULL) are readable by everyone,
-- overrides only for the tenant concerned. Never write to the system defaults.
-- Deliberately ENABLE without FORCE, unlike every other tenant table (ADR-0052): FORCE binds the
-- owner too, and here the owner is the only actor that may write the system defaults. The
-- application role is not the owner, so the policy below applies to it in full.
ALTER TABLE item_capability_profile ENABLE ROW LEVEL SECURITY;
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
    WHERE parent.relname IN ('audit_log', 'change_log',
                             'activity_entry', 'outbox_event', 'rule_run')
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
-- erasure that could be undone (audit.md §6).
REVOKE UPDATE, DELETE, TRUNCATE ON audit_pseudonym FROM hubtask_app;
GRANT  SELECT, INSERT ON audit_pseudonym TO hubtask_app;

-- And the anchors (migration 0090, audit.md §3): the chain's end exported outside the database is the one
-- thing that says anything against somebody who can rewrite the trail, and the row that says where
-- it went must not be one the application can rewrite or remove.
REVOKE UPDATE, DELETE, TRUNCATE ON audit_anchor FROM hubtask_app;
GRANT  SELECT, INSERT ON audit_anchor TO hubtask_app;

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

-- ============== The partition duty (audit.md §3) ======================
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

-- ============ The one cross-tenant question (data-protection.md §4) ===
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

-- ============ Who operates this installation (ADR-0070 §1) ==================
-- Checked when `admin:tenants` is minted and when it is exercised - either alone is a hole. An
-- empty register is the private installation: nothing configured, and the owner is the operator
-- exactly as they were before this table existed.
--
-- No row-level policy, and no grant to the application role either. The rows name accounts, so
-- unlike instance_setting this table *is* a person's data, and a policy-free table hubtask_app
-- could read would let every workspace enumerate the installation's operators. It is reachable only
-- through the four functions below - resolve_tenant's discipline applied to a table. See
-- db/migrations/0101_operator_register.sql for the whole reasoning.
CREATE TABLE operator (
  tenant_id  uuid NOT NULL,
  account_id uuid NOT NULL,
  added_at   timestamptz NOT NULL DEFAULT now(),
  added_by   uuid,
  PRIMARY KEY (tenant_id, account_id),
  CONSTRAINT operator_account_fkey FOREIGN KEY (tenant_id, account_id)
    REFERENCES account (tenant_id, id) ON DELETE CASCADE
);

-- ============ The instance's own settings (ADR-0070 §2) =====================
-- A value that applies to every workspace, and that a workspace may not change. Deliberately
-- without a row-level-security policy (the job table's precedent, and instance_event's below):
-- every workspace has to be able to read it, because the effective sign-in rule is resolved on
-- every password screen, and the standard policy comparing against current_tenant_id() would make
-- a tenant-less row invisible to everybody - which is exactly what it does to backup_target's
-- instance-wide rows today.
--
-- What makes that acceptable is the content: installation configuration, never a person's data.
-- The bound on writing it is the control plane's use case, behind `admin:tenants` and the operator
-- register; the grant has to allow the write for that use case to make it. See
-- db/migrations/0099_instance_setting.sql for the whole reasoning.
CREATE TABLE instance_setting (
  key         text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
  value       jsonb NOT NULL,
  lock_origin text NOT NULL DEFAULT 'OPEN' CHECK (lock_origin IN ('OPEN', 'INSTANCE', 'PLAN')),
  updated_by  uuid,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

-- Plans, between the installation and its workspaces (multi-tenancy.md §4.2, ADR-0086,
-- migration 0164). No row level security, for instance_setting's reason: every workspace reads
-- its plan, and the tables hold configuration. A removed plan keeps its row, and the application
-- role holds no DELETE on `plan`, so a short name is never reused.
CREATE TABLE plan (
  id           uuid PRIMARY KEY,
  name         text NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9-]{1,39}$'),
  display_name text NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 200),
  version      integer NOT NULL DEFAULT 1,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  updated_by   uuid,
  removed_at   timestamptz,
  CONSTRAINT plan_name_key UNIQUE (name)
);

-- What a plan decides, in the installation level's key shape, and `entitlement.<feature>` for a
-- feature it leaves out.
CREATE TABLE plan_setting (
  plan_id uuid NOT NULL REFERENCES plan (id) ON DELETE RESTRICT,
  key     text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$'),
  value   jsonb NOT NULL,
  locked  boolean NOT NULL DEFAULT false,
  PRIMARY KEY (plan_id, key)
);

ALTER TABLE tenant ADD CONSTRAINT tenant_plan_fk
  FOREIGN KEY (plan_id) REFERENCES plan (id) ON DELETE RESTRICT;

-- The installation's presets (UC-INS-18, migration 0198): offered to every workspace, one of them
-- the default a hub starts from where its workspace chose none, and only the default locks. No row
-- level security, for plan's reason: every workspace reads them, and the table holds configuration.
CREATE TABLE installation_preset (
  id         uuid PRIMARY KEY,
  name       text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
  settings   jsonb NOT NULL,
  is_default boolean NOT NULL DEFAULT false,
  locks      text[] NOT NULL DEFAULT '{}',
  version    integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  updated_by uuid,
  CONSTRAINT installation_preset_name_key UNIQUE (name),
  CONSTRAINT installation_preset_locks_check CHECK (is_default OR cardinality(locks) = 0)
);
CREATE UNIQUE INDEX installation_preset_default_idx
  ON installation_preset (is_default) WHERE is_default;

-- A platform's reference names one workspace (multi-tenancy.md §4.3, 0166).
CREATE UNIQUE INDEX tenant_external_reference_key
  ON tenant (external_reference) WHERE external_reference IS NOT NULL;

-- The installation at a glance (migrations 0105, 0128). Counts, states and limits - never rows,
-- which is ADR-0070 §5 in its own words. SECURITY DEFINER for `is_operator`'s reason: `account` is
-- behind row level security and FORCE, so the application role cannot count across workspaces at
-- all, and narrow by construction is what makes the exception acceptable - six integers, and no way
-- to ask for anybody's data. A workspace that holds no person - only a service account - is not a
-- customer: the status counts leave it out, and the sixth number says how many there are.
CREATE OR REPLACE FUNCTION instance_census()
RETURNS TABLE (
  workspaces_active           bigint,
  workspaces_suspended        bigint,
  workspaces_pending_deletion bigint,
  accounts_active             bigint,
  accounts_total              bigint,
  workspaces_service_only     bigint
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  WITH customer AS (
    SELECT t.status
    FROM tenant t
    WHERE EXISTS (
      SELECT 1 FROM account a
      WHERE a.tenant_id = t.id AND a.kind = 'USER' AND a.deleted_at IS NULL
    )
  )
  SELECT
    (SELECT count(*) FROM customer WHERE status = 'ACTIVE'),
    (SELECT count(*) FROM customer WHERE status = 'SUSPENDED'),
    (SELECT count(*) FROM customer WHERE status = 'PENDING_DELETION'),
    (SELECT count(*) FROM account WHERE deleted_at IS NULL AND status = 'ACTIVE'),
    (SELECT count(*) FROM account WHERE deleted_at IS NULL),
    (SELECT count(*) FROM tenant) - (SELECT count(*) FROM customer)
$$;
REVOKE ALL ON FUNCTION instance_census() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION instance_census() TO hubtask_app;

-- Which migrations the database has applied, for the readiness probe (deployment.md §5; migration
-- 0150): the ledger is closed to the application role, so it reads the numbers through this, and
-- nothing else. plpgsql, because this file is loaded without goose, where the ledger is absent and
-- the function answers NULL.
CREATE OR REPLACE FUNCTION applied_schema_versions()
RETURNS bigint[]
LANGUAGE plpgsql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
BEGIN
  RETURN (
    SELECT coalesce(array_agg(DISTINCT version_id ORDER BY version_id), ARRAY[]::bigint[])
    FROM public.goose_db_version
    WHERE is_applied
  );
EXCEPTION WHEN undefined_table THEN
  RETURN NULL;
END;
$$;
REVOKE ALL ON FUNCTION applied_schema_versions() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION applied_schema_versions() TO hubtask_app;

-- ============ The instance's own journal ============================
-- Evidence of acts whose per-tenant trail cannot hold them - above all a hard delete, after
-- which the tenant's own audit chain is gone by design. Identifiers, a slug, counts and
-- moments; never content. Deliberately without a row-level-security policy (the job table's
-- precedent): the rows belong to the installation, and a policy comparing against
-- current_tenant_id() would make them unreachable under every honest scope. See
-- db/migrations/0067_tenant_lifecycle.sql for the whole reasoning.
CREATE TABLE instance_event (
  id          uuid PRIMARY KEY,
  occurred_at timestamptz NOT NULL,
  action      text NOT NULL,
  -- A bare identifier, no foreign key: the row it named is usually gone, which is the reason
  -- this table exists.
  tenant_id   uuid,
  tenant_slug text,
  actor_label text,
  details     jsonb NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX instance_event_occurred_idx ON instance_event (occurred_at);

-- Append-only for the application, the audit trail's discipline: evidence that could be edited
-- or removed afterwards would not be evidence.
REVOKE UPDATE, DELETE, TRUNCATE ON instance_event FROM hubtask_app;
GRANT SELECT, INSERT ON instance_event TO hubtask_app;

-- ============ The installation's events =============================
-- What a provider's purchase platform learns about the workspaces (UC-INS-14, multi-tenancy.md
-- §4.3). Numbered in commit order: `number` is drawn while the writer holds the advisory lock
-- 8_010_002 until its transaction ends. No row-level-security policy, the journal's
-- reasoning; `tenant_id` is a bare identifier, since the `deleted` event outlives its workspace. See
-- db/migrations/0180_platform_event.sql.
CREATE SEQUENCE platform_event_number AS bigint;
CREATE TABLE platform_event (
  number      bigint PRIMARY KEY,
  id          uuid NOT NULL,
  event_type  text NOT NULL CHECK (event_type ~ '^de\.hubtask\.installation\.[a-z_.]+\.v[0-9]+$'),
  tenant_id   uuid NOT NULL,
  occurred_at timestamptz NOT NULL,
  actor_type  text NOT NULL,
  actor_id    uuid,
  payload     jsonb NOT NULL,
  dedupe_key  text,
  CONSTRAINT platform_event_id_key UNIQUE (id),
  CONSTRAINT platform_event_dedupe_key UNIQUE (dedupe_key)
);
CREATE INDEX platform_event_tenant_idx ON platform_event (tenant_id);
CREATE INDEX platform_event_occurred_idx ON platform_event (occurred_at);
GRANT SELECT, INSERT, DELETE ON platform_event TO hubtask_app;
GRANT UPDATE (payload) ON platform_event TO hubtask_app;
GRANT USAGE ON SEQUENCE platform_event_number TO hubtask_app;

-- The installation's subscriptions for a purchase platform and their deliveries, one row per event
-- and subscription across its attempts; never switched off for being unreachable. No tenant column:
-- they are the installation's. See db/migrations/0182_platform_subscription.sql.
CREATE TABLE platform_subscription (
  id               uuid PRIMARY KEY,
  target_url       text NOT NULL CHECK (char_length(target_url) BETWEEN 1 AND 2000),
  secret_enc       bytea NOT NULL,
  secret_key_id    text NOT NULL,
  state            text NOT NULL DEFAULT 'ACTIVE' CHECK (state IN ('ACTIVE', 'PAUSED')),
  last_error       text,
  last_delivery_at timestamptz,
  from_file        boolean NOT NULL DEFAULT false,
  created_at       timestamptz NOT NULL,
  created_by       uuid,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1
);
CREATE TABLE platform_delivery (
  id              uuid PRIMARY KEY,
  subscription_id uuid NOT NULL REFERENCES platform_subscription (id) ON DELETE CASCADE,
  event_id        uuid NOT NULL,
  event_type      text NOT NULL,
  attempt         integer NOT NULL DEFAULT 0,
  status          text NOT NULL DEFAULT 'PENDING'
    CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED', 'DEAD_LETTER')),
  response_status integer,
  error_code      text,
  next_attempt_at timestamptz,
  created_at      timestamptz NOT NULL,
  updated_at      timestamptz NOT NULL,
  CONSTRAINT platform_delivery_event_key UNIQUE (subscription_id, event_id)
);
CREATE INDEX platform_delivery_status_idx ON platform_delivery (subscription_id, status);
CREATE INDEX platform_delivery_updated_idx ON platform_delivery (updated_at);
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_subscription TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_delivery TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON instance_setting TO hubtask_app;
GRANT SELECT, INSERT, UPDATE ON plan TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON plan_setting TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON installation_preset TO hubtask_app;

-- The operator register's four doors (migration 0101). SECURITY DEFINER for resolve_tenant's
-- reason: reading a table that belongs to no tenant is the owner's right and the application role
-- does not hold it. Narrow by construction - a boolean, a listing the control plane alone reaches,
-- and two writes that keep the register's one invariant in the statement rather than in a read.
CREATE OR REPLACE FUNCTION is_operator(p_account uuid) RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT
    EXISTS (SELECT 1 FROM operator WHERE account_id = p_account)
    -- An empty register is the private installation, and it means the workspace's OWNER role
    -- holders rather than everybody (migration 0106): the dashboard draws a way in for whoever may
    -- reach it, so a guest who could raise their session would see and use the control plane.
    OR (
      NOT EXISTS (SELECT 1 FROM operator)
      AND (SELECT count(*) FROM tenant) = 1
      AND EXISTS (
        SELECT 1
        FROM membership m
        JOIN account a ON a.tenant_id = m.tenant_id AND a.id = m.account_id
        WHERE m.account_id = p_account
          AND m.role = 'OWNER'
          AND m.scope_type = 'TENANT'
          AND a.deleted_at IS NULL
          AND a.status = 'ACTIVE'
      )
    )
$$;

CREATE OR REPLACE FUNCTION operator_register() RETURNS SETOF operator
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT o.* FROM operator o ORDER BY o.added_at, o.account_id
$$;

-- An operator is named by address and workspace, because an account id is not something anybody
-- can look up: `account` is behind row level security, so the control plane cannot list accounts
-- across workspaces (migration 0109). One pair in, one identifier or nothing out.
CREATE OR REPLACE FUNCTION resolve_operator_account(p_slug text, p_email text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT a.id
  FROM account a
  JOIN tenant t ON t.id = a.tenant_id
  WHERE lower(t.slug) = lower(btrim(p_slug))
    AND lower(a.email) = lower(btrim(p_email))
    AND a.deleted_at IS NULL
  LIMIT 1
$$;

CREATE OR REPLACE FUNCTION add_operator(p_account uuid, p_by uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE added integer;
BEGIN
  -- The workspace comes from the account rather than from the caller: a pair that could disagree
  -- would be a pair somebody eventually gets wrong, and this function can read what the application
  -- role cannot. An account nobody holds inserts nothing, which the caller reads as "no such
  -- account".
  INSERT INTO operator (tenant_id, account_id, added_by)
  SELECT a.tenant_id, a.id, p_by
  FROM account a
  WHERE a.id = p_account AND a.deleted_at IS NULL
  ON CONFLICT (tenant_id, account_id) DO NOTHING;
  GET DIAGNOSTICS added = ROW_COUNT;
  RETURN added > 0;
END $$;

-- The last operator cannot remove themselves. In the statement, because two operators removing
-- each other at the same moment would both read "there are two".
CREATE OR REPLACE FUNCTION drop_operator(p_account uuid) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE removed boolean;
BEGIN
  -- The account alone: an identifier is unique across the installation, and a caller that had to
  -- name the workspace too would have to read the register first to find out which one it is.
  DELETE FROM operator
  WHERE account_id = p_account
    AND (SELECT count(*) FROM operator) > 1;
  GET DIAGNOSTICS removed = ROW_COUNT;
  RETURN removed;
END $$;

REVOKE ALL ON FUNCTION is_operator(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION operator_register() FROM PUBLIC;
REVOKE ALL ON FUNCTION add_operator(uuid, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION drop_operator(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION is_operator(uuid) TO hubtask_app;
GRANT EXECUTE ON FUNCTION operator_register() TO hubtask_app;
GRANT EXECUTE ON FUNCTION add_operator(uuid, uuid) TO hubtask_app;
GRANT EXECUTE ON FUNCTION drop_operator(uuid) TO hubtask_app;


-- The register a person can read and safely change (migration 0135, UC-INS-05): a roster with names
-- that answers nothing to a caller the register does not name, an addition that cannot lock the adder
-- out and admits only an active account, and a removal that tells "not there" from "the last one".
CREATE OR REPLACE FUNCTION operator_roster(p_caller uuid)
RETURNS TABLE (
  tenant_id      uuid,
  account_id     uuid,
  added_at       timestamptz,
  added_by       uuid,
  display_name   text,
  email          text,
  sign_in_name   text,
  kind           text,
  workspace_slug text,
  workspace_name text,
  added_by_name  text
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT o.tenant_id, o.account_id, o.added_at, o.added_by,
         a.display_name, a.email, a.sign_in_name, a.kind::text,
         t.slug, t.display_name,
         b.display_name
  FROM operator o
  JOIN account a ON a.tenant_id = o.tenant_id AND a.id = o.account_id
  JOIN tenant t ON t.id = o.tenant_id
  LEFT JOIN account b ON b.id = o.added_by AND b.deleted_at IS NULL
  WHERE is_operator(p_caller)
  ORDER BY o.added_at, o.account_id
$$;

CREATE OR REPLACE FUNCTION enlist_operator(p_account uuid, p_by uuid) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE
  target_status account_status;
  was_empty     boolean;
  added         integer;
  caller_added  integer := 0;
BEGIN
  -- Conflicts with itself and with every insert and delete, so "the register is empty" is read and
  -- written under one lock: two first additions at once leave one empty-register branch, not two.
  LOCK TABLE operator IN SHARE ROW EXCLUSIVE MODE;

  SELECT a.status INTO target_status
  FROM account a WHERE a.id = p_account AND a.deleted_at IS NULL;
  IF NOT FOUND THEN
    RETURN 'UNKNOWN';
  END IF;
  IF target_status <> 'ACTIVE' THEN
    RETURN 'NOT_ACTIVE';
  END IF;

  was_empty := NOT EXISTS (SELECT 1 FROM operator);

  INSERT INTO operator (tenant_id, account_id, added_by)
  SELECT a.tenant_id, a.id, p_by FROM account a WHERE a.id = p_account
  ON CONFLICT (tenant_id, account_id) DO NOTHING;
  GET DIAGNOSTICS added = ROW_COUNT;
  IF added = 0 THEN
    RETURN 'PRESENT';
  END IF;

  -- The adder was an operator only because the register was empty; the row just written ends that.
  IF was_empty AND p_by IS NOT NULL AND p_by <> p_account THEN
    INSERT INTO operator (tenant_id, account_id, added_by)
    SELECT a.tenant_id, a.id, p_by
    FROM account a
    WHERE a.id = p_by AND a.deleted_at IS NULL AND a.status = 'ACTIVE'
    ON CONFLICT (tenant_id, account_id) DO NOTHING;
    GET DIAGNOSTICS caller_added = ROW_COUNT;
  END IF;

  IF caller_added > 0 THEN
    RETURN 'ADDED_WITH_CALLER';
  END IF;
  RETURN 'ADDED';
END $$;

CREATE OR REPLACE FUNCTION dismiss_operator(p_account uuid) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE removed integer;
BEGIN
  -- The last one stays, in the statement: two operators removing each other at once would both read
  -- "there are two".
  DELETE FROM operator
  WHERE account_id = p_account
    AND (SELECT count(*) FROM operator) > 1;
  GET DIAGNOSTICS removed = ROW_COUNT;
  IF removed > 0 THEN
    RETURN 'REMOVED';
  END IF;
  IF EXISTS (SELECT 1 FROM operator WHERE account_id = p_account) THEN
    RETURN 'LAST';
  END IF;
  RETURN 'ABSENT';
END $$;

REVOKE ALL ON FUNCTION operator_roster(uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION enlist_operator(uuid, uuid) FROM PUBLIC;
REVOKE ALL ON FUNCTION dismiss_operator(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION operator_roster(uuid) TO hubtask_app;
GRANT EXECUTE ON FUNCTION enlist_operator(uuid, uuid) TO hubtask_app;
GRANT EXECUTE ON FUNCTION dismiss_operator(uuid) TO hubtask_app;

-- ============ The first start (migration 0128, UC-INS-01) ===================
-- A one-time setup code, issued at start while the installation is empty - no tenant row at all and
-- nobody in the operator register - and claimed by the setup that creates the first workspace. Only
-- the code's hash is stored. No row-level policy and no grant to the application role: the operator
-- register's precedent, reached only through the functions below.
CREATE TABLE instance_setup_code (
  id         uuid PRIMARY KEY,
  code_hash  bytea NOT NULL UNIQUE,
  issued_at  timestamptz NOT NULL,
  expires_at timestamptz NOT NULL
);
REVOKE ALL ON instance_setup_code FROM PUBLIC;

CREATE OR REPLACE FUNCTION installation_unset() RETURNS boolean
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT NOT EXISTS (SELECT 1 FROM tenant) AND NOT EXISTS (SELECT 1 FROM operator)
$$;

-- Lands only on an empty installation, and clears the codes that have run out on the way.
CREATE OR REPLACE FUNCTION issue_setup_code(
  p_id uuid, p_hash bytea, p_issued timestamptz, p_expires timestamptz
) RETURNS boolean
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE issued integer;
BEGIN
  IF NOT installation_unset() THEN
    RETURN false;
  END IF;
  DELETE FROM instance_setup_code WHERE expires_at <= p_issued;
  INSERT INTO instance_setup_code (id, code_hash, issued_at, expires_at)
  VALUES (p_id, p_hash, p_issued, p_expires)
  ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS issued = ROW_COUNT;
  RETURN issued > 0;
END $$;

-- Inside the setup's own transaction: an advisory lock serialises two setups, the emptiness is read
-- after it, and spending a code deletes every code. One word out, never a row.
CREATE OR REPLACE FUNCTION claim_setup_code(p_hash bytea, p_now timestamptz) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE expires timestamptz;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtext('hubtask.installation_setup'));
  IF NOT installation_unset() THEN
    RETURN 'ALREADY_DONE';
  END IF;
  SELECT expires_at INTO expires FROM instance_setup_code WHERE code_hash = p_hash;
  IF NOT FOUND THEN
    RETURN 'WRONG';
  END IF;
  IF expires <= p_now THEN
    RETURN 'EXPIRED';
  END IF;
  DELETE FROM instance_setup_code;
  RETURN 'CLAIMED';
END $$;

-- Two numbers for the health report: workspaces and operators, counted as `is_operator` counts.
CREATE OR REPLACE FUNCTION operator_register_state()
RETURNS TABLE (workspaces bigint, operators bigint)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT (SELECT count(*) FROM tenant), (SELECT count(*) FROM operator)
$$;

REVOKE ALL ON FUNCTION installation_unset() FROM PUBLIC;
REVOKE ALL ON FUNCTION issue_setup_code(uuid, bytea, timestamptz, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION claim_setup_code(bytea, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION operator_register_state() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION installation_unset() TO hubtask_app;
GRANT EXECUTE ON FUNCTION issue_setup_code(uuid, bytea, timestamptz, timestamptz) TO hubtask_app;
GRANT EXECUTE ON FUNCTION claim_setup_code(bytea, timestamptz) TO hubtask_app;
GRANT EXECUTE ON FUNCTION operator_register_state() TO hubtask_app;

-- ============ Tenant resolution before a credential exists ==========
-- Sign-in needs a tenant before it can check a password (multi-tenancy.md §3). One identifier or
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

-- In multi mode a request's host names its workspace (migration 0207, ADR-0095 §1): one identifier
-- or none, and only a serving row of a live workspace. The owner reads and writes `tenant_host`
-- across workspaces by the standing policies `doors_read` and `doors_write`.
DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'tenant_host' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON tenant_host FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'tenant_host' AND policyname = 'doors_write') THEN
    CREATE POLICY doors_write ON tenant_host FOR INSERT TO CURRENT_USER WITH CHECK (true);
  END IF;
END $owner$;

CREATE OR REPLACE FUNCTION resolve_tenant_host(request_host text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT h.tenant_id FROM tenant_host h
  JOIN tenant t ON t.id = h.tenant_id
  WHERE h.host = lower(rtrim(request_host, '.'))
    AND h.state = 'ACTIVE'
    AND t.deleted_at IS NULL
  LIMIT 1
$$;
REVOKE ALL ON FUNCTION resolve_tenant_host(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_tenant_host(text) TO hubtask_app;

-- The main row of every live workspace that has none, under the base host a multi-mode server
-- names at its start: the migrator does not know the installation's host. A count, never a row.
CREATE OR REPLACE FUNCTION ensure_main_hosts(base_host text) RETURNS integer
LANGUAGE plpgsql SECURITY DEFINER VOLATILE SET search_path = public, pg_temp AS $$
DECLARE
  base    text := lower(rtrim(coalesce(base_host, ''), '.'));
  written integer;
BEGIN
  IF base = '' OR position('.' in base) = 1 THEN
    RETURN 0;
  END IF;
  INSERT INTO tenant_host (tenant_id, host, state, verification, verified_at, is_canonical, created_at)
  SELECT t.id, lower(t.slug) || '.' || base, 'ACTIVE',
         'hbt-dv-' || replace(gen_random_uuid()::text, '-', ''), now(), true, now()
  FROM tenant t
  WHERE t.deleted_at IS NULL
    AND length(lower(t.slug) || '.' || base) BETWEEN 4 AND 253
    AND NOT EXISTS (SELECT 1 FROM tenant_host h WHERE h.tenant_id = t.id AND h.is_canonical)
  ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS written = ROW_COUNT;
  RETURN written;
END
$$;
REVOKE ALL ON FUNCTION ensure_main_hosts(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_main_hosts(text) TO hubtask_app;

-- A bare address in `HUBTASK_OPERATORS` in single mode (migration 0128): the one workspace's
-- account by address, nothing when there is not exactly one workspace.
CREATE OR REPLACE FUNCTION resolve_single_operator_account(p_email text) RETURNS uuid
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT a.id
  FROM account a
  WHERE a.tenant_id = resolve_tenant(NULL)
    AND lower(a.email) = lower(btrim(p_email))
    AND a.deleted_at IS NULL
  LIMIT 1
$$;
REVOKE ALL ON FUNCTION resolve_single_operator_account(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION resolve_single_operator_account(text) TO hubtask_app;

-- ============ An offered provider's count (ADR-0076 §1) ====================
-- A workspace's switch moves the installation row's count by one step, and nothing else.
CREATE OR REPLACE FUNCTION move_provider_offer(provider uuid, step integer) RETURNS void
LANGUAGE sql SECURITY DEFINER SET search_path = public, pg_temp AS $$
  UPDATE identity_provider
  SET offered_workspaces = greatest(0, offered_workspaces + step)
  WHERE id = provider AND tenant_id IS NULL AND step IN (-1, 1)
$$;

REVOKE ALL ON FUNCTION move_provider_offer(uuid, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION move_provider_offer(uuid, integer) TO hubtask_app;

-- Counted where it is read, since 0115 (ADR-0077 §1): how many live workspaces name the provider among
-- their own switches - a number, never which, and only outside a workspace. The column and the
-- function above stay for one release. Since 0133 an invite-only offer the installation's lock on the
-- ways in turns on (identity.md §18) counts every live workspace, whatever their own switches say.
-- Since 0167 a plan's lock does the same for every live workspace on that plan, where the
-- installation does not lock the ways in itself (multi-tenancy.md §4.2).
CREATE OR REPLACE FUNCTION count_provider_offers(provider uuid) RETURNS integer
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*)::integer FROM tenant t
  WHERE current_tenant_id() IS NULL
    AND t.deleted_at IS NULL
    AND (
      coalesce(t.settings -> 'offered_providers', '[]'::jsonb) ? provider::text
      OR EXISTS (
        SELECT 1 FROM identity_provider p
        JOIN instance_setting s ON s.key = 'sign_in.methods'
        WHERE p.id = provider
          AND p.tenant_id IS NULL
          AND p.provisioning = 'INVITED_ONLY'
          AND p.enabled
          AND (p.withdraw_at IS NULL OR p.withdraw_at > now())
          AND s.lock_origin <> 'OPEN'
          AND jsonb_typeof(s.value) = 'array'
          AND (s.value ? 'OIDC' OR NOT s.value ?| ARRAY['PASSWORD', 'OIDC'])
      )
      OR EXISTS (
        SELECT 1 FROM identity_provider p
        JOIN plan_setting ps ON ps.plan_id = t.plan_id AND ps.key = 'sign_in.methods'
        WHERE p.id = provider
          AND p.tenant_id IS NULL
          AND p.provisioning = 'INVITED_ONLY'
          AND p.enabled
          AND (p.withdraw_at IS NULL OR p.withdraw_at > now())
          AND ps.locked
          AND jsonb_typeof(ps.value) = 'array'
          AND (ps.value ? 'OIDC' OR NOT ps.value ?| ARRAY['PASSWORD', 'OIDC'])
          AND NOT EXISTS (
            SELECT 1 FROM instance_setting s WHERE s.key = 'sign_in.methods' AND s.lock_origin <> 'OPEN'
          )
      )
    )
$$;

REVOKE ALL ON FUNCTION count_provider_offers(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION count_provider_offers(uuid) TO hubtask_app;

-- ============ The control plane's two narrow acts ====================
-- The one legitimate tenant enumerator (multi-tenancy.md §2.1): provisioning and lifecycle are the
-- control plane's job, and the control plane must see its rows. SECURITY DEFINER for
-- resolve_tenant's reason; what bounds it is the application - the use case behind it demands
-- the admin:tenants scope, which no session carries.
-- It answers an operator's opening of the password beside the lifecycle (migration 0118), and
-- whether a legal hold is in force - a state, never which hold (migration 0121), and when the grace
-- job began the purge, after which no cancel lands (migration 0130).
CREATE OR REPLACE FUNCTION admin_tenants()
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz,
  password_opened_until timestamptz, password_opened_requester text, password_opened_reason text,
  legal_hold boolean, purge_started_at timestamptz,
  plan_name text, version integer
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT t.id, t.slug, t.display_name, t.status::text,
         t.default_locale, t.default_time_zone, t.created_at, t.purge_after,
         t.password_opened_until, t.password_opened_requester, t.password_opened_reason,
         EXISTS (SELECT 1 FROM legal_hold h WHERE h.tenant_id = t.id AND h.released_at IS NULL),
         t.purge_started_at,
         p.name, t.version
  FROM tenant t
  LEFT JOIN plan p ON p.id = t.plan_id
  WHERE t.deleted_at IS NULL
  ORDER BY t.created_at, t.id
$$;

REVOKE ALL ON FUNCTION admin_tenants() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenants() TO hubtask_app;

-- The control plane's filtered, paged read of the workspaces (migration 0165): the filters and the
-- keyset inside, so a page reads its own rows. Answers what admin_tenants() answers, and the
-- platform's reference.
CREATE OR REPLACE FUNCTION admin_tenants_query(
  want_id uuid, want_slug text, want_reference text, want_plan text, want_without_plan boolean,
  want_status text, after_at timestamptz, after_id uuid, page_size integer
)
RETURNS TABLE (
  id uuid, slug text, display_name text, status text,
  default_locale text, default_time_zone text,
  created_at timestamptz, purge_after timestamptz,
  password_opened_until timestamptz, password_opened_requester text, password_opened_reason text,
  legal_hold boolean, purge_started_at timestamptz,
  plan_name text, version integer, external_reference text
)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  SELECT t.id, t.slug, t.display_name, t.status::text,
         t.default_locale, t.default_time_zone, t.created_at, t.purge_after,
         t.password_opened_until, t.password_opened_requester, t.password_opened_reason,
         EXISTS (SELECT 1 FROM legal_hold h WHERE h.tenant_id = t.id AND h.released_at IS NULL),
         t.purge_started_at,
         p.name, t.version, t.external_reference
  FROM tenant t
  LEFT JOIN plan p ON p.id = t.plan_id
  WHERE t.deleted_at IS NULL
    AND (want_id IS NULL OR t.id = want_id)
    AND (want_slug IS NULL OR t.slug = want_slug)
    AND (want_reference IS NULL OR t.external_reference = want_reference)
    AND (want_plan IS NULL OR p.name = want_plan)
    AND (NOT coalesce(want_without_plan, false) OR t.plan_id IS NULL)
    AND (want_status IS NULL OR t.status::text = want_status)
    AND (after_at IS NULL OR (t.created_at, t.id) > (after_at, after_id))
  ORDER BY t.created_at, t.id
  LIMIT page_size
$$;

REVOKE ALL ON FUNCTION admin_tenants_query(uuid, text, text, text, boolean, text, timestamptz, uuid, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_tenants_query(uuid, text, text, text, boolean, text, timestamptz, uuid, integer) TO hubtask_app;

-- Removing a plan, refused while a workspace is on it (migration 0164). `tenant` is behind
-- tenant_self, so only the owner sees which workspaces are on a plan: the plan row is locked first,
-- then counted in a statement of its own, so a move that committed meanwhile is counted and a later
-- one finds the plan gone. One word, never a row.
CREATE OR REPLACE FUNCTION remove_plan(removed_plan uuid, at timestamptz, actor uuid)
RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM 1 FROM plan WHERE id = removed_plan AND removed_at IS NULL FOR UPDATE;
  IF NOT FOUND THEN
    RETURN 'NOT_FOUND';
  END IF;
  IF EXISTS (SELECT 1 FROM tenant WHERE plan_id = removed_plan AND deleted_at IS NULL) THEN
    RETURN 'IN_USE';
  END IF;
  UPDATE plan
     SET removed_at = at, updated_at = at, updated_by = actor, version = version + 1
   WHERE id = removed_plan;
  RETURN 'REMOVED';
END $$;

REVOKE ALL ON FUNCTION remove_plan(uuid, timestamptz, uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION remove_plan(uuid, timestamptz, uuid) TO hubtask_app;

-- The trail that dies with its tenant: audit_log rows carry no foreign key to tenant and the
-- application role deliberately holds no DELETE on them (T-15) - both correct for a living
-- tenant, and both in the way of a hard delete whose promise is that the tenant's data is gone.
-- This narrow act squares them; every use is preceded by the instance_event entry that outlives
-- it. See db/migrations/0067_tenant_lifecycle.sql.
CREATE OR REPLACE FUNCTION purge_tenant_trail(purged_tenant uuid) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  removed bigint;
BEGIN
  -- Only the partitioned trail itself: audit_anchor and audit_pseudonym carry foreign keys and
  -- die with the tenant row. The marker opens the immutability trigger for exactly this tenant,
  -- and the window closes before the function returns.
  PERFORM set_config('hubtask.trail_purge', purged_tenant::text, true);
  DELETE FROM audit_log WHERE tenant_id = purged_tenant;
  GET DIAGNOSTICS removed = ROW_COUNT;
  PERFORM set_config('hubtask.trail_purge', '', true);
  RETURN removed;
END $$;

REVOKE ALL ON FUNCTION purge_tenant_trail(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION purge_tenant_trail(uuid) TO hubtask_app;

-- ============ The stream partitions' duty ============================
-- ensure_audit_partition's shape for the three monthly streams, one function rather than three
-- copies: a month's partition, RLS carried, and - unlike the trail - the full grant, since
-- these tables are legitimately updated and swept. See db/migrations/0068_stream_partitions.sql
-- for the whole reasoning, the conversion strategy included.
CREATE OR REPLACE FUNCTION ensure_stream_partition(parent text, month date) RETURNS text
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  starts date := date_trunc('month', month)::date;
  ends   date := (date_trunc('month', month) + interval '1 month')::date;
  name   text := parent || '_' || to_char(date_trunc('month', month), 'YYYY_MM');
  target regclass;
BEGIN
  IF parent NOT IN ('activity_entry', 'outbox_event', 'rule_run', 'change_log') THEN
    RAISE EXCEPTION 'ensure_stream_partition: % is not a partitioned stream', parent
      USING ERRCODE = 'invalid_parameter_value';
  END IF;

  target := to_regclass(format('public.%I', name));
  IF target IS NULL THEN
    BEGIN
      EXECUTE format('CREATE TABLE %I PARTITION OF %I FOR VALUES FROM (%L) TO (%L)',
        name, parent, starts, ends);
    EXCEPTION WHEN check_violation OR invalid_table_definition OR invalid_object_definition THEN
      -- The month is already covered - its rows sit in the default partition, or the history
      -- partition's open-ended range holds it (every pre-conversion month does). Creating the
      -- month now would have to move rows; living with the covering partition is the honest
      -- outcome (ensure_audit_partition's reasoning).
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
  IF NOT EXISTS (SELECT 1 FROM pg_policy WHERE polrelid = target AND polname = 'tenant_isolation') THEN
    EXECUTE format($policy$
      CREATE POLICY tenant_isolation ON %I
        USING (tenant_id = current_tenant_id())
        WITH CHECK (tenant_id = current_tenant_id())
    $policy$, name);
  END IF;
  IF NOT has_table_privilege('hubtask_app', target, 'INSERT')
     OR NOT has_table_privilege('hubtask_app', target, 'SELECT')
     OR NOT has_table_privilege('hubtask_app', target, 'UPDATE')
     OR NOT has_table_privilege('hubtask_app', target, 'DELETE') THEN
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO hubtask_app', name);
  END IF;

  RETURN name;
END $$;

REVOKE ALL ON FUNCTION ensure_stream_partition(text, date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION ensure_stream_partition(text, date) TO hubtask_app;

-- The maintenance read the drop function opens (the trail purge's marker discipline; migration
-- 0068): a dedicated SELECT policy on the two period tables, honouring the transaction-scoped
-- marker the function sets and clears around one aggregate read.
CREATE POLICY retention_maintenance_read ON retention_policy FOR SELECT
  USING (current_setting('hubtask.retention_scan', true) = 'on');
CREATE POLICY retention_maintenance_read ON retention_rule FOR SELECT
  USING (current_setting('hubtask.retention_scan', true) = 'on');

-- The retention half: an aged-out month of a partitioned stream is a dropped partition, not a
-- million-row DELETE. It refuses the default partition and anything not wholly past the cutoff,
-- and counts the rows for the evidence.
CREATE OR REPLACE FUNCTION drop_stream_partition(parent text, default_days integer) RETURNS TABLE (
  dropped text, rows_removed bigint
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  kind      text;
  horizon   integer;
  cutoff    timestamptz;
  candidate record;
  removed   bigint;
BEGIN
  kind := CASE parent
    WHEN 'activity_entry' THEN 'ACTIVITY_ENTRY'
    WHEN 'outbox_event'   THEN 'OUTBOX_EVENT'
    WHEN 'rule_run'       THEN 'RULE_RUN'
    WHEN 'change_log'     THEN 'SYNC_LOG'
    ELSE NULL
  END;
  IF kind IS NULL THEN
    RAISE EXCEPTION 'drop_stream_partition: % is not a partitioned stream', parent
      USING ERRCODE = 'invalid_parameter_value';
  END IF;
  IF default_days <= 0 THEN
    -- A stream with no bound has no aged-out month; nothing may fall.
    RETURN;
  END IF;

  PERFORM set_config('hubtask.retention_scan', 'on', true);
  SELECT GREATEST(
    default_days,
    coalesce((SELECT max(retain_days) FROM retention_policy WHERE data_kind = kind), 0),
    coalesce((SELECT max(retain_days + coalesce(then_after_days, 0) + grace_days)
              FROM retention_rule WHERE data_kind = kind AND enabled), 0)
  ) INTO horizon;
  PERFORM set_config('hubtask.retention_scan', '', true);

  cutoff := now() - make_interval(days => horizon);

  FOR candidate IN
    SELECT c.relname AS name,
           (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO \(''([^'')]+)''\)'))[1] AS upper_bound
    FROM pg_class c
    JOIN pg_inherits i ON i.inhrelid = c.oid
    JOIN pg_class p ON p.oid = i.inhparent
    WHERE p.relname = parent
      AND pg_get_expr(c.relpartbound, c.oid) NOT LIKE 'DEFAULT%'
    ORDER BY c.relname
  LOOP
    IF candidate.upper_bound IS NULL OR candidate.upper_bound::timestamptz > cutoff THEN
      CONTINUE;
    END IF;
    EXECUTE format('SELECT count(*) FROM %I', candidate.name) INTO removed;
    EXECUTE format('ALTER TABLE %I DETACH PARTITION %I', parent, candidate.name);
    EXECUTE format('DROP TABLE %I', candidate.name);
    dropped := candidate.name;
    rows_removed := removed;
    RETURN NEXT;
  END LOOP;
  RETURN;
END $$;

REVOKE ALL ON FUNCTION drop_stream_partition(text, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION drop_stream_partition(text, integer) TO hubtask_app;

-- ============ The trail's period ====================================
-- drop_stream_partition's shape for audit_log: a month goes whole once it ended more than the
-- longest audit period any workspace has ago; a tenant-wide hold keeps it for every workspace in it,
-- the default partition is only counted, and an owner that cannot read across workspaces drops
-- nothing. See db/migrations/0153_audit_partition_drop.sql for the whole reasoning.
CREATE OR REPLACE FUNCTION drop_audit_partition(default_days integer) RETURNS TABLE (
  part text, part_month date, workspace uuid, entries bigint, outcome text
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  horizon   integer;
  cutoff    timestamptz;
  held      uuid[];
  candidate record;
  counted   record;
  is_held   boolean;
  any_rows  boolean;
BEGIN
  IF default_days <= 0 THEN
    RETURN;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles WHERE rolname = current_user AND (rolsuper OR rolbypassrls)
  ) THEN
    part := NULL; part_month := NULL; workspace := NULL; entries := NULL; outcome := 'unreadable';
    RETURN NEXT;
    RETURN;
  END IF;

  PERFORM set_config('hubtask.retention_scan', 'on', true);
  SELECT GREATEST(
    default_days,
    coalesce((SELECT max(p.retain_days) FROM retention_policy p WHERE p.data_kind = 'AUDIT'), 0),
    coalesce((SELECT max(r.retain_days + coalesce(r.then_after_days, 0) + r.grace_days)
              FROM retention_rule r WHERE r.data_kind = 'AUDIT' AND r.enabled), 0)
  ) INTO horizon;
  PERFORM set_config('hubtask.retention_scan', '', true);

  cutoff := now() - make_interval(days => horizon);
  SELECT coalesce(array_agg(DISTINCT h.tenant_id), '{}') INTO held
  FROM legal_hold h WHERE h.scope_kind = 'TENANT' AND h.released_at IS NULL;

  FOR candidate IN
    SELECT c.relname AS name,
           (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'FROM \(''([^'')]+)''\)'))[1] AS lower_bound,
           (regexp_match(pg_get_expr(c.relpartbound, c.oid), 'TO \(''([^'')]+)''\)'))[1] AS upper_bound
    FROM pg_class c
    JOIN pg_inherits i ON i.inhrelid = c.oid
    JOIN pg_class p ON p.oid = i.inhparent
    WHERE p.relname = 'audit_log'
      AND pg_get_expr(c.relpartbound, c.oid) NOT LIKE 'DEFAULT%'
    ORDER BY c.relname
  LOOP
    IF candidate.upper_bound IS NULL OR candidate.upper_bound::timestamptz > cutoff THEN
      CONTINUE;
    END IF;

    EXECUTE format('SELECT EXISTS (SELECT 1 FROM %I WHERE tenant_id = ANY($1))', candidate.name)
      INTO is_held USING held;
    IF is_held THEN
      part := candidate.name; part_month := candidate.lower_bound::date;
      workspace := NULL; entries := NULL; outcome := 'held';
      RETURN NEXT;
      CONTINUE;
    END IF;

    any_rows := false;
    FOR counted IN EXECUTE format(
      'SELECT tenant_id AS tenant, count(*) AS n FROM %I GROUP BY tenant_id', candidate.name)
    LOOP
      any_rows := true;
      part := candidate.name; part_month := candidate.lower_bound::date;
      workspace := counted.tenant; entries := counted.n; outcome := 'dropped';
      RETURN NEXT;
    END LOOP;
    IF NOT any_rows THEN
      part := candidate.name; part_month := candidate.lower_bound::date;
      workspace := NULL; entries := 0; outcome := 'dropped';
      RETURN NEXT;
    END IF;

    EXECUTE format('ALTER TABLE audit_log DETACH PARTITION %I', candidate.name);
    EXECUTE format('DROP TABLE %I', candidate.name);
  END LOOP;

  IF to_regclass('public.audit_log_default') IS NOT NULL THEN
    SELECT count(*) INTO entries FROM audit_log_default d WHERE d.occurred_at < cutoff;
    part := 'audit_log_default'; part_month := NULL; workspace := NULL; outcome := 'default_aged';
    RETURN NEXT;
  END IF;
  RETURN;
END $$;

REVOKE ALL ON FUNCTION drop_audit_partition(integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION drop_audit_partition(integer) TO hubtask_app;

SELECT ensure_stream_partition('activity_entry', (date_trunc('month', now()) + interval '1 month')::date);
SELECT ensure_stream_partition('outbox_event', (date_trunc('month', now()) + interval '1 month')::date);
SELECT ensure_stream_partition('rule_run', (date_trunc('month', now()) + interval '1 month')::date);
SELECT ensure_stream_partition('change_log', (date_trunc('month', now()) + interval '1 month')::date);

-- ============================ Restore drill ==========================
-- Two marker rows per drill run, written by the owner at a recorded moment; the drill restores to
-- a point between them and expects the first and not the second (backup-restore.md §8.5).
-- Installation-scoped, no tenant column, and the application role has no access at all.
CREATE TABLE restore_drill_marker (
  run_id     text NOT NULL,
  seq        smallint NOT NULL CHECK (seq IN (1, 2)),
  written_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  PRIMARY KEY (run_id, seq)
);
REVOKE ALL ON restore_drill_marker FROM hubtask_app;

-- ============ Use the platform can bill by (multi-tenancy.md §4.3, migration 0183) ============
-- Each workspace's entries, stored bytes and people per UTC day, the day's highest and closing count,
-- kept by statement-level triggers where the count changes; the purged workspaces' counts in the
-- installation's ledger; the control plane's read and the 13-month sweep.
-- The people limit's count (PL-04) and the billing count are one definition.
CREATE OR REPLACE FUNCTION count_people() RETURNS bigint
LANGUAGE sql STABLE SET search_path = public, pg_temp AS $$
  SELECT count(*) FROM account
  WHERE kind = 'USER' AND deleted_at IS NULL
    AND (status IN ('ACTIVE', 'RESTRICTED')
         OR (status = 'INVITED' AND (redemption_expires_at IS NULL OR redemption_expires_at > now())))
$$;

-- Whether one account row counts as a person: the same condition, for a transition table's rows.
CREATE OR REPLACE FUNCTION counts_as_person(
  kind account_kind, status account_status, deleted_at timestamptz, expires timestamptz
) RETURNS integer
LANGUAGE sql STABLE SET search_path = public, pg_temp AS $$
  SELECT CASE WHEN kind = 'USER' AND deleted_at IS NULL
                   AND (status IN ('ACTIVE', 'RESTRICTED')
                        OR (status = 'INVITED' AND (expires IS NULL OR expires > now())))
              THEN 1 ELSE 0 END
$$;

-- keep_usage adds one statement's delta to the workspace's day row, or starts the day with a fresh
-- count. Runs as the writer, under the writer's own row policy.
CREATE OR REPLACE FUNCTION keep_usage(kept text, delta bigint) RETURNS void
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE
  workspace uuid := current_tenant_id();
  today date := (now() AT TIME ZONE 'UTC')::date;
  counted bigint;
BEGIN
  IF delta = 0 OR workspace IS NULL THEN
    RETURN;
  END IF;
  -- Only while the workspace's row stands and its purge has not begun: the purge's own deletes,
  -- and the cascade from the row itself, record nothing.
  IF NOT EXISTS (SELECT 1 FROM tenant WHERE id = workspace AND purge_started_at IS NULL) THEN
    RETURN;
  END IF;
  UPDATE usage_record
     SET closing = closing + delta, value = greatest(value, closing + delta)
   WHERE tenant_id = workspace AND period = today AND metric = kept AND closing IS NOT NULL;
  IF FOUND THEN
    RETURN;
  END IF;
  counted := CASE kept
    WHEN 'items' THEN (SELECT count(*) FROM work_item)
    WHEN 'media_bytes' THEN (SELECT coalesce(sum(byte_size), 0)::bigint FROM media_object WHERE deleted_at IS NULL)
    WHEN 'accounts' THEN count_people()
  END;
  -- The count before this statement is how the day began.
  INSERT INTO usage_record (tenant_id, period, metric, value, closing)
  VALUES (workspace, today, kept, greatest(counted - delta, counted), counted)
  ON CONFLICT (tenant_id, period, metric) DO UPDATE
     SET closing = coalesce(usage_record.closing, 0) + delta,
         value = greatest(usage_record.value, coalesce(usage_record.closing, 0) + delta);
END
$$;

CREATE OR REPLACE FUNCTION keep_items_inserted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('items', (SELECT count(*) FROM inserted));
  RETURN NULL;
END
$$;
CREATE OR REPLACE FUNCTION keep_items_deleted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('items', -(SELECT count(*) FROM deleted));
  RETURN NULL;
END
$$;
CREATE OR REPLACE FUNCTION keep_media_inserted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('media_bytes',
    (SELECT coalesce(sum(byte_size), 0)::bigint FROM inserted WHERE deleted_at IS NULL));
  RETURN NULL;
END
$$;
CREATE OR REPLACE FUNCTION keep_media_deleted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('media_bytes',
    -(SELECT coalesce(sum(byte_size), 0)::bigint FROM deleted WHERE deleted_at IS NULL));
  RETURN NULL;
END
$$;
CREATE OR REPLACE FUNCTION keep_media_updated() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('media_bytes',
    (SELECT coalesce(sum(byte_size), 0)::bigint FROM inserted WHERE deleted_at IS NULL)
    - (SELECT coalesce(sum(byte_size), 0)::bigint FROM deleted WHERE deleted_at IS NULL));
  RETURN NULL;
END
$$;
CREATE OR REPLACE FUNCTION keep_people_inserted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('accounts', (SELECT coalesce(sum(counts_as_person(
    kind, status, deleted_at, redemption_expires_at)), 0)::bigint FROM inserted));
  RETURN NULL;
END
$$;
CREATE OR REPLACE FUNCTION keep_people_deleted() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('accounts', -(SELECT coalesce(sum(counts_as_person(
    kind, status, deleted_at, redemption_expires_at)), 0)::bigint FROM deleted));
  RETURN NULL;
END
$$;
CREATE OR REPLACE FUNCTION keep_people_updated() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
BEGIN
  PERFORM keep_usage('accounts',
    (SELECT coalesce(sum(counts_as_person(kind, status, deleted_at, redemption_expires_at)), 0) FROM inserted)
    - (SELECT coalesce(sum(counts_as_person(kind, status, deleted_at, redemption_expires_at)), 0)::bigint FROM deleted));
  RETURN NULL;
END
$$;

DROP TRIGGER IF EXISTS work_item_usage_inserted ON work_item;
CREATE TRIGGER work_item_usage_inserted AFTER INSERT ON work_item
  REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_items_inserted();
DROP TRIGGER IF EXISTS work_item_usage_deleted ON work_item;
CREATE TRIGGER work_item_usage_deleted AFTER DELETE ON work_item
  REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION keep_items_deleted();
DROP TRIGGER IF EXISTS media_object_usage_inserted ON media_object;
CREATE TRIGGER media_object_usage_inserted AFTER INSERT ON media_object
  REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_media_inserted();
DROP TRIGGER IF EXISTS media_object_usage_deleted ON media_object;
CREATE TRIGGER media_object_usage_deleted AFTER DELETE ON media_object
  REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION keep_media_deleted();
DROP TRIGGER IF EXISTS media_object_usage_updated ON media_object;
CREATE TRIGGER media_object_usage_updated AFTER UPDATE ON media_object
  REFERENCING OLD TABLE AS deleted NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_media_updated();
DROP TRIGGER IF EXISTS account_usage_inserted ON account;
CREATE TRIGGER account_usage_inserted AFTER INSERT ON account
  REFERENCING NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_people_inserted();
DROP TRIGGER IF EXISTS account_usage_deleted ON account;
CREATE TRIGGER account_usage_deleted AFTER DELETE ON account
  REFERENCING OLD TABLE AS deleted FOR EACH STATEMENT EXECUTE FUNCTION keep_people_deleted();
DROP TRIGGER IF EXISTS account_usage_updated ON account;
CREATE TRIGGER account_usage_updated AFTER UPDATE ON account
  REFERENCING OLD TABLE AS deleted NEW TABLE AS inserted FOR EACH STATEMENT EXECUTE FUNCTION keep_people_updated();

CREATE TABLE IF NOT EXISTS usage_archive (
  tenant_id uuid   NOT NULL,
  period    date   NOT NULL,
  metric    text   NOT NULL,
  value     bigint NOT NULL,
  closing   bigint,
  PRIMARY KEY (tenant_id, period, metric)
);
GRANT INSERT, SELECT, UPDATE, DELETE ON usage_archive TO hubtask_app;

DO $owner$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'usage_record' AND policyname = 'doors_read') THEN
    CREATE POLICY doors_read ON usage_record FOR SELECT TO CURRENT_USER USING (true);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'usage_record' AND policyname = 'doors_sweep') THEN
    CREATE POLICY doors_sweep ON usage_record FOR DELETE TO CURRENT_USER USING (true);
  END IF;
END $owner$;

-- The control plane's read of every workspace's counts, live and purged, a page of workspaces at a
-- time; for each workspace and metric the last row before the range rides along, so the reader can
-- carry its closing value into the range's first days.
CREATE OR REPLACE FUNCTION admin_usage(
  from_day date, to_day date, want_metric text, want_tenant uuid, after_tenant uuid, page_size integer
)
RETURNS TABLE (tenant_id uuid, period date, metric text, value bigint, closing bigint, purged boolean)
LANGUAGE sql SECURITY DEFINER STABLE SET search_path = public, pg_temp AS $$
  WITH counts AS (
    SELECT r.tenant_id, r.period, r.metric, r.value, r.closing, false AS purged FROM usage_record r
    UNION ALL
    SELECT a.tenant_id, a.period, a.metric, a.value, a.closing, true FROM usage_archive a
  ),
  wanted AS (
    SELECT * FROM counts c
    WHERE current_tenant_id() IS NULL
      AND c.period <= to_day
      AND (want_metric IS NULL OR c.metric = want_metric)
      AND (want_tenant IS NULL OR c.tenant_id = want_tenant)
      AND (after_tenant IS NULL OR c.tenant_id > after_tenant)
  ),
  page AS (
    SELECT DISTINCT w.tenant_id FROM wanted w ORDER BY w.tenant_id LIMIT page_size
  ),
  before_range AS (
    SELECT DISTINCT ON (w.tenant_id, w.metric) w.*
    FROM wanted w JOIN page p ON p.tenant_id = w.tenant_id
    WHERE w.period < from_day
    ORDER BY w.tenant_id, w.metric, w.period DESC
  )
  SELECT * FROM before_range
  UNION ALL
  SELECT w.* FROM wanted w JOIN page p ON p.tenant_id = w.tenant_id WHERE w.period >= from_day
  ORDER BY 1, 3, 2
$$;

-- Counts older than the given day go, inside workspaces and in the installation's ledger.
CREATE OR REPLACE FUNCTION sweep_usage(before_day date) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public, pg_temp AS $$
DECLARE
  swept bigint;
  archived bigint;
BEGIN
  IF current_tenant_id() IS NOT NULL THEN
    RETURN 0;
  END IF;
  DELETE FROM usage_record WHERE period < before_day;
  GET DIAGNOSTICS swept = ROW_COUNT;
  DELETE FROM usage_archive WHERE period < before_day;
  GET DIAGNOSTICS archived = ROW_COUNT;
  RETURN swept + archived;
END
$$;

REVOKE ALL ON FUNCTION admin_usage(date, date, text, uuid, uuid, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION admin_usage(date, date, text, uuid, uuid, integer) TO hubtask_app;
REVOKE ALL ON FUNCTION sweep_usage(date) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sweep_usage(date) TO hubtask_app;

-- The assets and configuration module (M6), migration 0236.
CREATE TABLE cmdb_class (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key            text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  parent_key     text NOT NULL,
  name           text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  icon           text NOT NULL DEFAULT '',
  abstract       boolean NOT NULL DEFAULT false,
  attributes     jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(attributes) = 'array'),
  rules          jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(rules) = 'array'),
  required       jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(required) = 'array'),
  staleness_days integer NOT NULL DEFAULT 0 CHECK (staleness_days BETWEEN 0 AND 3650),
  archived_at    timestamptz,
  created_at     timestamptz NOT NULL,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key)
);

CREATE TABLE cmdb_relation_type (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key          text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name         text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  reverse_name text NOT NULL CHECK (char_length(reverse_name) BETWEEN 1 AND 120),
  impact       text NOT NULL CHECK (impact IN ('NONE', 'SOURCE', 'TARGET', 'BOTH')),
  pairs        jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(pairs) = 'array'),
  archived_at  timestamptz,
  created_at   timestamptz NOT NULL,
  updated_at   timestamptz NOT NULL,
  version      integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key)
);

CREATE TABLE cmdb_source (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$' AND key <> 'manual'),
  kind        text NOT NULL CHECK (kind IN ('FILE', 'API', 'CONNECTOR')),
  rank        integer NOT NULL CHECK (rank BETWEEN 1 AND 1000),
  classes     text[] NOT NULL DEFAULT '{}',
  may_create  boolean NOT NULL DEFAULT false,
  mapping     jsonb,
  last_run_at timestamptz,
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key)
);

CREATE TABLE cmdb_ci (
  id               uuid PRIMARY KEY,
  tenant_id        uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id           uuid NOT NULL,
  class_key        text NOT NULL,
  name             text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 200),
  stage            text NOT NULL CHECK (stage IN
                     ('PLANNED', 'ORDERED', 'IN_STOCK', 'IN_USE', 'MAINTENANCE', 'RETIRED', 'DISPOSED')),
  status           text NOT NULL CHECK (status IN ('OK', 'PENDING', 'BLOCKED', 'MERGED')),
  attributes       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
  provenance       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(provenance) = 'object'),
  owner_account_id uuid,
  owner_group_id   uuid,
  location_ci_id   uuid,
  protection       jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(protection) = 'object'),
  review_due       date,
  quantity         integer NOT NULL DEFAULT 1 CHECK (quantity BETWEEN 1 AND 1000000),
  last_seen_at     timestamptz,
  merged_into      uuid,
  created_by       uuid NOT NULL,
  created_at       timestamptz NOT NULL,
  updated_by       uuid NOT NULL,
  updated_at       timestamptz NOT NULL,
  version          integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_ci_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cmdb_ci_hub_idx ON cmdb_ci (tenant_id, hub_id, class_key, id);

CREATE TABLE cmdb_ci_identifier (
  tenant_id uuid NOT NULL,
  ci_id     uuid NOT NULL,
  hub_id    uuid NOT NULL,
  name      text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  value     text NOT NULL CHECK (char_length(value) BETWEEN 1 AND 500),
  PRIMARY KEY (tenant_id, ci_id, name),
  CONSTRAINT cmdb_ci_identifier_ci_fkey FOREIGN KEY (tenant_id, ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cmdb_ci_identifier_lookup_idx ON cmdb_ci_identifier (tenant_id, hub_id, name, value);

CREATE TABLE cmdb_relation (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL,
  hub_id       uuid NOT NULL,
  type_key     text NOT NULL,
  source_ci_id uuid NOT NULL,
  target_ci_id uuid NOT NULL CHECK (target_ci_id <> source_ci_id),
  data_source  text NOT NULL,
  first_seen   timestamptz NOT NULL,
  last_seen    timestamptz NOT NULL,
  created_by   uuid NOT NULL,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, type_key, source_ci_id, target_ci_id),
  CONSTRAINT cmdb_relation_source_fkey FOREIGN KEY (tenant_id, source_ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_relation_target_fkey FOREIGN KEY (tenant_id, target_ci_id)
    REFERENCES cmdb_ci (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cmdb_relation_source_idx ON cmdb_relation (tenant_id, source_ci_id);
CREATE INDEX cmdb_relation_target_idx ON cmdb_relation (tenant_id, target_ci_id);
CREATE INDEX cmdb_relation_hub_idx ON cmdb_relation (tenant_id, hub_id);

CREATE TABLE cmdb_run (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL,
  hub_id     uuid NOT NULL,
  source_key text NOT NULL,
  actor_id   uuid NOT NULL,
  started_at timestamptz NOT NULL,
  counts     jsonb NOT NULL DEFAULT '{}'::jsonb,
  decisions  jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(decisions) = 'array'),
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_run_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cmdb_run_hub_idx ON cmdb_run (tenant_id, hub_id, started_at DESC);

CREATE TABLE cmdb_hold (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL,
  hub_id      uuid NOT NULL,
  run_id      uuid NOT NULL,
  source_key  text NOT NULL,
  item        jsonb NOT NULL,
  reason      text NOT NULL,
  created_at  timestamptz NOT NULL,
  resolved_at timestamptz,
  resolved_by uuid,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_hold_run_fkey FOREIGN KEY (tenant_id, run_id)
    REFERENCES cmdb_run (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cmdb_hold_hub_idx ON cmdb_hold (tenant_id, hub_id, created_at DESC) WHERE resolved_at IS NULL;

CREATE TABLE cmdb_duplicate (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL,
  hub_id      uuid NOT NULL,
  ci_ids      uuid[] NOT NULL CHECK (cardinality(ci_ids) BETWEEN 2 AND 50),
  rule        text NOT NULL,
  created_at  timestamptz NOT NULL,
  resolved_at timestamptz,
  resolved_by uuid,
  survivor_id uuid,
  UNIQUE (tenant_id, id),
  CONSTRAINT cmdb_duplicate_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cmdb_duplicate_hub_idx ON cmdb_duplicate (tenant_id, hub_id, created_at DESC) WHERE resolved_at IS NULL;

ALTER TABLE cmdb_class ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_class FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_class
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_relation_type ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_relation_type FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_relation_type
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_source ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_source FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_source
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_ci ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_ci FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_ci
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_ci_identifier ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_ci_identifier FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_ci_identifier
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_relation ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_relation FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_relation
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_run ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_run FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_run
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_hold ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_hold FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_hold
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_duplicate ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_duplicate FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_duplicate
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

-- DELETE is the identifier index's, a relation's and the replacing restore's (no use case deletes
-- a CI, a run, a hold or a duplicate).
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_class, cmdb_relation_type, cmdb_source, cmdb_ci,
  cmdb_ci_identifier, cmdb_relation, cmdb_run, cmdb_hold, cmdb_duplicate TO hubtask_app;

-- The connectors, their alerts and the signed deliveries seen (migrations 0238, 0239).
CREATE TABLE cmdb_connector (
  id                    uuid PRIMARY KEY,
  tenant_id             uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  hub_id                uuid NOT NULL,
  key                   text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  kind                  text NOT NULL CHECK (kind IN
                          ('GLPI', 'NETBOX', 'INTUNE', 'ENTRA', 'HETZNER', 'OPENSTACK',
                           'ALERTMANAGER', 'ZABBIX', 'CHECKMK', 'GLPI_AGENT')),
  source_key            text NOT NULL,
  base_url              text NOT NULL DEFAULT '',
  settings              jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(settings) = 'object'),
  allowed_hosts         text[] NOT NULL DEFAULT '{}',
  every_minutes         integer NOT NULL DEFAULT 0 CHECK (every_minutes = 0 OR every_minutes BETWEEN 15 AND 10080),
  enabled               boolean NOT NULL DEFAULT true,
  collection_id         uuid,
  run_as                uuid NOT NULL,
  credential_enc        bytea,
  credential_key_id     text,
  cursor                text NOT NULL DEFAULT '',
  next_run_at           timestamptz,
  last_run_at           timestamptz,
  last_run_id           uuid,
  last_outcome          text NOT NULL DEFAULT '' CHECK (last_outcome IN ('', 'OK', 'EMPTY', 'FAILED')),
  last_error            text NOT NULL DEFAULT '',
  failures              integer NOT NULL DEFAULT 0,
  created_at            timestamptz NOT NULL,
  updated_at            timestamptz NOT NULL,
  version               integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, key),
  CONSTRAINT cmdb_connector_hub_fkey FOREIGN KEY (tenant_id, hub_id)
    REFERENCES container (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT cmdb_connector_credential_whole CHECK ((credential_enc IS NULL) = (credential_key_id IS NULL))
);
CREATE INDEX cmdb_connector_hub_idx ON cmdb_connector (tenant_id, hub_id, key);
CREATE INDEX cmdb_connector_due_idx ON cmdb_connector (tenant_id, next_run_at)
  WHERE enabled AND next_run_at IS NOT NULL;

CREATE TABLE cmdb_alert (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL,
  hub_id        uuid NOT NULL,
  connector_id  uuid NOT NULL,
  dedup_key     text NOT NULL CHECK (char_length(dedup_key) BETWEEN 1 AND 200),
  status        text NOT NULL CHECK (status IN ('FIRING', 'RESOLVED')),
  severity      text NOT NULL DEFAULT '' CHECK (char_length(severity) <= 30),
  title         text NOT NULL DEFAULT '' CHECK (char_length(title) <= 300),
  ci_id         uuid,
  entry_id      uuid,
  first_at      timestamptz NOT NULL,
  last_at       timestamptz NOT NULL,
  resolved_at   timestamptz,
  count         integer NOT NULL DEFAULT 1 CHECK (count >= 1),
  UNIQUE (tenant_id, id),
  UNIQUE (tenant_id, connector_id, dedup_key),
  CONSTRAINT cmdb_alert_connector_fkey FOREIGN KEY (tenant_id, connector_id)
    REFERENCES cmdb_connector (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX cmdb_alert_hub_idx ON cmdb_alert (tenant_id, hub_id, last_at DESC);

CREATE TABLE cmdb_intake_seen (
  tenant_id     uuid NOT NULL,
  connector_id  uuid NOT NULL,
  digest        bytea NOT NULL,
  seen_at       timestamptz NOT NULL,
  PRIMARY KEY (tenant_id, connector_id, digest),
  CONSTRAINT cmdb_intake_seen_connector_fkey FOREIGN KEY (tenant_id, connector_id)
    REFERENCES cmdb_connector (tenant_id, id) ON DELETE CASCADE
);

ALTER TABLE cmdb_connector ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_connector FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_connector
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_alert ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_alert FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_alert
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

ALTER TABLE cmdb_intake_seen ENABLE ROW LEVEL SECURITY;
ALTER TABLE cmdb_intake_seen FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON cmdb_intake_seen
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());

-- DELETE is the replacing restore's and the expired signatures'; no use case deletes a connector
-- or an alert.
GRANT SELECT, INSERT, UPDATE, DELETE ON cmdb_connector, cmdb_alert, cmdb_intake_seen TO hubtask_app;


-- Business calendars, SLA policies and what became of each goal on an entry (migration 0240, M5).
CREATE TABLE business_calendar (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  spec        jsonb NOT NULL,
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT business_calendar_key_uq UNIQUE (tenant_id, key)
);
ALTER TABLE business_calendar ENABLE ROW LEVEL SECURITY;
ALTER TABLE business_calendar FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON business_calendar
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON business_calendar TO hubtask_app;

CREATE TABLE sla_policy (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key            text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name           text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  calendar_key   text CHECK (calendar_key ~ '^[a-z][a-z0-9_]{0,49}$'),
  entry_types    text[] NOT NULL,
  priority_field text NOT NULL DEFAULT '',
  goals          jsonb NOT NULL,
  position       integer NOT NULL DEFAULT 0,
  enabled        boolean NOT NULL DEFAULT true,
  created_at     timestamptz NOT NULL,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT sla_policy_key_uq UNIQUE (tenant_id, key),
  CONSTRAINT sla_policy_calendar_fkey
    FOREIGN KEY (tenant_id, calendar_key) REFERENCES business_calendar (tenant_id, key)
);
ALTER TABLE sla_policy ENABLE ROW LEVEL SECURITY;
ALTER TABLE sla_policy FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sla_policy
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sla_policy TO hubtask_app;

CREATE TABLE entry_sla (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id        uuid NOT NULL,
  goal           text NOT NULL CHECK (goal IN ('FIRST_RESPONSE', 'RESOLUTION')),
  policy_key     text NOT NULL,
  target_seconds bigint NOT NULL CHECK (target_seconds > 0),
  standing       text NOT NULL CHECK (standing IN ('RUNNING', 'PAUSED', 'MET', 'BREACHED')),
  armed_at       timestamptz NOT NULL,
  due_at         timestamptz,
  at_risk_at     timestamptz,
  met_at         timestamptz,
  breached_at    timestamptz,
  updated_at     timestamptz NOT NULL,
  version        integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT entry_sla_goal_uq UNIQUE (tenant_id, item_id, goal),
  CONSTRAINT entry_sla_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE,
  CHECK ((standing = 'MET') = (met_at IS NOT NULL)),
  CHECK (standing <> 'BREACHED' OR breached_at IS NOT NULL)
);
-- A queue sorted by the next breach, and a report by standing.
CREATE INDEX entry_sla_due_idx ON entry_sla (tenant_id, due_at) WHERE standing = 'RUNNING';
CREATE INDEX entry_sla_standing_idx ON entry_sla (tenant_id, policy_key, standing);
ALTER TABLE entry_sla ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_sla FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON entry_sla
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_sla TO hubtask_app;


-- Canned replies (migration 0242, M5).
CREATE TABLE reply_template (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,49}$'),
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  body        text NOT NULL CHECK (char_length(body) BETWEEN 1 AND 20000),
  created_at  timestamptz NOT NULL,
  updated_at  timestamptz NOT NULL,
  version     integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT reply_template_key_uq UNIQUE (tenant_id, key)
);
ALTER TABLE reply_template ENABLE ROW LEVEL SECURITY;
ALTER TABLE reply_template FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON reply_template
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON reply_template TO hubtask_app;


-- How satisfied a requester was with a resolved request (migration 0243, M5).
CREATE TABLE satisfaction_rating (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  item_id    uuid NOT NULL,
  score      smallint NOT NULL CHECK (score BETWEEN 1 AND 5),
  rated_at   timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CONSTRAINT satisfaction_rating_item_uq UNIQUE (tenant_id, item_id),
  CONSTRAINT satisfaction_rating_item_fkey
    FOREIGN KEY (tenant_id, item_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE satisfaction_rating ENABLE ROW LEVEL SECURITY;
ALTER TABLE satisfaction_rating FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON satisfaction_rating
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON satisfaction_rating TO hubtask_app;


-- An entry's scheduling mode and a plan's baselines (migration 0244, M2).
CREATE TABLE entry_schedule (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entry_id   uuid NOT NULL,
  mode       text NOT NULL CHECK (mode IN ('MANUAL', 'AUTOMATIC')),
  milestone  boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL,
  version    integer NOT NULL DEFAULT 1,
  PRIMARY KEY (tenant_id, entry_id),
  CONSTRAINT entry_schedule_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE entry_schedule ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_schedule FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON entry_schedule
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_schedule TO hubtask_app;

CREATE TABLE plan_baseline (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  plan_id     uuid NOT NULL,
  name        text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  taken_at    timestamptz NOT NULL,
  taken_by    uuid,
  entry_count integer NOT NULL DEFAULT 0 CHECK (entry_count BETWEEN 0 AND 10000),
  UNIQUE (tenant_id, id),
  CONSTRAINT plan_baseline_plan_fkey
    FOREIGN KEY (tenant_id, plan_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX plan_baseline_plan_idx ON plan_baseline (tenant_id, plan_id, taken_at);
ALTER TABLE plan_baseline ENABLE ROW LEVEL SECURITY;
ALTER TABLE plan_baseline FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON plan_baseline
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON plan_baseline TO hubtask_app;

CREATE TABLE plan_baseline_entry (
  tenant_id   uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  baseline_id uuid NOT NULL,
  entry_id    uuid NOT NULL,
  start_at    timestamptz,
  due_at      timestamptz,
  PRIMARY KEY (tenant_id, baseline_id, entry_id),
  CONSTRAINT plan_baseline_entry_baseline_fkey
    FOREIGN KEY (tenant_id, baseline_id) REFERENCES plan_baseline (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE plan_baseline_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE plan_baseline_entry FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON plan_baseline_entry
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, DELETE ON plan_baseline_entry TO hubtask_app;

-- Sprints, their entries and an entry's estimate (migration 0245, M2).
CREATE TABLE sprint (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  collection_id   uuid NOT NULL,
  name            text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
  goal            text NOT NULL DEFAULT '' CHECK (char_length(goal) <= 500),
  start_on        date NOT NULL,
  end_on          date NOT NULL,
  state           text NOT NULL CHECK (state IN ('PLANNED', 'ACTIVE', 'CLOSED')),
  unit            text NOT NULL DEFAULT 'POINTS' CHECK (unit IN ('POINTS', 'HOURS')),
  committed_value double precision NOT NULL DEFAULT 0,
  committed_count integer NOT NULL DEFAULT 0,
  completed_value double precision NOT NULL DEFAULT 0,
  completed_count integer NOT NULL DEFAULT 0,
  started_at      timestamptz,
  closed_at       timestamptz,
  created_at      timestamptz NOT NULL,
  created_by      uuid,
  updated_at      timestamptz NOT NULL,
  version         integer NOT NULL DEFAULT 1,
  UNIQUE (tenant_id, id),
  CHECK (end_on >= start_on AND end_on - start_on < 92),
  CONSTRAINT sprint_collection_fkey
    FOREIGN KEY (tenant_id, collection_id) REFERENCES container (tenant_id, id) ON DELETE CASCADE
);
CREATE INDEX sprint_collection_idx ON sprint (tenant_id, collection_id, start_on);
CREATE UNIQUE INDEX sprint_one_active_idx ON sprint (tenant_id, collection_id)
  WHERE state = 'ACTIVE';
ALTER TABLE sprint ENABLE ROW LEVEL SECURITY;
ALTER TABLE sprint FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sprint
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sprint TO hubtask_app;

CREATE TABLE sprint_entry (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  sprint_id  uuid NOT NULL,
  entry_id   uuid NOT NULL,
  added_at   timestamptz NOT NULL,
  removed_at timestamptz,
  committed  boolean NOT NULL DEFAULT false,
  outcome    text CHECK (outcome IN ('DONE', 'CARRIED', 'RETURNED')),
  PRIMARY KEY (tenant_id, sprint_id, entry_id),
  CONSTRAINT sprint_entry_sprint_fkey
    FOREIGN KEY (tenant_id, sprint_id) REFERENCES sprint (tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT sprint_entry_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX sprint_entry_one_open_idx ON sprint_entry (tenant_id, entry_id)
  WHERE removed_at IS NULL AND outcome IS NULL;
ALTER TABLE sprint_entry ENABLE ROW LEVEL SECURITY;
ALTER TABLE sprint_entry FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sprint_entry
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON sprint_entry TO hubtask_app;

CREATE TABLE entry_estimate (
  tenant_id  uuid NOT NULL REFERENCES tenant(id) ON DELETE CASCADE,
  entry_id   uuid NOT NULL,
  unit       text NOT NULL CHECK (unit IN ('POINTS', 'HOURS', 'SIZE')),
  value      double precision NOT NULL CHECK (value >= 0 AND value <= 10000),
  size       text CHECK (size IN ('XS', 'S', 'M', 'L', 'XL', 'XXL')),
  updated_at timestamptz NOT NULL,
  updated_by uuid,
  version    integer NOT NULL DEFAULT 1,
  PRIMARY KEY (tenant_id, entry_id),
  CHECK ((unit = 'SIZE') = (size IS NOT NULL)),
  CONSTRAINT entry_estimate_entry_fkey
    FOREIGN KEY (tenant_id, entry_id) REFERENCES work_item (tenant_id, id) ON DELETE CASCADE
);
ALTER TABLE entry_estimate ENABLE ROW LEVEL SECURITY;
ALTER TABLE entry_estimate FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON entry_estimate
  USING (tenant_id = current_tenant_id()) WITH CHECK (tenant_id = current_tenant_id());
GRANT SELECT, INSERT, UPDATE, DELETE ON entry_estimate TO hubtask_app;

COMMIT;
