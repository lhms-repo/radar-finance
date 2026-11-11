-- Notifications are derived from the current state (RF-64); this table only
-- keeps which ones each user already read, by a stable key such as
-- "overdue:bill:7:2026-11".

-- +goose Up
CREATE TABLE notification_reads (
    user_id INTEGER  NOT NULL REFERENCES users (id),
    key     TEXT     NOT NULL,
    read_at DATETIME NOT NULL,
    PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE notification_reads;
