-- +goose Up
-- E-01 gave a job a `progress` field on the contract and documented null as the honest answer from
-- a job that cannot compute one. Every job answered null, because there was nowhere to put a
-- number. E-05 brings the first job long enough for the question to be asked out loud - a backup
-- run takes minutes and a caller polls it - so the column arrives now rather than being invented
-- per job kind afterwards.
--
-- Nullable, and nullable on purpose: most jobs still cannot say, and a default of zero would make
-- every one of them look as if it had started and got nowhere. Expand only; nothing reads it yet
-- except the resource that already promised it.
ALTER TABLE job ADD COLUMN progress real;

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE job DROP COLUMN progress;
