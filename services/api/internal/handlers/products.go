package handlers

import (
	"database/sql"
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
)

func queryInt(c fiber.Ctx, key string, fallback int) int {
	v := c.Query(key, "")
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

type ProductHandler struct {
	db *sql.DB
}

func NewProductHandler(db *sql.DB) *ProductHandler {
	return &ProductHandler{db: db}
}

// List returns products with optional search query
func (h *ProductHandler) List(c fiber.Ctx) error {
	query := c.Query("q", "")
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)

	var rows *sql.Rows
	var err error

	if query != "" {
		rows, err = h.db.Query(
			`SELECT id, name, brand, category_id, ean, unit, unit_quantity, image_url
			 FROM products
			 WHERE to_tsvector('simple', name) @@ plainto_tsquery('simple', $1)
			 ORDER BY name
			 LIMIT $2 OFFSET $3`,
			query, limit, offset,
		)
	} else {
		rows, err = h.db.Query(
			`SELECT id, name, brand, category_id, ean, unit, unit_quantity, image_url
			 FROM products
			 ORDER BY name
			 LIMIT $1 OFFSET $2`,
			limit, offset,
		)
	}

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to query products"})
	}
	defer rows.Close()

	products := []models.Product{}
	for rows.Next() {
		var p models.Product
		if err := rows.Scan(&p.ID, &p.Name, &p.Brand, &p.CategoryID, &p.EAN, &p.Unit, &p.UnitQuantity, &p.ImageURL); err != nil {
			continue
		}
		products = append(products, p)
	}

	return c.JSON(fiber.Map{"products": products, "count": len(products)})
}

// Get returns a single product by ID
func (h *ProductHandler) Get(c fiber.Ctx) error {
	id := c.Params("id")

	var p models.Product
	err := h.db.QueryRow(
		`SELECT id, name, brand, category_id, ean, unit, unit_quantity, image_url
		 FROM products WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.Brand, &p.CategoryID, &p.EAN, &p.Unit, &p.UnitQuantity, &p.ImageURL)

	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "product not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get product"})
	}

	return c.JSON(p)
}

// GetByBarcode looks up a product by EAN barcode
func (h *ProductHandler) GetByBarcode(c fiber.Ctx) error {
	ean := c.Params("ean")

	var p models.Product
	err := h.db.QueryRow(
		`SELECT id, name, brand, category_id, ean, unit, unit_quantity, image_url
		 FROM products WHERE ean = $1`, ean,
	).Scan(&p.ID, &p.Name, &p.Brand, &p.CategoryID, &p.EAN, &p.Unit, &p.UnitQuantity, &p.ImageURL)

	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "product not found for this barcode"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get product"})
	}

	// Also fetch prices from all stores
	priceRows, err := h.db.Query(
		`SELECT s.id, s.name, s.slug, sp.current_price, sp.currency, sp.in_stock, sp.product_url, sp.last_scraped_at
		 FROM store_products sp
		 JOIN stores s ON s.id = sp.store_id
		 WHERE sp.product_id = $1
		 ORDER BY sp.current_price ASC`, p.ID,
	)
	if err != nil {
		return c.JSON(fiber.Map{"product": p, "prices": []models.StorePrice{}})
	}
	defer priceRows.Close()

	prices := []models.StorePrice{}
	for priceRows.Next() {
		var sp models.StorePrice
		if err := priceRows.Scan(&sp.Store.ID, &sp.Store.Name, &sp.Store.Slug, &sp.Price, &sp.Currency, &sp.InStock, &sp.ProductURL, &sp.LastScraped); err != nil {
			continue
		}
		prices = append(prices, sp)
	}

	return c.JSON(fiber.Map{"product": p, "prices": prices})
}

// ComparePrice returns prices across all stores for a product
func (h *ProductHandler) ComparePrice(c fiber.Ctx) error {
	id := c.Params("id")

	rows, err := h.db.Query(
		`SELECT s.id, s.name, s.slug, s.website_url, sp.current_price, sp.currency, sp.in_stock, sp.product_url, sp.last_scraped_at
		 FROM store_products sp
		 JOIN stores s ON s.id = sp.store_id
		 WHERE sp.product_id = $1
		 ORDER BY sp.current_price ASC`, id,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get prices"})
	}
	defer rows.Close()

	prices := []models.StorePrice{}
	for rows.Next() {
		var sp models.StorePrice
		if err := rows.Scan(&sp.Store.ID, &sp.Store.Name, &sp.Store.Slug, &sp.Store.WebsiteURL, &sp.Price, &sp.Currency, &sp.InStock, &sp.ProductURL, &sp.LastScraped); err != nil {
			continue
		}
		prices = append(prices, sp)
	}

	return c.JSON(fiber.Map{"prices": prices})
}

// PriceHistory returns historical prices for a product
func (h *ProductHandler) PriceHistory(c fiber.Ctx) error {
	id := c.Params("id")
	days := queryInt(c, "days", 30)

	rows, err := h.db.Query(
		`SELECT ph.price, ph.recorded_at, s.name
		 FROM price_history ph
		 JOIN store_products sp ON sp.id = ph.store_product_id
		 JOIN stores s ON s.id = sp.store_id
		 WHERE sp.product_id = $1
		   AND ph.recorded_at >= now() - make_interval(days => $2)
		 ORDER BY ph.recorded_at ASC`,
		id, days,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get price history"})
	}
	defer rows.Close()

	history := []models.PricePoint{}
	for rows.Next() {
		var pp models.PricePoint
		if err := rows.Scan(&pp.Price, &pp.RecordedAt, &pp.StoreName); err != nil {
			continue
		}
		history = append(history, pp)
	}

	return c.JSON(fiber.Map{"history": history})
}
