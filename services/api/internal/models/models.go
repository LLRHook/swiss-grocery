package models

import "time"

type Store struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	WebsiteURL string `json:"website_url,omitempty"`
	LogoURL    string `json:"logo_url,omitempty"`
}

type Product struct {
	ID           int      `json:"id"`
	Name         string   `json:"name"`
	Brand        string   `json:"brand,omitempty"`
	CategoryID   *int     `json:"category_id,omitempty"`
	EAN          string   `json:"ean,omitempty"`
	Unit         string   `json:"unit"`
	UnitQuantity *float64 `json:"unit_quantity,omitempty"`
	ImageURL     string   `json:"image_url,omitempty"`
}

type StorePrice struct {
	Store       Store   `json:"store"`
	Price       float64 `json:"price"`
	Currency    string  `json:"currency"`
	InStock     bool    `json:"in_stock"`
	ProductURL  string  `json:"product_url,omitempty"`
	LastScraped string  `json:"last_scraped_at"`
}

type PricePoint struct {
	Price      float64   `json:"price"`
	RecordedAt time.Time `json:"recorded_at"`
	StoreName  string    `json:"store_name"`
}

type ShoppingList struct {
	ID        int                `json:"id"`
	Name      string             `json:"name"`
	Items     []ShoppingListItem `json:"items"`
	CreatedAt time.Time          `json:"created_at"`
}

type ShoppingListItem struct {
	ID        int      `json:"id"`
	ProductID *int     `json:"product_id,omitempty"`
	Name      string   `json:"name"`
	Quantity  float64  `json:"quantity"`
	Unit      string   `json:"unit,omitempty"`
	Checked   bool     `json:"checked"`
}

type OptimizedPlan struct {
	TotalCost   float64          `json:"total_cost"`
	Currency    string           `json:"currency"`
	Savings     float64          `json:"savings"`
	StorePlans  []StorePlan      `json:"store_plans"`
}

type StorePlan struct {
	Store    Store               `json:"store"`
	Items    []OptimizedItem     `json:"items"`
	Subtotal float64             `json:"subtotal"`
}

type OptimizedItem struct {
	Name  string  `json:"name"`
	Price float64 `json:"price"`
	Unit  string  `json:"unit,omitempty"`
}

type Recipe struct {
	ID           int                `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Servings     int                `json:"servings"`
	PrepMinutes  *int               `json:"prep_minutes,omitempty"`
	CookMinutes  *int               `json:"cook_minutes,omitempty"`
	ImageURL     string             `json:"image_url,omitempty"`
	Ingredients  []RecipeIngredient `json:"ingredients,omitempty"`
}

type RecipeIngredient struct {
	ID        int      `json:"id"`
	ProductID *int     `json:"product_id,omitempty"`
	Name      string   `json:"name"`
	Quantity  *float64 `json:"quantity,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Optional  bool     `json:"optional"`
}

type MealPlan struct {
	ID        int             `json:"id"`
	WeekStart string          `json:"week_start"`
	Entries   []MealPlanEntry `json:"entries"`
}

type MealPlanEntry struct {
	ID        int    `json:"id"`
	RecipeID  int    `json:"recipe_id"`
	Recipe    Recipe `json:"recipe,omitempty"`
	DayOfWeek int    `json:"day_of_week"`
	MealType  string `json:"meal_type"`
	Servings  int    `json:"servings"`
}
