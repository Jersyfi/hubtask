-- +goose Up
-- `RestoreRequest.create_safety_backup` has been in the contract since A-06 with a default of true,
-- and backup-restore.md §8.3 step 4 makes the copy part of the procedure: "an automatic safety copy
-- of the current state before destructive modes (if there is room at the target)". The parenthesis
-- is why it can be declined - an operator restoring onto a target that is full has to be able to
-- say so - and a request field that is accepted and then forgotten is a field that lies.
--
-- The row is written when the restore is accepted and read minutes later by the job that takes the
-- copy, so the answer has to survive in between. Expand only; true for everything already written,
-- which is what the contract's default says it would have been.
ALTER TABLE restore_run ADD COLUMN create_safety_backup boolean NOT NULL DEFAULT true;

-- +goose Down
-- Development only; in production the rule is to fix forwards (versioning-release.md §4).
ALTER TABLE restore_run DROP COLUMN create_safety_backup;
