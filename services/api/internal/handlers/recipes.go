package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v3"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
)

type RecipeHandler struct {
	db *sql.DB
}

func NewRecipeHandler(db *sql.DB) *RecipeHandler {
	return &RecipeHandler{db: db}
}

func (h *RecipeHandler) List(c fiber.Ctx) error {
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)

	rows, err := h.db.Query(
		`SELECT id, name, description, servings, prep_minutes, cook_minutes, image_url
		 FROM recipes ORDER BY name LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to query recipes"})
	}
	defer rows.Close()

	recipes := []models.Recipe{}
	for rows.Next() {
		var r models.Recipe
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Servings, &r.PrepMinutes, &r.CookMinutes, &r.ImageURL); err != nil {
			continue
		}
		recipes = append(recipes, r)
	}

	return c.JSON(fiber.Map{"recipes": recipes, "count": len(recipes)})
}

func (h *RecipeHandler) Get(c fiber.Ctx) error {
	id := c.Params("id")

	var r models.Recipe
	err := h.db.QueryRow(
		`SELECT id, name, description, servings, prep_minutes, cook_minutes, image_url
		 FROM recipes WHERE id = $1`, id,
	).Scan(&r.ID, &r.Name, &r.Description, &r.Servings, &r.PrepMinutes, &r.CookMinutes, &r.ImageURL)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "recipe not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get recipe"})
	}

	// Fetch ingredients
	rows, err := h.db.Query(
		`SELECT id, product_id, name, quantity, unit, optional
		 FROM recipe_ingredients WHERE recipe_id = $1 ORDER BY id`, id,
	)
	if err != nil {
		return c.JSON(r)
	}
	defer rows.Close()

	r.Ingredients = []models.RecipeIngredient{}
	for rows.Next() {
		var ing models.RecipeIngredient
		if err := rows.Scan(&ing.ID, &ing.ProductID, &ing.Name, &ing.Quantity, &ing.Unit, &ing.Optional); err != nil {
			continue
		}
		r.Ingredients = append(r.Ingredients, ing)
	}

	return c.JSON(r)
}
