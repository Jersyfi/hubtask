-- A page's cover (work-surface concept E-18, UC-DOC-16): a gradient of the design system, a picture
-- of the shipped gallery or an uploaded image, with how far down the picture is shown. The domain
-- checks the document; the column holds it as written.
--
-- Expand only (rule 12): a nullable column without a default writes no row and takes no scan. A pod
-- of the previous release never reads it; its UpdatePage leaves it as it stands.
-- +goose Up
ALTER TABLE page ADD COLUMN IF NOT EXISTS cover jsonb;

-- +goose Down
-- Forward only (rule 12).
