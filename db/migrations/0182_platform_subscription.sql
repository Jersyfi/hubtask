-- The installation's subscriptions for a purchase platform, and their deliveries (UC-INS-14,
-- multi-tenancy.md §4.3).
--
-- A subscription is an address every installation event is pushed to, signed with a secret sealed
-- under the installation's key. It is `ACTIVE` or `PAUSED` and never switched off for being
-- unreachable: a delivery that ends without arriving stays as a dead letter, which the operator
-- redelivers.
--
-- A delivery is one event sent to one subscription, one row across its attempts (`attempt` counts
-- on, a redelivery's included), unique per subscription and event, so a fan-out repeated writes it
-- once. The event is named by its id rather than referenced: the events are swept on their own
-- clock, and a delivery whose event is gone settles as expired.
--
-- Neither table carries a tenant: they belong to the installation, and are reached only by the
-- control plane's use cases behind `admin:tenants` and the register, and by the installation's
-- delivery jobs under the system scope.
--
-- Expand only and safe for a rolling update (rule 12): two new tables.

-- +goose Up
CREATE TABLE IF NOT EXISTS platform_subscription (
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

CREATE TABLE IF NOT EXISTS platform_delivery (
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

-- The screen's dead letters and the redelivery read by status; the sweep by age.
CREATE INDEX IF NOT EXISTS platform_delivery_status_idx ON platform_delivery (subscription_id, status);
CREATE INDEX IF NOT EXISTS platform_delivery_updated_idx ON platform_delivery (updated_at);

GRANT SELECT, INSERT, UPDATE, DELETE ON platform_subscription TO hubtask_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON platform_delivery TO hubtask_app;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
