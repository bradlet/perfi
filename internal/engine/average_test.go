package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAverageCalculator_Method(t *testing.T) {
	calc := &AverageCalculator{}
	assert.Equal(t, "average", calc.Method())
}

func TestAverageCalculator_EmptyTransactions(t *testing.T) {
	calc := &AverageCalculator{}
	result, err := calc.Calculate(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "average", result.Method)
	assert.Empty(t, result.Consumptions)
}

func TestAverageCalculator_Calculate(t *testing.T) {
	tests := []struct {
		name          string
		txns          []Transaction
		wantBasis     string // expected cost basis of first sale
		wantGainLoss  string
		wantSummaries int
		wantErr       string
	}{
		{
			name: "single buy and sell at same price",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-02-01", "-10", "100"),
			},
			wantBasis:     "1000",
			wantGainLoss:  "0",
			wantSummaries: 1,
		},
		{
			name: "single buy and sell at higher price",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-02-01", "-10", "150"),
			},
			wantBasis:     "1000",
			wantGainLoss:  "500",
			wantSummaries: 1,
		},
		{
			name: "two buys at different prices then full sell",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"), // cost 1000
				txn(2, "2024-01-15", "10", "200"), // cost 2000
				txn(3, "2024-02-01", "-20", "180"), // avg cost = 3000/20 = 150
			},
			wantBasis:     "3000",
			wantGainLoss:  "600", // proceeds 3600 - basis 3000
			wantSummaries: 1,
		},
		{
			name: "partial sell uses average cost",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"), // cost 1000
				txn(2, "2024-01-15", "10", "200"), // cost 2000
				txn(3, "2024-02-01", "-5", "180"),  // avg cost = 150, basis = 750
			},
			wantBasis:     "750",
			wantGainLoss:  "150", // proceeds 900 - basis 750
			wantSummaries: 1,
		},
		{
			name: "sequential sales update running average",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"), // cost 1000
				txn(2, "2024-02-01", "-5", "120"),  // avg=100, basis=500, remaining: 5@100=500
				txn(3, "2024-02-15", "10", "200"), // cost 2000, total: 15 units, cost 2500, avg~166.67
				txn(4, "2024-03-01", "-10", "180"), // avg=166.666..., basis=1666.67
			},
			wantSummaries: 2,
		},
		{
			name: "insufficient holdings",
			txns: []Transaction{
				txn(1, "2024-01-01", "5", "100"),
				txn(2, "2024-02-01", "-10", "120"),
			},
			wantErr: "insufficient holdings",
		},
		{
			name: "buys only",
			txns: []Transaction{
				txn(1, "2024-01-01", "10", "100"),
				txn(2, "2024-02-01", "5", "110"),
			},
			wantSummaries: 0,
		},
	}

	calc := &AverageCalculator{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := calc.Calculate(context.Background(), tt.txns)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "average", result.Method)
			assert.Len(t, result.SaleSummaries, tt.wantSummaries)

			if tt.wantBasis != "" && len(result.SaleSummaries) > 0 {
				s := result.SaleSummaries[0]
				assert.True(t, d(tt.wantBasis).Equal(s.CostBasis),
					"cost basis: want %s, got %s", tt.wantBasis, s.CostBasis)
				assert.True(t, d(tt.wantGainLoss).Equal(s.GainLoss),
					"gain/loss: want %s, got %s", tt.wantGainLoss, s.GainLoss)
			}
		})
	}
}

func TestNewCalculator_Average(t *testing.T) {
	calc, err := NewCalculator("average")
	require.NoError(t, err)
	assert.Equal(t, "average", calc.Method())
}
