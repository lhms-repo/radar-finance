-- +goose Up
-- The active group lives beside the session data, written only by a group switch. scs saves the whole session at
-- the end of every request, so a request that loaded the session before a switch would write the old group back
-- if the group stayed only inside the data (RF-81).
ALTER TABLE sessions ADD COLUMN group_id INTEGER;

-- +goose Down
ALTER TABLE sessions DROP COLUMN group_id;
