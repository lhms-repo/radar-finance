-- name: ListShoppingLists :many
SELECT l.*,
       CAST((SELECT count(*) FROM list_items i WHERE i.list_id = l.id) AS INTEGER) AS item_count,
       CAST((SELECT count(*) FROM list_items i WHERE i.list_id = l.id AND i.picked) AS INTEGER) AS picked_count
FROM shopping_lists l
ORDER BY l.status = 'done', l.created_at DESC, l.id DESC;

-- name: GetShoppingList :one
SELECT *
FROM shopping_lists
WHERE id = ?;

-- name: CreateShoppingList :one
INSERT INTO shopping_lists (name, created_by, created_at)
VALUES (?, ?, ?)
RETURNING *;

-- name: RenameShoppingList :exec
UPDATE shopping_lists
SET name = ?
WHERE id = ?;

-- name: DeleteShoppingList :exec
DELETE FROM shopping_lists
WHERE id = ?;

-- name: ListItems :many
SELECT i.*, p.name AS product_name, p.brand, p.by_weight, p.size_milli, p.unit, pe.name AS picked_by_name
FROM list_items i
JOIN products p ON p.id = i.product_id
LEFT JOIN users u ON u.id = i.picked_by
LEFT JOIN persons pe ON pe.id = u.person_id
WHERE i.list_id = ?
ORDER BY i.picked, p.name COLLATE NOCASE, p.brand COLLATE NOCASE;

-- name: GetListItem :one
SELECT *
FROM list_items
WHERE id = ?;

-- name: ListItemByProduct :one
SELECT *
FROM list_items
WHERE list_id = ? AND product_id = ?;

-- name: CreateListItem :one
INSERT INTO list_items (list_id, product_id, quantity_milli, created_at)
VALUES (?, ?, ?, ?)
RETURNING id;

-- name: SetListItemQuantity :exec
UPDATE list_items
SET quantity_milli = ?
WHERE id = ?;

-- name: SetListItemPicked :exec
UPDATE list_items
SET picked = ?, picked_by = ?
WHERE id = ?;

-- name: DeleteListItem :exec
DELETE FROM list_items
WHERE id = ?;
