package sheets

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExcelSerialToTime(t *testing.T) {
	tests := []struct {
		name   string
		serial float64
		want   time.Time
	}{
		{
			name:   "Jan 1 2024",
			serial: 45292,
			want:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "Feb 14 2022 (from spreadsheet sample)",
			serial: 44606,
			want:   time.Date(2022, 2, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "Jan 1 2025",
			serial: 45658,
			want:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "Mar 1 2024 (leap year)",
			serial: 45352,
			want:   time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExcelSerialToTime(tt.serial)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSheetRows(t *testing.T) {
	tests := []struct {
		name    string
		rows    [][]interface{}
		asset   string
		wantLen int
		wantErr string
	}{
		{
			name: "valid rows with header skipped",
			rows: [][]interface{}{
				{"Source", "Date", "Solana Count", "Solana Value", "Transaction Value"},
				{"Coinbase", 45292.0, 12.15, 82.25, 1000.0},
				{"Phantom", 45307.0, -3.5, 95.0, 332.5},
			},
			asset:   "SOL",
			wantLen: 2,
		},
		{
			name: "zero quantity row skipped",
			rows: [][]interface{}{
				{"Coinbase", 45292.0, 0.0, 100.0, 0.0},
				{"Coinbase", 45300.0, 5.0, 110.0, 550.0},
			},
			asset:   "SOL",
			wantLen: 1,
		},
		{
			name: "string number cells",
			rows: [][]interface{}{
				{"Coinbase", "45292", "12.15", "82.25", "1000"},
			},
			asset:   "SOL",
			wantLen: 1,
		},
		{
			name: "too few columns",
			rows: [][]interface{}{
				{"Coinbase", 45292.0, 12.15},
			},
			asset:   "SOL",
			wantErr: "expected at least 5 columns",
		},
		{
			name:    "empty rows",
			rows:    [][]interface{}{},
			asset:   "SOL",
			wantLen: 0,
		},
		{
			name: "invalid quantity",
			rows: [][]interface{}{
				{"Coinbase", 45292.0, "not-a-number", 82.25, 1000.0},
			},
			asset:   "SOL",
			wantErr: "row 1 col C (quantity)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txns, err := ParseSheetRows(tt.rows, tt.asset)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Len(t, txns, tt.wantLen)
		})
	}
}

func TestParseSheetRows_ValuesCorrect(t *testing.T) {
	rows := [][]interface{}{
		{"Coinbase", 45292.0, 12.15, 82.25, 1000.0},
	}
	txns, err := ParseSheetRows(rows, "SOL")
	require.NoError(t, err)
	require.Len(t, txns, 1)

	txn := txns[0]
	assert.Equal(t, "Coinbase", txn.Source)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), txn.Date)
	assert.Equal(t, "SOL", txn.Asset)
	assert.True(t, decimal.RequireFromString("12.15").Equal(txn.Quantity))
	assert.True(t, decimal.RequireFromString("82.25").Equal(txn.PricePerUnit))
	assert.True(t, decimal.RequireFromString("1000").Equal(txn.TotalValue))
}

func TestParseSheetRows_NegativeQuantity(t *testing.T) {
	rows := [][]interface{}{
		{"Coinbase", 45292.0, -5.5, 120.0, 660.0},
	}
	txns, err := ParseSheetRows(rows, "SOL")
	require.NoError(t, err)
	require.Len(t, txns, 1)
	assert.True(t, txns[0].Quantity.IsNegative())
	assert.True(t, decimal.RequireFromString("-5.5").Equal(txns[0].Quantity))
}
