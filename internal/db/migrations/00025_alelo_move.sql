-- An Alelo purchase can move between the personal space and the groups of the holder (RF-83). Like the other
-- items, its division between members goes away when it changes space, since the members differ.

-- +goose Up
-- +goose StatementBegin
CREATE TRIGGER alelo_purchases_member_splits_move AFTER UPDATE OF group_id ON alelo_purchases WHEN old.group_id IS NOT new.group_id BEGIN
    DELETE FROM member_splits WHERE item_kind = 'alelo' AND item_id = old.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER alelo_purchases_member_splits_move;
