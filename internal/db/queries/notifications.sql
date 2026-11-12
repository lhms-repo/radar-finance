-- name: ListNotificationReads :many
SELECT key
FROM notification_reads
WHERE user_id = ?;

-- name: MarkNotificationRead :exec
INSERT INTO notification_reads (user_id, key, read_at)
VALUES (?, ?, ?)
ON CONFLICT (user_id, key) DO NOTHING;

-- name: ListRegistrations :many
-- When each bill, financing and card was registered, so overdue warnings start there (RF-64).
SELECT CAST('bill' AS TEXT) AS kind, b.id, b.created_at
FROM bills b
WHERE b.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
UNION ALL
SELECT 'financing', f.id, f.created_at
FROM financings f
WHERE f.group_id = CAST(sqlc.arg(group_id) AS INTEGER)
UNION ALL
SELECT 'invoice', c.id, c.created_at
FROM cards c
WHERE EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = CAST(sqlc.arg(group_id) AS INTEGER) AND gm.person_id = c.person_id);
