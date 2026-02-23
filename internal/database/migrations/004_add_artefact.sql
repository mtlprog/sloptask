-- +goose Up
ALTER TABLE tasks ADD COLUMN artefact TEXT;

COMMENT ON COLUMN tasks.artefact IS 'External artefact URL proving task completion (required for DONE status)';

-- +goose Down
ALTER TABLE tasks DROP COLUMN artefact;
