package engine

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// AverageCalculator implements cost basis calculation using the average cost method.
// Each sale's cost basis is the weighted average price of all shares held at the time of sale.
type AverageCalculator struct{}

func (a *AverageCalculator) Method() string {
	return "average"
}

func (a *AverageCalculator) Calculate(ctx context.Context, txns []Transaction) (*CostBasisResult, error) {
	if len(txns) == 0 {
		return &CostBasisResult{Method: a.Method()}, nil
	}

	sorted := make([]Transaction, len(txns))
	copy(sorted, txns)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	asset := sorted[0].Asset

	// Running totals for the average cost calculation.
	totalQty := decimal.Zero
	totalCost := decimal.Zero

	var consumptions []LotConsumption
	var summaries []SaleSummary

	// Track purchase dates for holding period (use earliest remaining).
	type heldLot struct {
		date time.Time
		qty  decimal.Decimal
	}
	var held []heldLot
	heldIdx := 0

	for _, txn := range sorted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if txn.IsBuy() {
			totalQty = totalQty.Add(txn.Quantity)
			totalCost = totalCost.Add(txn.TotalValue)
			held = append(held, heldLot{date: txn.Date, qty: txn.Quantity})
			continue
		}

		if !txn.IsSell() {
			continue
		}

		sellQty := txn.Quantity.Abs()
		if sellQty.GreaterThan(totalQty) {
			return nil, fmt.Errorf(
				"insufficient holdings: selling %s but only %s held on %s (transaction ID %d)",
				sellQty, totalQty, txn.Date.Format("2006-01-02"), txn.ID,
			)
		}

		// Average cost per unit at time of sale.
		avgCost := totalCost.Div(totalQty)
		saleCostBasis := avgCost.Mul(sellQty)

		consumptions = append(consumptions, LotConsumption{
			SaleTransactionID: txn.ID,
			LotTransactionID:  0, // average method doesn't map to specific lots
			QuantityUsed:      sellQty,
			CostBasis:         saleCostBasis,
		})

		// Determine holding period from earliest held lots.
		allLongTerm := true
		remaining := sellQty
		for remaining.IsPositive() && heldIdx < len(held) {
			lot := &held[heldIdx]
			consumed := decimal.Min(remaining, lot.qty)
			if txn.Date.Sub(lot.date) <= 365*24*time.Hour {
				allLongTerm = false
			}
			lot.qty = lot.qty.Sub(consumed)
			remaining = remaining.Sub(consumed)
			if lot.qty.IsZero() {
				heldIdx++
			}
		}

		proceeds := txn.TotalValue.Abs()
		summaries = append(summaries, SaleSummary{
			TransactionID: txn.ID,
			Date:          txn.Date,
			QuantitySold:  sellQty,
			Proceeds:      proceeds,
			CostBasis:     saleCostBasis,
			GainLoss:      proceeds.Sub(saleCostBasis),
			IsLongTerm:    allLongTerm,
		})

		totalQty = totalQty.Sub(sellQty)
		totalCost = totalCost.Sub(saleCostBasis)
	}

	return &CostBasisResult{
		Asset:         asset,
		Method:        a.Method(),
		Consumptions:  consumptions,
		SaleSummaries: summaries,
	}, nil
}
