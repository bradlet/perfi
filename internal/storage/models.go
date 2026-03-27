package storage

import (
	"time"

	"github.com/bradlet/costbasis/internal/engine"
	"github.com/shopspring/decimal"
)

// transactionRow is the database representation of a transaction.
type transactionRow struct {
	ID           int64
	Asset        string
	Source       string
	Date         string // RFC3339
	Quantity     string // decimal string
	PricePerUnit string
	TotalValue   string
	SyncedAt     string
}

func (r transactionRow) toTransaction() (engine.Transaction, error) {
	date, err := time.Parse(time.RFC3339, r.Date)
	if err != nil {
		return engine.Transaction{}, err
	}
	qty, err := decimal.NewFromString(r.Quantity)
	if err != nil {
		return engine.Transaction{}, err
	}
	price, err := decimal.NewFromString(r.PricePerUnit)
	if err != nil {
		return engine.Transaction{}, err
	}
	total, err := decimal.NewFromString(r.TotalValue)
	if err != nil {
		return engine.Transaction{}, err
	}

	return engine.Transaction{
		ID:           r.ID,
		Source:       r.Source,
		Date:         date,
		Asset:        r.Asset,
		Quantity:     qty,
		PricePerUnit: price,
		TotalValue:   total,
	}, nil
}

func transactionToRow(asset string, t engine.Transaction) transactionRow {
	return transactionRow{
		Asset:        asset,
		Source:       t.Source,
		Date:         t.Date.Format(time.RFC3339),
		Quantity:     t.Quantity.String(),
		PricePerUnit: t.PricePerUnit.String(),
		TotalValue:   t.TotalValue.String(),
		SyncedAt:     time.Now().UTC().Format(time.RFC3339),
	}
}
