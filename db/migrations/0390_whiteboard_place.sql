-- A whiteboard's place in its hub's content tree (navigation concept §7.2, UC-CNV-10): the page it
-- stands under, or none for the top of the space, and its key among its siblings - pages and boards
-- in one order, the key space of page.order_key.
--
-- No foreign key on parent_page_id: a page is trashed, not deleted, and the boards under a page
-- are lifted by the application when it is trashed (UC-CNV-10 check 4); a hub's purge removes
-- pages and boards together, which a key would have to order.
--
-- Expand (rule 12): two nullable columns, which write no row under the lock; then the backfill
-- gives every existing board a key after its hub's last top-level page, A-Z by title (check 5).
-- A pod of the previous release writes a board without a key; it sorts after the keyed ones, by
-- title, until it is moved. The NOT NULL on order_key is the contract step of a later release.
-- +goose Up
ALTER TABLE whiteboard ADD COLUMN IF NOT EXISTS parent_page_id uuid;
ALTER TABLE whiteboard ADD COLUMN IF NOT EXISTS order_key text;

-- The key: the hub's last top-level page key (or the first key, a0), extended by three base-62
-- digits of the board's rank and a closing V, so it sorts after that page and before whatever is
-- appended after it, and never ends in the digit 0 (core/domain/service/Ordering.go).
WITH last_page AS (
  SELECT tenant_id, hub_id, max(order_key) AS order_key
  FROM page
  WHERE parent_id IS NULL AND deleted_at IS NULL
  GROUP BY tenant_id, hub_id
), ranked AS (
  SELECT w.tenant_id, w.id, w.hub_id,
         row_number() OVER (PARTITION BY w.tenant_id, w.hub_id ORDER BY w.title COLLATE hubtask_name, w.id) AS n
  FROM whiteboard w
  WHERE w.order_key IS NULL
)
UPDATE whiteboard w
SET order_key = coalesce(l.order_key, 'a0')
  || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', ((r.n / 3844) % 62)::int + 1, 1)
  || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', ((r.n / 62) % 62)::int + 1, 1)
  || substr('0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz', (r.n % 62)::int + 1, 1)
  || 'V'
FROM ranked r
LEFT JOIN last_page l ON l.tenant_id = r.tenant_id AND l.hub_id = r.hub_id
WHERE w.tenant_id = r.tenant_id AND w.id = r.id;

-- +goose Down
-- +goose StatementBegin
DO $forward_only$
BEGIN
  RAISE EXCEPTION 'migrations are forward-only (rule 12); recovery is a restore, not a down migration'
    USING ERRCODE = 'feature_not_supported';
END $forward_only$;
-- +goose StatementEnd
