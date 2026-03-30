package engine_test

import (
	"context"
	"encoding/csv"
	"os"
	"testing"
	"time"

	"github.com/bradlet/perfi/internal/engine"
	"github.com/bradlet/perfi/internal/storage"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseCSVTransactions reads a CSV file and returns engine.Transaction values.
// Expected columns: Source, Date (YYYY-MM-DD), Quantity, PricePerUnit, TotalValue.
func parseCSVTransactions(t *testing.T, path, asset string) []engine.Transaction {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	require.NoError(t, err)

	var txns []engine.Transaction
	for i, row := range records {
		if i == 0 {
			continue // skip header
		}
		require.Len(t, row, 5, "row %d should have 5 columns", i+1)

		date, err := time.Parse("2006-01-02", row[1])
		require.NoError(t, err, "row %d date", i+1)

		qty, err := decimal.NewFromString(row[2])
		require.NoError(t, err, "row %d quantity", i+1)

		price, err := decimal.NewFromString(row[3])
		require.NoError(t, err, "row %d price", i+1)

		total, err := decimal.NewFromString(row[4])
		require.NoError(t, err, "row %d total", i+1)

		txns = append(txns, engine.Transaction{
			Source:       row[0],
			Date:         date,
			Asset:        asset,
			Quantity:     qty,
			PricePerUnit: price,
			TotalValue:   total,
		})
	}
	return txns
}

func TestIntegration_CSVToFIFOCostBasis(t *testing.T) {
	txns := parseCSVTransactions(t, "../../testdata/transactions.csv", "SOL")
	require.Len(t, txns, 5, "expected 3 buys + 2 sells")

	// Store transactions in SQLite.
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()
	require.NoError(t, store.Init(ctx))
	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))

	// Reload to get database IDs.
	storedTxns, err := store.GetTransactions(ctx, "SOL")
	require.NoError(t, err)
	require.Len(t, storedTxns, 5)

	// Run FIFO calculation.
	calc, err := engine.NewCalculator("fifo")
	require.NoError(t, err)

	result, err := calc.Calculate(ctx, storedTxns)
	require.NoError(t, err)

	// Save and reload results.
	require.NoError(t, store.SaveResults(ctx, result))

	assert.Equal(t, "SOL", result.Asset)
	assert.Equal(t, "fifo", result.Method)
	require.Len(t, result.SaleSummaries, 2, "two sell transactions in the CSV")

	// Sale 1: sell 12 on 2024-03-01 @ $150 = $1800 proceeds.
	// FIFO: 10 @ $100 (lot 1) + 2 @ $110 (lot 2) = $1000 + $220 = $1220 cost basis.
	// Gain: $1800 - $1220 = $580.
	s1 := result.SaleSummaries[0]
	assert.Equal(t, "2024-03-01", s1.Date.Format("2006-01-02"))
	assert.True(t, decimal.RequireFromString("12").Equal(s1.QuantitySold))
	assert.True(t, decimal.RequireFromString("1800").Equal(s1.Proceeds))
	assert.True(t, decimal.RequireFromString("1220").Equal(s1.CostBasis))
	assert.True(t, decimal.RequireFromString("580").Equal(s1.GainLoss))
	assert.False(t, s1.IsLongTerm, "held < 365 days")

	// Sale 2: sell 3 on 2024-06-15 @ $130 = $390 proceeds.
	// FIFO: 3 remaining from lot 2 (5 - 2 = 3 left) @ $110 = $330 cost basis.
	// Gain: $390 - $330 = $60.
	s2 := result.SaleSummaries[1]
	assert.Equal(t, "2024-06-15", s2.Date.Format("2006-01-02"))
	assert.True(t, decimal.RequireFromString("3").Equal(s2.QuantitySold))
	assert.True(t, decimal.RequireFromString("390").Equal(s2.Proceeds))
	assert.True(t, decimal.RequireFromString("330").Equal(s2.CostBasis))
	assert.True(t, decimal.RequireFromString("60").Equal(s2.GainLoss))
	assert.False(t, s2.IsLongTerm)
}

func TestIntegration_CSVToAverageCostBasis(t *testing.T) {
	txns := parseCSVTransactions(t, "../../testdata/transactions.csv", "SOL")

	ctx := context.Background()
	calc, err := engine.NewCalculator("average")
	require.NoError(t, err)

	result, err := calc.Calculate(ctx, txns)
	require.NoError(t, err)

	require.Len(t, result.SaleSummaries, 2)

	// Buys: 10@100 + 5@110 + 8@120 = 23 units, $2510 total.
	// Average cost = $2510 / 23 = $109.130434...
	// Sale 1: sell 12 @ $150 = $1800 proceeds.
	// Cost basis: 12 * $109.130434... = $1309.565217...
	s1 := result.SaleSummaries[0]
	assert.True(t, decimal.RequireFromString("12").Equal(s1.QuantitySold))
	assert.True(t, decimal.RequireFromString("1800").Equal(s1.Proceeds))
	// Average cost basis should be approximately $1309.57.
	assert.True(t, s1.CostBasis.GreaterThan(decimal.RequireFromString("1309")))
	assert.True(t, s1.CostBasis.LessThan(decimal.RequireFromString("1310")))
	assert.True(t, s1.GainLoss.IsPositive())
}
