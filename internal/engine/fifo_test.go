package engine

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func d(val string) decimal.Decimal {
	return decimal.RequireFromString(val)
}

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func txn(id int64, dateStr string, qty, price string) Transaction {
	q := d(qty)
	p := d(price)
	return Transaction{
		ID:           id,
		Source:       "test",
		Date:         date(dateStr),
		Asset:        "SOL",
		Quantity:     q,
		PricePerUnit: p,
		TotalValue:   q.Abs().Mul(p),
	}
}

func TestFIFOCalculator_Method(t *testing.T) {
	calc := &FIFOCalculator{}
	assert.Equal(t, "fifo", calc.Method())
}

func TestFIFOCalculator_EmptyTransactions(t *testing.T) {
	calc := &FIFOCalculator{}
	result, err := calc.Calculate(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "fifo", result.Method)
	assert.Empty(t, result.Consumptions)
	assert.Empty(t, result.SaleSummaries)
}

func TestFIFOCalculator_Calculate(t *testing.T) {
	tests := []struct {
		name             string
		txns             []Transaction
		wantConsumptions []LotConsumption
		wantSummaries    int
		wantErr          string
	}{
		{
			name: "single buy and exact sell",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-02-01", "-10", "120"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 2, LotTransactionID: 1, QuantityUsed: d("10"), CostBasis: d("1000")},
			},
			wantSummaries: 1,
		},
		{
			name: "partial sell from one lot",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-02-01", "-3", "120"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 2, LotTransactionID: 1, QuantityUsed: d("3"), CostBasis: d("300")},
			},
			wantSummaries: 1,
		},
		{
			name: "sell spans multiple lots",
			txns: []Transaction{
				txn(1, "2024-01-01", "5", "100"),
				txn(2, "2024-01-15", "5", "110"),
				txn(3, "2024-02-01", "-8", "120"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 3, LotTransactionID: 1, QuantityUsed: d("5"), CostBasis: d("500")},
				{SaleTransactionID: 3, LotTransactionID: 2, QuantityUsed: d("3"), CostBasis: d("330")},
			},
			wantSummaries: 1,
		},
		{
			name: "multiple sequential sales",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-02-01", "-3", "120"),
				txn(3, "2024-03-01", "-4", "130"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 2, LotTransactionID: 1, QuantityUsed: d("3"), CostBasis: d("300")},
				{SaleTransactionID: 3, LotTransactionID: 1, QuantityUsed: d("4"), CostBasis: d("400")},
			},
			wantSummaries: 2,
		},
		{
			name: "sell exactly at lot boundary",
			txns: []Transaction{
				txn(1, "2024-01-01", "5", "100"),
				txn(2, "2024-01-15", "5", "110"),
				txn(3, "2024-02-01", "-5", "120"),
				txn(4, "2024-03-01", "-5", "130"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 3, LotTransactionID: 1, QuantityUsed: d("5"), CostBasis: d("500")},
				{SaleTransactionID: 4, LotTransactionID: 2, QuantityUsed: d("5"), CostBasis: d("550")},
			},
			wantSummaries: 2,
		},
		{
			name: "fractional crypto amounts with high precision",
			txns: []Transaction{
				txn(1, "2024-01-01", "12.15", "82.25"),
				txn(2, "2024-01-02", "-0.0203", "85.50"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 2, LotTransactionID: 1, QuantityUsed: d("0.0203"), CostBasis: d("1.669675")},
			},
			wantSummaries: 1,
		},
		{
			name: "insufficient lots returns error",
			txns: []Transaction{
				txn(1, "2024-01-01", "5", "100"),
				txn(2, "2024-02-01", "-10", "120"),
			},
			wantErr: "insufficient lots: 5 units unsatisfied",
		},
		{
			name: "transactions out of order are sorted by date",
			txns: []Transaction{
				txn(2, "2024-02-01", "-3", "120"),
				txn(1, "2024-01-01", "10", "100"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 2, LotTransactionID: 1, QuantityUsed: d("3"), CostBasis: d("300")},
			},
			wantSummaries: 1,
		},
		{
			name: "buys only produces no summaries",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-02-01", "5", "110"),
			},
			wantConsumptions: nil,
			wantSummaries:    0,
		},
		{
			name: "zero quantity transaction is skipped",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-01-15", "0", "105"),
				txn(3, "2024-02-01", "-5", "120"),
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 3, LotTransactionID: 1, QuantityUsed: d("5"), CostBasis: d("500")},
			},
			wantSummaries: 1,
		},
		{
			name: "complex multi-lot multi-sale scenario",
			txns: []Transaction{
				txn(1, "2024-01-01", "5", "100"),
				txn(2, "2024-01-15", "10", "110"),
				txn(3, "2024-02-01", "-7", "120"),  // consumes 5 from lot 1, 2 from lot 2
				txn(4, "2024-02-15", "3", "115"),
				txn(5, "2024-03-01", "-10", "125"), // consumes 8 from lot 2, 2 from lot 4
			},
			wantConsumptions: []LotConsumption{
				{SaleTransactionID: 3, LotTransactionID: 1, QuantityUsed: d("5"), CostBasis: d("500")},
				{SaleTransactionID: 3, LotTransactionID: 2, QuantityUsed: d("2"), CostBasis: d("220")},
				{SaleTransactionID: 5, LotTransactionID: 2, QuantityUsed: d("8"), CostBasis: d("880")},
				{SaleTransactionID: 5, LotTransactionID: 4, QuantityUsed: d("2"), CostBasis: d("230")},
			},
			wantSummaries: 2,
		},
	}

	calc := &FIFOCalculator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := calc.Calculate(context.Background(), tt.txns)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, "fifo", result.Method)

			if tt.wantConsumptions == nil {
				assert.Empty(t, result.Consumptions)
			} else {
				require.Len(t, result.Consumptions, len(tt.wantConsumptions))
				for i, want := range tt.wantConsumptions {
					got := result.Consumptions[i]
					assert.Equal(t, want.SaleTransactionID, got.SaleTransactionID, "consumption[%d] sale txn ID", i)
					assert.Equal(t, want.LotTransactionID, got.LotTransactionID, "consumption[%d] lot txn ID", i)
					assert.True(t, want.QuantityUsed.Equal(got.QuantityUsed),
						"consumption[%d] quantity: want %s, got %s", i, want.QuantityUsed, got.QuantityUsed)
					assert.True(t, want.CostBasis.Equal(got.CostBasis),
						"consumption[%d] cost basis: want %s, got %s", i, want.CostBasis, got.CostBasis)
				}
			}

			assert.Len(t, result.SaleSummaries, tt.wantSummaries)
		})
	}
}

