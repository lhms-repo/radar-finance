-- Foundation schema: people of the couple, users and sessions (RF-01, RF-02).
-- Users have no password: the couple picks who is using the app (RF-01).
--
-- Convention for every business table created in later phases (RF-03):
--   created_by INTEGER NOT NULL REFERENCES users(id)
--   created_at DATETIME NOT NULL
-- Money columns are INTEGER cents (RN-21). Timestamps are stored in UTC.

-- +goose Up
CREATE TABLE persons (
    id         INTEGER PRIMARY KEY,
    name       TEXT     NOT NULL CHECK (length(trim(name)) > 0),
    created_at DATETIME NOT NULL
);

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    login      TEXT     NOT NULL UNIQUE COLLATE NOCASE CHECK (length(trim(login)) > 0),
    person_id  INTEGER  NOT NULL UNIQUE REFERENCES persons (id),
    theme      TEXT     NOT NULL DEFAULT 'system' CHECK (theme IN ('system', 'light', 'dark')),
    haptics    BOOLEAN  NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE sessions (
    token     TEXT    PRIMARY KEY,
    data      BLOB    NOT NULL,
    expiry_ms INTEGER NOT NULL
);

CREATE INDEX sessions_expiry_idx ON sessions (expiry_ms);

-- +goose Down
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE persons;
