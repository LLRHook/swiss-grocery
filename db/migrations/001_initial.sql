-- Stores (Migros, Coop, Aldi, Lidl, Denner, etc.)
CREATE TABLE IF NOT EXISTS stores (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    slug            TEXT NOT NULL UNIQUE,
    website_url     TEXT,
    logo_url        TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Store locations
CREATE TABLE IF NOT EXISTS store_locations (
    id              SERIAL PRIMARY KEY,
    store_id        INT NOT NULL REFERENCES stores(id),
    address         TEXT NOT NULL,
    city            TEXT NOT NULL,
    canton          TEXT NOT NULL,
    postal_code     TEXT NOT NULL,
    latitude        DOUBLE PRECISION,
    longitude       DOUBLE PRECISION,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Product categories (hierarchical)
CREATE TABLE IF NOT EXISTS categories (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    parent_id       INT REFERENCES categories(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Canonical products (store-agnostic)
CREATE TABLE IF NOT EXISTS products (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    brand           TEXT,
    category_id     INT REFERENCES categories(id),
    ean             TEXT,                     -- barcode
    unit            TEXT NOT NULL DEFAULT 'piece', -- piece, kg, liter, etc.
    unit_quantity   NUMERIC(10,3),            -- e.g. 0.5 for 500ml
    image_url       TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_products_ean ON products(ean) WHERE ean IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_products_name ON products USING gin(to_tsvector('simple', name));

-- Store-specific product listings with current price
CREATE TABLE IF NOT EXISTS store_products (
    id              SERIAL PRIMARY KEY,
    product_id      INT NOT NULL REFERENCES products(id),
    store_id        INT NOT NULL REFERENCES stores(id),
    external_id     TEXT,                     -- store's internal product ID
    current_price   NUMERIC(10,2) NOT NULL,
    currency        TEXT NOT NULL DEFAULT 'CHF',
    product_url     TEXT,
    in_stock        BOOLEAN NOT NULL DEFAULT true,
    last_scraped_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(product_id, store_id)
);

-- Price history (append-only for trends)
CREATE TABLE IF NOT EXISTS price_history (
    id              SERIAL PRIMARY KEY,
    store_product_id INT NOT NULL REFERENCES store_products(id),
    price           NUMERIC(10,2) NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_price_history_product_date
    ON price_history(store_product_id, recorded_at DESC);

-- Recipes for meal planning
CREATE TABLE IF NOT EXISTS recipes (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT,
    servings        INT NOT NULL DEFAULT 2,
    prep_minutes    INT,
    cook_minutes    INT,
    image_url       TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Recipe ingredients (link recipes to products)
CREATE TABLE IF NOT EXISTS recipe_ingredients (
    id              SERIAL PRIMARY KEY,
    recipe_id       INT NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    product_id      INT REFERENCES products(id),
    name            TEXT NOT NULL,             -- display name (e.g. "2 large eggs")
    quantity        NUMERIC(10,3),
    unit            TEXT,                      -- g, ml, piece, etc.
    optional        BOOLEAN NOT NULL DEFAULT false
);

-- Shopping lists
CREATE TABLE IF NOT EXISTS shopping_lists (
    id              SERIAL PRIMARY KEY,
    name            TEXT NOT NULL DEFAULT 'My List',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS shopping_list_items (
    id              SERIAL PRIMARY KEY,
    list_id         INT NOT NULL REFERENCES shopping_lists(id) ON DELETE CASCADE,
    product_id      INT REFERENCES products(id),
    name            TEXT NOT NULL,
    quantity        NUMERIC(10,3) NOT NULL DEFAULT 1,
    unit            TEXT,
    checked         BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Meal plans (weekly planner)
CREATE TABLE IF NOT EXISTS meal_plans (
    id              SERIAL PRIMARY KEY,
    week_start      DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS meal_plan_entries (
    id              SERIAL PRIMARY KEY,
    meal_plan_id    INT NOT NULL REFERENCES meal_plans(id) ON DELETE CASCADE,
    recipe_id       INT NOT NULL REFERENCES recipes(id),
    day_of_week     INT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6), -- 0=Monday
    meal_type       TEXT NOT NULL CHECK (meal_type IN ('breakfast', 'lunch', 'dinner', 'snack')),
    servings        INT NOT NULL DEFAULT 2
);

-- Seed Swiss grocery stores
INSERT INTO stores (name, slug, website_url) VALUES
    ('Migros',  'migros',  'https://www.migros.ch'),
    ('Coop',    'coop',    'https://www.coop.ch'),
    ('Aldi',    'aldi',    'https://www.aldi-now.ch'),
    ('Lidl',    'lidl',    'https://www.lidl.ch'),
    ('Denner',  'denner',  'https://www.denner.ch')
ON CONFLICT (slug) DO NOTHING;
