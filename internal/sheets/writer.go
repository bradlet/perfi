package sheets

import (
	"github.com/bradlet/perfi/internal/engine"
)

// FormatTransactionsForSheet converts transactions back to the sheet's expected
// row format: Source, Date (YYYY-MM-DD), Quantity, PricePerUnit, TotalValue.
// No header row is included since these are appended to existing data.
func FormatTransactionsForSheet(txns []engine.Transaction) [][]interface{} {
	rows := make([][]interface{}, 0, len(txns))
	for _, t := range txns {
		rows = append(rows, []interface{}{
			t.Source,
			t.Date.Format("01/02/2006"),
			t.Quantity.InexactFloat64(),
			t.PricePerUnit.InexactFloat64(),
			t.TotalValue.InexactFloat64(),
		})
	}
	return rows
}

// FormatSaleSummaries converts sale summaries into rows suitable for writing
// to a Google Sheet. Each row contains: Date, Quantity Sold, Proceeds,
// Cost Basis, Gain/Loss, Holding Period.
func FormatSaleSummaries(summaries []engine.SaleSummary) [][]interface{} {
	header := []interface{}{
		"Date", "Quantity Sold", "Proceeds", "Cost Basis", "Gain/Loss", "Holding Period",
	}

	rows := make([][]interface{}, 0, len(summaries)+1)
	rows = append(rows, header)

	for _, s := range summaries {
		term := "Short-term"
		if s.IsLongTerm {
			term = "Long-term"
		}
		rows = append(rows, []interface{}{
			s.Date.Format("01/02/2006"),
			s.QuantitySold.StringFixed(8),
			s.Proceeds.StringFixed(2),
			s.CostBasis.StringFixed(2),
			s.GainLoss.StringFixed(2),
			term,
		})
	}

	return rows
}

// FormatLotConsumptions converts lot consumptions into rows showing which
// lots were consumed by each sale. Each row contains: Sale Txn ID,
// Lot Txn ID, Quantity Used, Cost Basis.
func FormatLotConsumptions(consumptions []engine.LotConsumption) [][]interface{} {
	header := []interface{}{
		"Sale Txn ID", "Lot Txn ID", "Quantity Used", "Cost Basis",
	}

	rows := make([][]interface{}, 0, len(consumptions)+1)
	rows = append(rows, header)

	for _, c := range consumptions {
		rows = append(rows, []interface{}{
			c.SaleTransactionID,
			c.LotTransactionID,
			c.QuantityUsed.StringFixed(8),
			c.CostBasis.StringFixed(2),
		})
	}

	return rows
}
