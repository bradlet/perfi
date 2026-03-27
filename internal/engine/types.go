package engine

import (
	"time"

	"github.com/shopspring/decimal"
)

// Transaction represents a single buy or sell event for any asset type.
// Quantity is positive for purchases and negative for sales.
type Transaction struct {
	ID           int64
	Source       string
	Date         time.Time
	Asset        string
	Quantity     decimal.Decimal
	PricePerUnit decimal.Decimal
	TotalValue   decimal.Decimal
}

// IsBuy returns true if the transaction is a purchase.
func (t Transaction) IsBuy() bool {
	return t.Quantity.IsPositive()
}

// IsSell returns true if the transaction is a sale.
func (t Transaction) IsSell() bool {
	return t.Quantity.IsNegative()
}

// Lot represents a purchase lot held in inventory.
type Lot struct {
	TransactionID int64
	Date          time.Time
	OriginalQty   decimal.Decimal
	RemainingQty  decimal.Decimal
	PricePerUnit  decimal.Decimal
}

// LotConsumption records how a specific sale consumed a specific purchase lot.
type LotConsumption struct {
	SaleTransactionID int64
	LotTransactionID  int64
	QuantityUsed      decimal.Decimal
	CostBasis         decimal.Decimal // PricePerUnit * QuantityUsed
}

// SaleSummary aggregates the cost basis information for a single sale transaction.
type SaleSummary struct {
	TransactionID int64
	Date          time.Time
	QuantitySold  decimal.Decimal
	Proceeds      decimal.Decimal
	CostBasis     decimal.Decimal
	GainLoss      decimal.Decimal
	IsLongTerm    bool // true if all consumed lots were held > 1 year
}

// CostBasisResult is the complete output of a cost basis calculation run.
type CostBasisResult struct {
	Asset         string
	Method        string
	Consumptions  []LotConsumption
	SaleSummaries []SaleSummary
}
