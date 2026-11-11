-- How a finished shopping list was paid: the real total, the date and the way,
-- an account, a credit card or an Alelo card (RF-77, RN-26).

-- +goose Up
ALTER TABLE shopping_lists ADD COLUMN paid_cents INTEGER CHECK (paid_cents IS NULL OR paid_cents > 0);
ALTER TABLE shopping_lists ADD COLUMN paid_on TEXT;
ALTER TABLE shopping_lists ADD COLUMN paid_method TEXT CHECK (paid_method IS NULL OR paid_method IN ('account', 'card', 'alelo'));
ALTER TABLE shopping_lists ADD COLUMN paid_with TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE shopping_lists DROP COLUMN paid_with;
ALTER TABLE shopping_lists DROP COLUMN paid_method;
ALTER TABLE shopping_lists DROP COLUMN paid_on;
ALTER TABLE shopping_lists DROP COLUMN paid_cents;
