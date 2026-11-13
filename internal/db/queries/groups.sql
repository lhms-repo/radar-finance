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
-- Shared groups only: a personal space is never offered at the entrance (RF-01, RF-83).
SELECT id, name
FROM groups
WHERE kind = 'shared'
ORDER BY name COLLATE NOCASE, id;

-- name: ListGroupsOfPerson :many
-- The personal space first, then the shared groups by name (RF-81, RF-83).
SELECT g.id, g.name, g.kind, m.role
FROM group_members m
JOIN groups g ON g.id = m.group_id
WHERE m.person_id = ?
ORDER BY g.kind = 'personal' DESC, g.name COLLATE NOCASE, g.id;

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

-- name: EnsurePersonalGroup :exec
-- Creates the personal space of a person once (RF-83).
INSERT INTO groups (name, kind, owner_person_id, created_by, created_at)
VALUES ('Pessoal', 'personal', sqlc.arg(person_id), sqlc.arg(created_by), sqlc.arg(created_at))
ON CONFLICT (owner_person_id) DO NOTHING;

-- name: EnsurePersonalMember :exec
-- Makes the person the only member of her personal space (RF-83).
INSERT OR IGNORE INTO group_members (group_id, person_id, role, weight_bp, joined_at)
SELECT g.id, g.owner_person_id, 'admin', 10000, sqlc.arg(joined_at)
FROM groups g
WHERE g.owner_person_id = sqlc.arg(person_id);
