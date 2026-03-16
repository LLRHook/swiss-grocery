import os

import psycopg2
from psycopg2.extras import execute_values


def get_connection():
    return psycopg2.connect(
        os.getenv(
            "DATABASE_URL",
            "postgres://postgres:postgres@localhost:5432/swissgrocery",
        )
    )


def upsert_products(conn, products: list[dict]):
    """Upsert canonical products by EAN or name+brand."""
    with conn.cursor() as cur:
        for p in products:
            if p.get("ean"):
                cur.execute(
                    """
                    INSERT INTO products (name, brand, ean, unit, unit_quantity, image_url)
                    VALUES (%(name)s, %(brand)s, %(ean)s, %(unit)s, %(unit_quantity)s, %(image_url)s)
                    ON CONFLICT (ean) WHERE ean IS NOT NULL
                    DO UPDATE SET name = EXCLUDED.name, brand = EXCLUDED.brand,
                                  unit = EXCLUDED.unit, unit_quantity = EXCLUDED.unit_quantity,
                                  image_url = EXCLUDED.image_url, updated_at = now()
                    RETURNING id
                    """,
                    p,
                )
            else:
                cur.execute(
                    """
                    INSERT INTO products (name, brand, unit, unit_quantity, image_url)
                    VALUES (%(name)s, %(brand)s, %(unit)s, %(unit_quantity)s, %(image_url)s)
                    RETURNING id
                    """,
                    p,
                )
            p["id"] = cur.fetchone()[0]
    conn.commit()


def upsert_store_prices(conn, store_slug: str, prices: list[dict]):
    """Upsert store-specific prices and append to price history."""
    with conn.cursor() as cur:
        cur.execute("SELECT id FROM stores WHERE slug = %s", (store_slug,))
        row = cur.fetchone()
        if not row:
            return
        store_id = row[0]

        for p in prices:
            cur.execute(
                """
                INSERT INTO store_products (product_id, store_id, external_id, current_price, product_url, last_scraped_at)
                VALUES (%(product_id)s, %(store_id)s, %(external_id)s, %(price)s, %(product_url)s, now())
                ON CONFLICT (product_id, store_id)
                DO UPDATE SET current_price = EXCLUDED.current_price,
                              product_url = EXCLUDED.product_url,
                              last_scraped_at = now()
                RETURNING id
                """,
                {**p, "store_id": store_id},
            )
            store_product_id = cur.fetchone()[0]

            # Append to price history
            cur.execute(
                """
                INSERT INTO price_history (store_product_id, price)
                VALUES (%s, %s)
                """,
                (store_product_id, p["price"]),
            )
    conn.commit()
