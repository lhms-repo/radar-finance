-- Categories and subcategories (RF-50, RN-18), seeded with the list of section 6.
-- Two levels only: a top level category has an icon (section 5.6), a
-- subcategory points to its parent and classifies entries. Entries reference
-- categories without ON DELETE, so SQLite refuses to delete one in use (RN-18).

-- +goose Up
CREATE TABLE categories (
    id         INTEGER  PRIMARY KEY,
    parent_id  INTEGER  REFERENCES categories (id),
    name       TEXT     NOT NULL CHECK (length(trim(name)) > 0 AND length(name) <= 60),
    icon       TEXT     CHECK (icon IS NULL OR length(icon) BETWEEN 1 AND 40),
    active     BOOLEAN  NOT NULL DEFAULT 1,
    sort_order INTEGER  NOT NULL DEFAULT 0,
    -- NULL only for the initial list loaded by this migration (RF-03).
    created_by INTEGER  REFERENCES users (id),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CHECK (parent_id IS NOT NULL OR icon IS NOT NULL),
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE INDEX categories_parent_idx ON categories (parent_id);

INSERT INTO categories (id, parent_id, name, icon, sort_order, created_at, updated_at) VALUES
    (1, NULL, 'Moradia', 'house', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (2, NULL, 'Alimentação', 'utensils', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, NULL, 'Carro', 'car', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (4, NULL, 'Transporte', 'bus', 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (5, NULL, 'Saúde', 'heart-pulse', 5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (6, NULL, 'Cuidados pessoais', 'scissors', 6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (7, NULL, 'Assinaturas', 'repeat', 7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (8, NULL, 'Lazer', 'ticket', 8, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (9, NULL, 'Compras', 'shopping-bag', 9, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (10, NULL, 'Educação', 'graduation-cap', 10, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (11, NULL, 'Pets', 'paw-print', 11, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (12, NULL, 'Financeiro', 'landmark', 12, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (13, NULL, 'Outros', 'ellipsis', 13, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

INSERT INTO categories (parent_id, name, sort_order, created_at, updated_at) VALUES
    (1, 'Financiamento da casa', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (1, 'Lote', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (1, 'Energia', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (1, 'Energia solar', 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (1, 'Água', 5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (1, 'Internet', 6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (1, 'Manutenção e reparos', 7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (1, 'Móveis e utilidades', 8, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (2, 'Compras para casa', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (2, 'Delivery', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (2, 'Restaurantes e lanches', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, 'Financiamento', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, 'Combustível', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, 'Mecânico', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, 'Seguro', 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, 'IPVA e licenciamento', 5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, 'Estacionamento e pedágio', 6, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (3, 'Lavagem', 7, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (4, 'Aplicativos de corrida', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (4, 'Transporte público', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (5, 'Farmácia', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (5, 'Consultas e exames', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (5, 'Plano de saúde', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (5, 'Academia', 4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (6, 'Barbearia e salão', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (6, 'Cosméticos e higiene', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (7, 'Streaming', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (7, 'Software e serviços digitais', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (7, 'Telefonia', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (8, 'Passeios e eventos', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (8, 'Viagens', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (8, 'Hobbies', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (9, 'Roupas e calçados', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (9, 'Eletrônicos', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (9, 'Presentes', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (10, 'Cursos', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (10, 'Livros e material', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (11, 'Ração', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (11, 'Veterinário', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (11, 'Petshop', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (12, 'Tarifas bancárias', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (12, 'Juros e multas', 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (12, 'Impostos', 3, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
    (13, 'Sem categoria definida', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);

-- +goose Down
DROP TABLE categories;
