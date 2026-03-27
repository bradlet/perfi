package engine

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// FIFOCalculator implements cost basis calculation using the First-In-First-Out method.
// The earliest purchased lots are consumed first when processing sales.
type FIFOCalculator struct{}

func (f *FIFOCalculator) Method() string {
	return "fifo"
}

func (f *FIFOCalculator) Calculate(ctx context.Context, txns []Transaction) (*CostBasisResult, error) {
	if len(txns) == 0 {
		return &CostBasisResult{Method: f.Method()}, nil
	}

	// Sort transactions chronologically; stable sort preserves original order for same-date txns.
	sorted := make([]Transaction, len(txns))
	copy(sorted, txns)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Date.Before(sorted[j].Date)
	})

	asset := sorted[0].Asset

	var lots []Lot
	var consumptions []LotConsumption
	lotIdx := 0

	for _, txn := range sorted {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if txn.IsBuy() {
			lots = append(lots, Lot{
				TransactionID: txn.ID,
				Date:          txn.Date,
				OriginalQty:   txn.Quantity,
				RemainingQty:  txn.Quantity,
				PricePerUnit:  txn.PricePerUnit,
			})
			continue
		}

		if !txn.IsSell() {
			// Zero-quantity transactions are skipped.
			continue
		}

		remaining := txn.Quantity.Abs()
		for remaining.IsPositive() && lotIdx < len(lots) {
			lot := &lots[lotIdx]
			consumed := decimal.Min(remaining, lot.RemainingQty)
			lot.RemainingQty = lot.RemainingQty.Sub(consumed)

			consumptions = append(consumptions, LotConsumption{
				SaleTransactionID: txn.ID,
				LotTransactionID:  lot.TransactionID,
				QuantityUsed:      consumed,
				CostBasis:         lot.PricePerUnit.Mul(consumed),
			})

			remaining = remaining.Sub(consumed)
			if lot.RemainingQty.IsZero() {
				lotIdx++
			}
		}

		if remaining.IsPositive() {
			return nil, fmt.Errorf(
				"insufficient lots: %s units unsatisfied for sale on %s (transaction ID %d)",
				remaining, txn.Date.Format("2006-01-02"), txn.ID,
			)
		}
	}

	summaries := buildSaleSummaries(sorted, consumptions, lots)

	return &CostBasisResult{
		Asset:         asset,
		Method:        f.Method(),
		Consumptions:  consumptions,
		SaleSummaries: summaries,
	}, nil
}

// buildSaleSummaries aggregates lot consumptions into per-sale summaries.
func buildSaleSummaries(txns []Transaction, consumptions []LotConsumption, lots []Lot) []SaleSummary {
	// Index lots by transaction ID for quick date lookup.
	lotByTxnID := make(map[int64]Lot, len(lots))
	for _, lot := range lots {
		lotByTxnID[lot.TransactionID] = lot
	}

	// Group consumptions by sale transaction ID.
	grouped := make(map[int64][]LotConsumption)
	for _, c := range consumptions {
		grouped[c.SaleTransactionID] = append(grouped[c.SaleTransactionID], c)
	}

	var summaries []SaleSummary
	for _, txn := range txns {
		cs, ok := grouped[txn.ID]
		if !ok {
			continue
		}

		totalCostBasis := decimal.Zero
		allLongTerm := true
		for _, c := range cs {
			totalCostBasis = totalCostBasis.Add(c.CostBasis)
			lot := lotByTxnID[c.LotTransactionID]
			if txn.Date.Sub(lot.Date) <= 365*24*time.Hour {
				allLongTerm = false
			}
		}

		proceeds := txn.TotalValue.Abs()
		summaries = append(summaries, SaleSummary{
			TransactionID: txn.ID,
			Date:          txn.Date,
			QuantitySold:  txn.Quantity.Abs(),
			Proceeds:      proceeds,
			CostBasis:     totalCostBasis,
			GainLoss:      proceeds.Sub(totalCostBasis),
			IsLongTerm:    allLongTerm,
		})
	}

	return summaries
}
