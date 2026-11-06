package handlers

import (
	"math"
	"sort"

	"github.com/LLRHook/swiss-grocery/services/api/internal/models"
	"github.com/LLRHook/swiss-grocery/services/api/internal/optimizer"
)

// Frozen test-only reference of the pre-port Go selection/aggregation algorithm.
// repairStoreID changes ONLY the demonstrated unpopulated storeID defect.
// Sorted name/store iteration replaces the legacy random map iteration so both
// the reference and port have a defined addition/output order. Keep this code
// independent of the Rust implementation and production transport decoder.
func legacyReference(rows []optimizer.Candidate, repairStoreID bool) models.OptimizedPlan {
	type itemPrice struct {
		itemName string
		storeID  int
		store    models.Store
		price    float64
	}
	bestPrices := map[string]itemPrice{}
	worstPrices := map[string]float64{}
	for _, row := range rows {
		ip := itemPrice{itemName: row.Name, store: row.Store, price: row.Price}
		if repairStoreID {
			ip.storeID = ip.store.ID
		}
		if existing, ok := worstPrices[ip.itemName]; !ok || ip.price > existing {
			worstPrices[ip.itemName] = ip.price
		}
		if existing, ok := bestPrices[ip.itemName]; !ok || ip.price < existing.price {
			bestPrices[ip.itemName] = ip
		}
	}

	names := make([]string, 0, len(bestPrices))
	for name := range bestPrices {
		names = append(names, name)
	}
	sort.Strings(names)
	storePlans := map[int]*models.StorePlan{}
	var totalCost float64
	for _, name := range names {
		bp := bestPrices[name]
		if _, ok := storePlans[bp.storeID]; !ok {
			storePlans[bp.storeID] = &models.StorePlan{Store: bp.store, Items: []models.OptimizedItem{}}
		}
		storePlans[bp.storeID].Items = append(storePlans[bp.storeID].Items, models.OptimizedItem{Name: bp.itemName, Price: bp.price})
		storePlans[bp.storeID].Subtotal += bp.price
		totalCost += bp.price
	}
	var worstTotal float64
	for _, name := range names {
		worstTotal += worstPrices[name]
	}
	ids := make([]int, 0, len(storePlans))
	for id := range storePlans {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	plans := []models.StorePlan{}
	for _, id := range ids {
		sp := storePlans[id]
		sp.Subtotal = math.Round(sp.Subtotal*100) / 100
		plans = append(plans, *sp)
	}
	return models.OptimizedPlan{
		TotalCost:  math.Round(totalCost*100) / 100,
		Currency:   "CHF",
		Savings:    math.Round((worstTotal-totalCost)*100) / 100,
		StorePlans: plans,
	}
}
