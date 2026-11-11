-- Look of the Alelo card drawing: the classic green or the Alelo Tudo identity.
-- Existing cards take the Alelo Tudo look, the one the owner chose.

-- +goose Up
ALTER TABLE alelo_cards ADD COLUMN style TEXT NOT NULL DEFAULT 'tudo' CHECK (style IN ('classic', 'tudo'));

-- +goose Down
ALTER TABLE alelo_cards DROP COLUMN style;
