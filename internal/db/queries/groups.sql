-- name: CreateGroup :one
INSERT INTO groups (name, created_by, created_at)
VALUES (?, ?, ?)
RETURNING *;

-- name: RenameGroup :exec
UPDATE groups
SET name = ?
WHERE id = ?;

-- name: GetGroup :one
SELECT *
FROM groups
WHERE id = ?;

-- name: ListGroups :many
SELECT id, name
FROM groups
ORDER BY name COLLATE NOCASE, id;

-- name: ListGroupsOfPerson :many
SELECT g.id, g.name, m.role
FROM group_members m
JOIN groups g ON g.id = m.group_id
WHERE m.person_id = ?
ORDER BY g.name COLLATE NOCASE, g.id;

-- name: AddGroupMember :exec
INSERT INTO group_members (group_id, person_id, role, weight_bp, joined_at)
VALUES (?, ?, ?, ?, ?);

-- name: SetGroupMemberRole :exec
UPDATE group_members
SET role = ?
WHERE group_id = ? AND person_id = ?;

-- name: SetGroupMemberWeight :exec
UPDATE group_members
SET weight_bp = ?
WHERE group_id = ? AND person_id = ?;

-- name: GetGroupMember :one
SELECT group_id, person_id, role, weight_bp
FROM group_members
WHERE group_id = ? AND person_id = ?;

-- name: ListGroupMembers :many
SELECT u.id AS user_id, u.login, p.id AS person_id, p.name AS person_name, m.role, m.weight_bp,
       CAST(u.pin_hash IS NOT NULL AS INTEGER) AS has_pin, m.joined_at
FROM group_members m
JOIN persons p ON p.id = m.person_id
JOIN users u ON u.person_id = p.id
WHERE m.group_id = ?
ORDER BY m.joined_at, p.id;
