package handlers

import (
	"database/sql"
	"math"

	"github.com/gofiber/fiber/v3"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
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

// Optimize finds the cheapest store allocation for list items
func (h *ListHandler) Optimize(c fiber.Ctx) error {
	listID := c.Params("id")

	// Get all items that have a linked product
	rows, err := h.db.Query(
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

	// Group: for each item, find cheapest store
	type itemPrice struct {
		itemName string
		storeID  int
		store    models.Store
		price    float64
		currency string
	}

	bestPrices := map[string]itemPrice{} // keyed by item name
	var worstTotal float64
	worstPrices := map[string]float64{}

	for rows.Next() {
		var ip itemPrice
		var productID int
		if err := rows.Scan(&ip.itemName, &productID, &ip.store.ID, &ip.store.Name, &ip.store.Slug, &ip.price, &ip.currency); err != nil {
			continue
		}

		// Track worst price per item for savings calc
		if existing, ok := worstPrices[ip.itemName]; !ok || ip.price > existing {
			worstPrices[ip.itemName] = ip.price
		}

		// Keep cheapest per item
		if existing, ok := bestPrices[ip.itemName]; !ok || ip.price < existing.price {
			bestPrices[ip.itemName] = ip
		}
	}

	// Build store plans
	storePlans := map[int]*models.StorePlan{}
	var totalCost float64

	for _, bp := range bestPrices {
		if _, ok := storePlans[bp.storeID]; !ok {
			storePlans[bp.storeID] = &models.StorePlan{
				Store: bp.store,
				Items: []models.OptimizedItem{},
			}
		}
		storePlans[bp.storeID].Items = append(storePlans[bp.storeID].Items, models.OptimizedItem{
			Name:  bp.itemName,
			Price: bp.price,
		})
		storePlans[bp.storeID].Subtotal += bp.price
		totalCost += bp.price
	}

	for _, wp := range worstPrices {
		worstTotal += wp
	}

	plans := []models.StorePlan{}
	for _, sp := range storePlans {
		sp.Subtotal = math.Round(sp.Subtotal*100) / 100
		plans = append(plans, *sp)
	}

	result := models.OptimizedPlan{
		TotalCost:  math.Round(totalCost*100) / 100,
		Currency:   "CHF",
		Savings:    math.Round((worstTotal-totalCost)*100) / 100,
		StorePlans: plans,
	}

	return c.JSON(result)
}
