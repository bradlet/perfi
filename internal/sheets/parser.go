package sheets

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/bradlet/perfi/internal/engine"
	"github.com/shopspring/decimal"
)

// excelEpoch is the Excel serial date epoch (December 30, 1899).
// Excel incorrectly treats 1900 as a leap year, so the epoch is offset by 2 days.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// ExcelSerialToTime converts an Excel serial date number to a time.Time.
// Google Sheets uses the same serial date system as Excel.
func ExcelSerialToTime(serial float64) time.Time {
	days := int(math.Floor(serial))
	return excelEpoch.AddDate(0, 0, days)
}

// ParseSheetRows converts raw Google Sheets rows into engine.Transaction values.
// Expected column order: Source (A), Date (B), Quantity (C), PricePerUnit (D), TotalValue (E).
// The asset parameter is applied to all parsed transactions.
func ParseSheetRows(rows [][]interface{}, asset string) ([]engine.Transaction, error) {
	var txns []engine.Transaction

	for i, row := range rows {
		if len(row) < 5 {
			return nil, fmt.Errorf("row %d: expected at least 5 columns, got %d", i+1, len(row))
		}

		source, err := cellToString(row[0])
		if err != nil {
			return nil, fmt.Errorf("row %d col A (source): %w", i+1, err)
		}

		// Skip header rows — if the date cell can't be parsed as a number, skip it.
		dateStr, err := cellToString(row[1])
		if err != nil {
			return nil, fmt.Errorf("row %d col B (date): %w", i+1, err)
		}
		dateVal, err := strconv.ParseFloat(strings.TrimSpace(dateStr), 64)
		if err != nil {
			// Likely a header row, skip.
			continue
		}
		date := ExcelSerialToTime(dateVal)

		qty, err := cellToDecimal(row[2])
		if err != nil {
			return nil, fmt.Errorf("row %d col C (quantity): %w", i+1, err)
		}

		// Skip zero-quantity rows.
		if qty.IsZero() {
			continue
		}

		price, err := cellToDecimal(row[3])
		if err != nil {
			return nil, fmt.Errorf("row %d col D (price): %w", i+1, err)
		}

		total, err := cellToDecimal(row[4])
		if err != nil {
			return nil, fmt.Errorf("row %d col E (total): %w", i+1, err)
		}

		txns = append(txns, engine.Transaction{
			Source:       source,
			Date:         date,
			Asset:        asset,
			Quantity:     qty,
			PricePerUnit: price,
			TotalValue:   total,
		})
	}

	return txns, nil
}

func cellToString(cell interface{}) (string, error) {
	switch v := cell.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case nil:
		return "", nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func cellToDecimal(cell interface{}) (decimal.Decimal, error) {
	s, err := cellToString(cell)
	if err != nil {
		return decimal.Zero, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero, fmt.Errorf("parsing decimal %q: %w", s, err)
	}
	return d, nil
}
