-- name: ListCategories :many
SELECT *
FROM categories
ORDER BY parent_id IS NOT NULL, sort_order, id;

-- name: GetCategory :one
SELECT *
FROM categories
WHERE id = ?;

-- name: NextCategorySortOrder :one
SELECT CAST(coalesce(max(sort_order), 0) + 1 AS INTEGER)
FROM categories
WHERE parent_id IS sqlc.narg(parent_id);

-- name: CreateCategory :one
INSERT INTO categories (parent_id, name, icon, sort_order, created_by, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: UpdateCategory :exec
UPDATE categories
SET parent_id = ?, name = ?, icon = ?, active = ?, sort_order = ?, updated_at = ?
WHERE id = ?;

-- name: CountCategoryChildren :one
SELECT count(*)
FROM categories
WHERE parent_id = ?;

-- name: DeleteCategory :exec
DELETE FROM categories
WHERE id = ?;

-- name: GetSelectableCategory :one
SELECT c.id, c.active, p.active AS parent_active
FROM categories c
JOIN categories p ON p.id = c.parent_id
WHERE c.id = ?;

-- name: CategoryInUse :one
-- Whether anything registered points to the category, which then must stay a subcategory.
SELECT CAST(EXISTS (SELECT 1 FROM entries e WHERE e.category_id = sqlc.arg(id))
    OR EXISTS (SELECT 1 FROM purchases p WHERE p.category_id = sqlc.arg(id))
    OR EXISTS (SELECT 1 FROM recurrences r WHERE r.category_id = sqlc.arg(id))
    OR EXISTS (SELECT 1 FROM bills b WHERE b.category_id = sqlc.arg(id))
    OR EXISTS (SELECT 1 FROM financings f WHERE f.category_id = sqlc.arg(id))
    OR EXISTS (SELECT 1 FROM alelo_purchases ap WHERE ap.category_id = sqlc.arg(id))
    OR EXISTS (SELECT 1 FROM alias_rules ar WHERE ar.category_id = sqlc.arg(id)) AS INTEGER) AS in_use;
