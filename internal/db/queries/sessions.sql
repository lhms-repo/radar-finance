-- name: FindSession :one
SELECT data
FROM sessions
WHERE token = ? AND expiry_ms > ?;

-- name: UpsertSession :exec
INSERT INTO sessions (token, data, expiry_ms)
VALUES (?, ?, ?)
ON CONFLICT (token) DO UPDATE SET data = excluded.data, expiry_ms = excluded.expiry_ms;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token = ?;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expiry_ms <= ?;

-- name: SetSessionGroup :exec
-- Only a group switch writes it; the commits of scs keep it (RF-81).
UPDATE sessions
SET group_id = ?
WHERE token = ?;

-- name: SessionGroup :one
SELECT CAST(coalesce(group_id, 0) AS INTEGER) AS group_id
FROM sessions
WHERE token = ?;
