-- name: CreatePerson :one
INSERT INTO persons (name, created_at)
VALUES (?, ?)
RETURNING *;

-- name: CreateUser :one
INSERT INTO users (login, person_id, created_at, updated_at)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: GetUserByLogin :one
SELECT u.id, u.login, u.theme, u.haptics, u.person_id, p.name AS person_name
FROM users u
JOIN persons p ON p.id = u.person_id
WHERE u.login = ?;

-- name: GetUserByID :one
SELECT u.id, u.login, u.theme, u.haptics, u.person_id, p.name AS person_name
FROM users u
JOIN persons p ON p.id = u.person_id
WHERE u.id = ?;

-- name: ListUsers :many
SELECT u.id, u.login, p.name AS person_name, u.created_at
FROM users u
JOIN persons p ON p.id = u.person_id
ORDER BY u.id;

-- name: UpdateUserPreferences :exec
UPDATE users
SET theme = ?, haptics = ?, updated_at = ?
WHERE id = ?;

-- name: ListPersons :many
SELECT id, name
FROM persons
ORDER BY id;
