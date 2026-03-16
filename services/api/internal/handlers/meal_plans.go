package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v3"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
)

type MealPlanHandler struct {
	db *sql.DB
}

func NewMealPlanHandler(db *sql.DB) *MealPlanHandler {
	return &MealPlanHandler{db: db}
}

func (h *MealPlanHandler) Create(c fiber.Ctx) error {
	var body struct {
		WeekStart string `json:"week_start"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.WeekStart == "" {
		return c.Status(400).JSON(fiber.Map{"error": "week_start is required (YYYY-MM-DD)"})
	}

	var id int
	err := h.db.QueryRow(
		`INSERT INTO meal_plans (week_start) VALUES ($1) RETURNING id`, body.WeekStart,
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create meal plan"})
	}

	return c.Status(201).JSON(fiber.Map{"id": id, "week_start": body.WeekStart})
}

func (h *MealPlanHandler) Get(c fiber.Ctx) error {
	id := c.Params("id")

	var plan models.MealPlan
	err := h.db.QueryRow(
		`SELECT id, week_start FROM meal_plans WHERE id = $1`, id,
	).Scan(&plan.ID, &plan.WeekStart)
	if err == sql.ErrNoRows {
		return c.Status(404).JSON(fiber.Map{"error": "meal plan not found"})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to get meal plan"})
	}

	rows, err := h.db.Query(
		`SELECT mpe.id, mpe.recipe_id, r.name, r.servings, mpe.day_of_week, mpe.meal_type, mpe.servings
		 FROM meal_plan_entries mpe
		 JOIN recipes r ON r.id = mpe.recipe_id
		 WHERE mpe.meal_plan_id = $1
		 ORDER BY mpe.day_of_week, mpe.meal_type`, id,
	)
	if err != nil {
		return c.JSON(plan)
	}
	defer rows.Close()

	plan.Entries = []models.MealPlanEntry{}
	for rows.Next() {
		var e models.MealPlanEntry
		if err := rows.Scan(&e.ID, &e.RecipeID, &e.Recipe.Name, &e.Recipe.Servings, &e.DayOfWeek, &e.MealType, &e.Servings); err != nil {
			continue
		}
		plan.Entries = append(plan.Entries, e)
	}

	return c.JSON(plan)
}

func (h *MealPlanHandler) AddEntry(c fiber.Ctx) error {
	planID := c.Params("id")

	var body struct {
		RecipeID  int    `json:"recipe_id"`
		DayOfWeek int    `json:"day_of_week"`
		MealType  string `json:"meal_type"`
		Servings  int    `json:"servings"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if body.Servings <= 0 {
		body.Servings = 2
	}

	var id int
	err := h.db.QueryRow(
		`INSERT INTO meal_plan_entries (meal_plan_id, recipe_id, day_of_week, meal_type, servings)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		planID, body.RecipeID, body.DayOfWeek, body.MealType, body.Servings,
	).Scan(&id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to add entry"})
	}

	return c.Status(201).JSON(fiber.Map{"id": id})
}

// GenerateList creates a shopping list from all recipe ingredients in the meal plan
func (h *MealPlanHandler) GenerateList(c fiber.Ctx) error {
	planID := c.Params("id")

	// Create a new shopping list
	var listID int
	err := h.db.QueryRow(
		`INSERT INTO shopping_lists (name)
		 VALUES ('Meal Plan Shopping List') RETURNING id`,
	).Scan(&listID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to create shopping list"})
	}

	// Gather all ingredients from all recipes in the plan
	_, err = h.db.Exec(
		`INSERT INTO shopping_list_items (list_id, product_id, name, quantity, unit)
		 SELECT $1, ri.product_id, ri.name, SUM(ri.quantity * mpe.servings / r.servings), ri.unit
		 FROM meal_plan_entries mpe
		 JOIN recipes r ON r.id = mpe.recipe_id
		 JOIN recipe_ingredients ri ON ri.recipe_id = r.id
		 WHERE mpe.meal_plan_id = $2 AND ri.optional = false
		 GROUP BY ri.product_id, ri.name, ri.unit`,
		listID, planID,
	)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "failed to generate list items"})
	}

	return c.Status(201).JSON(fiber.Map{"list_id": listID})
}
