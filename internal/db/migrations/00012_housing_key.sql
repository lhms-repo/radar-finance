-- System key of the Moradia category, used by the housing share of the
-- overview (section 5.3), so a rename does not break the link.

-- +goose Up
UPDATE categories SET system_key = 'housing' WHERE name = 'Moradia' AND parent_id IS NULL;

-- +goose Down
UPDATE categories SET system_key = NULL WHERE system_key = 'housing';