func TestFIFOCalculator_SaleSummaryGainLoss(t *testing.T) {
	calc := &FIFOCalculator{}
	txns := []Transaction{
		txn(1, "2024-01-01", "10", "100"),  // buy 10 @ $100 = $1000 cost
		txn(2, "2024-06-01", "-10", "150"), // sell 10 @ $150 = $1500 proceeds
	}

	result, err := calc.Calculate(context.Background(), txns)
	require.NoError(t, err)
	require.Len(t, result.SaleSummaries, 1)

	s := result.SaleSummaries[0]
	assert.True(t, d("10").Equal(s.QuantitySold))
	assert.True(t, d("1500").Equal(s.Proceeds))
	assert.True(t, d("1000").Equal(s.CostBasis))
	assert.True(t, d("500").Equal(s.GainLoss))
	assert.False(t, s.IsLongTerm, "held < 365 days should be short term")
}

func TestFIFOCalculator_LongTermGain(t *testing.T) {
	calc := &FIFOCalculator{}
	txns := []Transaction{
		txn(1, "2023-01-01", "10", "100"),
		txn(2, "2024-06-01", "-10", "150"), // held > 1 year
	}

	result, err := calc.Calculate(context.Background(), txns)
	require.NoError(t, err)
	require.Len(t, result.SaleSummaries, 1)
	assert.True(t, result.SaleSummaries[0].IsLongTerm)
}

func TestFIFOCalculator_MixedHoldingPeriods(t *testing.T) {
	calc := &FIFOCalculator{}
	txns := []Transaction{
		txn(1, "2023-01-01", "5", "100"), // > 1 year before sale
		txn(2, "2024-06-01", "5", "110"), // < 1 year before sale
		txn(3, "2024-07-01", "-8", "120"),
	}

	result, err := calc.Calculate(context.Background(), txns)
	require.NoError(t, err)
	require.Len(t, result.SaleSummaries, 1)
	// Mixed holding periods: not all lots are long-term
	assert.False(t, result.SaleSummaries[0].IsLongTerm)
}

func TestFIFOCalculator_ContextCancellation(t *testing.T) {
	calc := &FIFOCalculator{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	txns := []Transaction{
		txn(1, "2024-01-01", "10", "100"),
		txn(2, "2024-02-01", "-5", "120"),
	}

	_, err := calc.Calculate(ctx, txns)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestNewCalculator(t *testing.T) {
	calc, err := NewCalculator("fifo")
	require.NoError(t, err)
	assert.Equal(t, "fifo", calc.Method())

	_, err = NewCalculator("unknown")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown cost basis method")
}
