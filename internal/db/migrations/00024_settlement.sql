-- +goose Up
-- Settlement between the members of a group (Fase 10, RF-84, RN-28 to RN-31).

-- Day the settlement of a group starts; nothing before it counts (RN-31). Groups that exist now start today, in
-- the business time zone (America/Sao_Paulo, three hours behind UTC).
ALTER TABLE groups ADD COLUMN settles_from TEXT;
UPDATE groups SET settles_from = date('now', '-3 hours');

-- Weight of each member from a day on. An expense uses the weights in force on its day, and a member takes part
-- only from the day of her first weight, so joining or changing weights never rewrites the past (RN-28).
CREATE TABLE group_weights (
    group_id  INTEGER NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    person_id INTEGER NOT NULL REFERENCES persons (id),
    weight_bp INTEGER NOT NULL CHECK (weight_bp BETWEEN 1 AND 10000),
    from_day  TEXT    NOT NULL,
    PRIMARY KEY (group_id, person_id, from_day)
);
INSERT INTO group_weights (group_id, person_id, weight_bp, from_day)
SELECT m.group_id, m.person_id, m.weight_bp, g.settles_from
FROM group_members m JOIN groups g ON g.id = m.group_id
WHERE g.kind = 'shared';

-- Division between members chosen when an expense was recorded, when it is not by the weights (RN-28). Equal
-- lists who took part; custom holds each member's value, used as a proportion on every occurrence of a bill,
-- financing or recurrence.
CREATE TABLE member_splits (
    item_kind   TEXT    NOT NULL CHECK (item_kind IN ('entry', 'purchase', 'recurrence', 'bill', 'financing', 'alelo')),
    item_id     INTEGER NOT NULL,
    mode        TEXT    NOT NULL CHECK (mode IN ('equal', 'custom')),
    person_id   INTEGER NOT NULL REFERENCES persons (id),
    share_cents INTEGER NOT NULL DEFAULT 0 CHECK (share_cents >= 0),
    PRIMARY KEY (item_kind, item_id, person_id)
);

-- A division goes away with its item, or when the item moves to another space, so a reused id never inherits it.
-- +goose StatementBegin
CREATE TRIGGER entries_member_splits_delete AFTER DELETE ON entries BEGIN
    DELETE FROM member_splits WHERE item_kind = 'entry' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER entries_member_splits_move AFTER UPDATE OF group_id ON entries WHEN old.group_id IS NOT new.group_id BEGIN
    DELETE FROM member_splits WHERE item_kind = 'entry' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER purchases_member_splits_delete AFTER DELETE ON purchases BEGIN
    DELETE FROM member_splits WHERE item_kind = 'purchase' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER purchases_member_splits_move AFTER UPDATE OF group_id ON purchases WHEN old.group_id IS NOT new.group_id BEGIN
    DELETE FROM member_splits WHERE item_kind = 'purchase' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER recurrences_member_splits_delete AFTER DELETE ON recurrences BEGIN
    DELETE FROM member_splits WHERE item_kind = 'recurrence' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER bills_member_splits_delete AFTER DELETE ON bills BEGIN
    DELETE FROM member_splits WHERE item_kind = 'bill' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER bills_member_splits_move AFTER UPDATE OF group_id ON bills WHEN old.group_id IS NOT new.group_id BEGIN
    DELETE FROM member_splits WHERE item_kind = 'bill' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER financings_member_splits_delete AFTER DELETE ON financings BEGIN
    DELETE FROM member_splits WHERE item_kind = 'financing' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER financings_member_splits_move AFTER UPDATE OF group_id ON financings WHEN old.group_id IS NOT new.group_id BEGIN
    DELETE FROM member_splits WHERE item_kind = 'financing' AND item_id = old.id;
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER alelo_purchases_member_splits_delete AFTER DELETE ON alelo_purchases BEGIN
    DELETE FROM member_splits WHERE item_kind = 'alelo' AND item_id = old.id;
END;
-- +goose StatementEnd

-- A payment between members is a transfer between their accounts, kept apart so it counts in the settlement
-- and never as spending or income (RN-30). Deleting the transfer undoes it.
CREATE TABLE settlements (
    id             INTEGER  PRIMARY KEY,
    group_id       INTEGER  NOT NULL REFERENCES groups (id),
    transfer_id    INTEGER  NOT NULL UNIQUE REFERENCES transfers (id) ON DELETE CASCADE,
    from_person_id INTEGER  NOT NULL REFERENCES persons (id),
    to_person_id   INTEGER  NOT NULL REFERENCES persons (id),
    amount_cents   INTEGER  NOT NULL CHECK (amount_cents > 0),
    paid_on        TEXT     NOT NULL,
    created_by     INTEGER  NOT NULL REFERENCES users (id),
    created_at     DATETIME NOT NULL,
    CHECK (from_person_id <> to_person_id)
);
CREATE INDEX settlements_group_idx ON settlements (group_id, paid_on);

-- +goose Down
DROP TABLE settlements;
DROP TRIGGER alelo_purchases_member_splits_delete;
DROP TRIGGER financings_member_splits_move;
DROP TRIGGER financings_member_splits_delete;
DROP TRIGGER bills_member_splits_move;
DROP TRIGGER bills_member_splits_delete;
DROP TRIGGER recurrences_member_splits_delete;
DROP TRIGGER purchases_member_splits_move;
DROP TRIGGER purchases_member_splits_delete;
DROP TRIGGER entries_member_splits_move;
DROP TRIGGER entries_member_splits_delete;
DROP TABLE member_splits;
DROP TABLE group_weights;
ALTER TABLE groups DROP COLUMN settles_from;
