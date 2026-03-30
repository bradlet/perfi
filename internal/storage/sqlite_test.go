package storage

import (
	"context"
	"testing"
	"time"

	"github.com/bradlet/perfi/internal/engine"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(val string) decimal.Decimal {
	return decimal.RequireFromString(val)
}

func setupStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.Init(context.Background()))
	t.Cleanup(func() { store.Close() })
	return store
}

func sampleTransactions() []engine.Transaction {
	return []engine.Transaction{
		{
			Source:       "Coinbase",
			Date:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Asset:        "SOL",
			Quantity:     d("12.15"),
			PricePerUnit: d("82.25"),
			TotalValue:   d("1000.00"),
		},
		{
			Source:       "Coinbase",
			Date:         time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
			Asset:        "SOL",
			Quantity:     d("-3.5"),
			PricePerUnit: d("95.00"),
			TotalValue:   d("332.50"),
		},
		{
			Source:       "Phantom",
			Date:         time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
			Asset:        "SOL",
			Quantity:     d("5.0"),
			PricePerUnit: d("90.00"),
			TotalValue:   d("450.00"),
		},
	}
}

func TestSQLiteStore_Init(t *testing.T) {
	store := setupStore(t)
	// Init is idempotent — calling it again should not error.
	require.NoError(t, store.Init(context.Background()))
}

func TestSQLiteStore_UpsertAndGetTransactions(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()
	txns := sampleTransactions()

	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))

	got, err := store.GetTransactions(ctx, "SOL")
	require.NoError(t, err)
	require.Len(t, got, 3)

	// Verify ordering: Jan 1, Jan 15, Feb 1
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), got[0].Date)
	assert.Equal(t, time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), got[1].Date)
	assert.Equal(t, time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC), got[2].Date)

	// Verify values round-trip correctly.
	assert.True(t, d("12.15").Equal(got[0].Quantity))
	assert.True(t, d("82.25").Equal(got[0].PricePerUnit))
	assert.True(t, d("1000.00").Equal(got[0].TotalValue))
	assert.Equal(t, "Coinbase", got[0].Source)
	assert.Equal(t, "SOL", got[0].Asset)

	// Verify negative quantity preserved.
	assert.True(t, d("-3.5").Equal(got[2].Quantity))
}

func TestSQLiteStore_UpsertIdempotency(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()
	txns := sampleTransactions()

	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))
	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))

	got, err := store.GetTransactions(ctx, "SOL")
	require.NoError(t, err)
	assert.Len(t, got, 3, "duplicate upsert should not create extra rows")
}

func TestSQLiteStore_UpsertUpdatesValues(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	txns := []engine.Transaction{{
		Source:       "Coinbase",
		Date:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Asset:        "SOL",
		Quantity:     d("10"),
		PricePerUnit: d("100"),
		TotalValue:   d("1000"),
	}}
	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))

	// Update the price
	txns[0].PricePerUnit = d("105")
	txns[0].TotalValue = d("1050")
	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))

	got, err := store.GetTransactions(ctx, "SOL")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.True(t, d("105").Equal(got[0].PricePerUnit), "price should be updated")
	assert.True(t, d("1050").Equal(got[0].TotalValue), "total should be updated")
}

func TestSQLiteStore_GetTransactions_FiltersByAsset(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	solTxns := []engine.Transaction{{
		Source: "Coinbase", Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Asset: "SOL", Quantity: d("10"), PricePerUnit: d("100"), TotalValue: d("1000"),
	}}
	ethTxns := []engine.Transaction{{
		Source: "Coinbase", Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Asset: "ETH", Quantity: d("5"), PricePerUnit: d("3000"), TotalValue: d("15000"),
	}}

	require.NoError(t, store.UpsertTransactions(ctx, "SOL", solTxns))
	require.NoError(t, store.UpsertTransactions(ctx, "ETH", ethTxns))

	sol, err := store.GetTransactions(ctx, "SOL")
	require.NoError(t, err)
	assert.Len(t, sol, 1)
	assert.Equal(t, "SOL", sol[0].Asset)

	eth, err := store.GetTransactions(ctx, "ETH")
	require.NoError(t, err)
	assert.Len(t, eth, 1)
	assert.Equal(t, "ETH", eth[0].Asset)
}

func TestSQLiteStore_GetTransactions_EmptyResult(t *testing.T) {
	store := setupStore(t)
	got, err := store.GetTransactions(context.Background(), "BTC")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestSQLiteStore_SaveAndGetResults(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	// First insert transactions so foreign keys are valid.
	txns := sampleTransactions()
	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))

	got, err := store.GetTransactions(ctx, "SOL")
	require.NoError(t, err)

	result := &engine.CostBasisResult{
		Asset:  "SOL",
		Method: "fifo",
		Consumptions: []engine.LotConsumption{
			{
				SaleTransactionID: got[2].ID, // the sell
				LotTransactionID:  got[0].ID, // first buy
				QuantityUsed:      d("3.5"),
				CostBasis:         d("287.875"),
			},
		},
		SaleSummaries: []engine.SaleSummary{
			{TransactionID: got[2].ID},
		},
	}

	require.NoError(t, store.SaveResults(ctx, result))

	loaded, err := store.GetResults(ctx, "SOL", "fifo")
	require.NoError(t, err)
	require.Len(t, loaded.Consumptions, 1)
	assert.Equal(t, "SOL", loaded.Asset)
	assert.Equal(t, "fifo", loaded.Method)
	assert.True(t, d("3.5").Equal(loaded.Consumptions[0].QuantityUsed))
	assert.True(t, d("287.875").Equal(loaded.Consumptions[0].CostBasis))
}

func TestSQLiteStore_SaveResults_ReplacesOld(t *testing.T) {
	store := setupStore(t)
	ctx := context.Background()

	txns := []engine.Transaction{{
		Source: "Coinbase", Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Asset: "SOL", Quantity: d("10"), PricePerUnit: d("100"), TotalValue: d("1000"),
	}}
	require.NoError(t, store.UpsertTransactions(ctx, "SOL", txns))
	got, _ := store.GetTransactions(ctx, "SOL")

	result1 := &engine.CostBasisResult{
		Asset: "SOL", Method: "fifo",
		Consumptions: []engine.LotConsumption{
			{SaleTransactionID: got[0].ID, LotTransactionID: got[0].ID, QuantityUsed: d("5"), CostBasis: d("500")},
		},
	}
	require.NoError(t, store.SaveResults(ctx, result1))

	result2 := &engine.CostBasisResult{
		Asset: "SOL", Method: "fifo",
		Consumptions: []engine.LotConsumption{
			{SaleTransactionID: got[0].ID, LotTransactionID: got[0].ID, QuantityUsed: d("10"), CostBasis: d("1000")},
		},
	}
	require.NoError(t, store.SaveResults(ctx, result2))

	loaded, err := store.GetResults(ctx, "SOL", "fifo")
	require.NoError(t, err)
	require.Len(t, loaded.Consumptions, 1, "old results should be replaced")
	assert.True(t, d("10").Equal(loaded.Consumptions[0].QuantityUsed))
}
