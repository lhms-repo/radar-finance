-- name: ListNotificationReads :many
SELECT key
FROM notification_reads
WHERE user_id = ?;

-- name: MarkNotificationRead :exec
INSERT INTO notification_reads (user_id, key, read_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id, key) DO NOTHING;
