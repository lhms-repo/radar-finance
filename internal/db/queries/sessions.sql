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
