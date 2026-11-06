package handlers

import (
	"context"
	"database/sql"

	"github.com/gofiber/fiber/v3"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
	"github.com/LLRHook/swiss-grocery/services/api/internal/optimizer"
)

type ListHandler struct {
	db *sql.DB
}

func NewListHandler(db *sql.DB) *ListHandler {
	return &ListHandler{db: db}
}

func (h *ListHandler) Create(c fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Name == "" {
		body.Name = "My List"
	}

	var id int
	err := h.db.QueryRow(
		`INSERT INTO shopping_lists (name) VALUES ($1) RETURNING id`, body.Name,
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create list"})
	}

	return c.Status(201).JSON(fiber.Map{"id": id, "name": body.Name})
}

func (h *ListHandler) Get(c fiber.Ctx) error {
	id := c.Params("id")

	var list models.ShoppingList
	err := h.db.QueryRow(
		`SELECT id, name, created_at FROM shopping_lists WHERE id = $1`, id,
	).Scan(&list.ID, &list.Name, &list.CreatedAt)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "list not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get list"})
	}

	rows, err := h.db.Query(
		`SELECT id, product_id, name, quantity, unit, checked
		 FROM shopping_list_items WHERE list_id = $1 ORDER BY id`, id,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get list items"})
	}
	defer rows.Close()

	list.Items = []models.ShoppingListItem{}
	for rows.Next() {
		var item models.ShoppingListItem
		if err := rows.Scan(&item.ID, &item.ProductID, &item.Name, &item.Quantity, &item.Unit, &item.Checked); err != nil {
			continue
		}
		list.Items = append(list.Items, item)
	}

	return c.JSON(list)
}

func (h *ListHandler) AddItem(c fiber.Ctx) error {
	listID := c.Params("id")

	var body struct {
		ProductID *int    `json:"product_id"`
		Name      string  `json:"name"`
		Quantity  float64 `json:"quantity"`
		Unit      string  `json:"unit"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name is required"})
	}
	if body.Quantity <= 0 {
		body.Quantity = 1
	}

	var id int
	err := h.db.QueryRow(
		`INSERT INTO shopping_list_items (list_id, product_id, name, quantity, unit)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		listID, body.ProductID, body.Name, body.Quantity, body.Unit,
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to add item"})
	}

	return c.Status(201).JSON(fiber.Map{"id": id})
}

func (h *ListHandler) RemoveItem(c fiber.Ctx) error {
	itemID := c.Params("itemId")

	_, err := h.db.Exec(`DELETE FROM shopping_list_items WHERE id = $1`, itemID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to remove item"})
	}

	return c.JSON(fiber.Map{"deleted": true})
}

// Optimize loads offers and delegates allocation and calculations to Rust.
func (h *ListHandler) Optimize(c fiber.Ctx) error {
	listID := c.Params("id")
	ctx, cancel := context.WithTimeout(c.Context(), optimizer.Timeout)
	defer cancel()

	// Keep SQL row order: equal-price selections retain their first row.
	rows, err := h.db.QueryContext(
		ctx,
		`SELECT sli.name, sli.product_id, s.id, s.name, s.slug, sp.current_price, sp.currency
		 FROM shopping_list_items sli
		 JOIN store_products sp ON sp.product_id = sli.product_id
		 JOIN stores s ON s.id = sp.store_id
		 WHERE sli.list_id = $1 AND sli.product_id IS NOT NULL
		 ORDER BY sli.id, sp.current_price ASC`,
		listID,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to optimize"})
	}
	defer rows.Close()

	var batch optimizer.Batch
	for rows.Next() {
		var row optimizer.Candidate
		var productID int
		var currency string
		if err := rows.Scan(&row.Name, &productID, &row.Store.ID, &row.Store.Name, &row.Store.Slug, &row.Price, &currency); err != nil {
			continue // Preserve the existing scan-error policy.
		}
		if err := batch.Add(row); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": "failed to optimize"})
		}
	}
	if err := rows.Err(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to optimize"})
	}
	// Release SQL resources before starting the single native batch process.
	if err := rows.Close(); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to optimize"})
	}
	result, err := batch.Optimize(ctx)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to optimize"})
	}
	return c.JSON(result)
}
