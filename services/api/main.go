package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"

	"github.com/LLRHook/swiss-grocery/services/api/internal/config"
	"github.com/LLRHook/swiss-grocery/services/api/internal/database"
	"github.com/LLRHook/swiss-grocery/services/api/internal/handlers"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	app := fiber.New(fiber.Config{
		AppName: "Swiss Grocery API",
	})

	app.Use(logger.New())
	app.Use(cors.New())

	// Health check
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// API routes
	api := app.Group("/api/v1")

	productHandler := handlers.NewProductHandler(db)
	api.Get("/products", productHandler.List)
	api.Get("/products/:id", productHandler.Get)
	api.Get("/products/barcode/:ean", productHandler.GetByBarcode)
	api.Get("/products/:id/prices", productHandler.ComparePrice)
	api.Get("/products/:id/history", productHandler.PriceHistory)

	listHandler := handlers.NewListHandler(db)
	api.Post("/lists", listHandler.Create)
	api.Get("/lists/:id", listHandler.Get)
	api.Post("/lists/:id/items", listHandler.AddItem)
	api.Delete("/lists/:id/items/:itemId", listHandler.RemoveItem)
	api.Get("/lists/:id/optimize", listHandler.Optimize)

	recipeHandler := handlers.NewRecipeHandler(db)
	api.Get("/recipes", recipeHandler.List)
	api.Get("/recipes/:id", recipeHandler.Get)

	mealPlanHandler := handlers.NewMealPlanHandler(db)
	api.Post("/meal-plans", mealPlanHandler.Create)
	api.Get("/meal-plans/:id", mealPlanHandler.Get)
	api.Post("/meal-plans/:id/entries", mealPlanHandler.AddEntry)
	api.Post("/meal-plans/:id/generate-list", mealPlanHandler.GenerateList)

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on :%s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("failed to start server: %v", err)
		os.Exit(1)
	}
}
