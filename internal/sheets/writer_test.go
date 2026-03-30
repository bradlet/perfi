package sheets

import (
	"testing"
	"time"

	"github.com/bradlet/perfi/internal/engine"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatSaleSummaries(t *testing.T) {
	summaries := []engine.SaleSummary{
		{
			TransactionID: 1,
			Date:          time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			QuantitySold:  decimal.RequireFromString("10.5"),
			Proceeds:      decimal.RequireFromString("1575.00"),
			CostBasis:     decimal.RequireFromString("1050.00"),
			GainLoss:      decimal.RequireFromString("525.00"),
			IsLongTerm:    true,
		},
		{
			TransactionID: 2,
			Date:          time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
			QuantitySold:  decimal.RequireFromString("3.25"),
			Proceeds:      decimal.RequireFromString("487.50"),
			CostBasis:     decimal.RequireFromString("500.00"),
			GainLoss:      decimal.RequireFromString("-12.50"),
			IsLongTerm:    false,
		},
	}

	rows := FormatSaleSummaries(summaries)
	require.Len(t, rows, 3) // header + 2 data rows

	// Check header
	assert.Equal(t, "Date", rows[0][0])
	assert.Equal(t, "Gain/Loss", rows[0][4])
	assert.Equal(t, "Holding Period", rows[0][5])

	// Check first data row
	assert.Equal(t, "2024-06-01", rows[1][0])
	assert.Equal(t, "10.50000000", rows[1][1])
	assert.Equal(t, "1575.00", rows[1][2])
	assert.Equal(t, "1050.00", rows[1][3])
	assert.Equal(t, "525.00", rows[1][4])
	assert.Equal(t, "Long-term", rows[1][5])

	// Check second data row (loss, short-term)
	assert.Equal(t, "-12.50", rows[2][4])
	assert.Equal(t, "Short-term", rows[2][5])
}

func TestFormatSaleSummaries_Empty(t *testing.T) {
	rows := FormatSaleSummaries(nil)
	require.Len(t, rows, 1) // header only
}

func TestFormatLotConsumptions(t *testing.T) {
	consumptions := []engine.LotConsumption{
		{
			SaleTransactionID: 5,
			LotTransactionID:  1,
			QuantityUsed:      decimal.RequireFromString("3.5"),
			CostBasis:         decimal.RequireFromString("287.88"),
		},
	}

	rows := FormatLotConsumptions(consumptions)
	require.Len(t, rows, 2) // header + 1 data row
	assert.Equal(t, "Sale Txn ID", rows[0][0])
	assert.Equal(t, int64(5), rows[1][0])
	assert.Equal(t, int64(1), rows[1][1])
	assert.Equal(t, "3.50000000", rows[1][2])
	assert.Equal(t, "287.88", rows[1][3])
}
