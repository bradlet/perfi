package engine

import (
	"context"
	"fmt"
)

// Calculator computes cost basis for a set of transactions using a specific method.
type Calculator interface {
	// Method returns the name of this cost basis method (e.g. "fifo", "average").
	Method() string
	// Calculate processes transactions and returns the full cost basis result.
	Calculate(ctx context.Context, txns []Transaction) (*CostBasisResult, error)
}

// NewCalculator returns a Calculator for the named method.
// Supported methods: "fifo", "average".
func NewCalculator(method string) (Calculator, error) {
	switch method {
	case "fifo":
		return &FIFOCalculator{}, nil
	case "average":
		return &AverageCalculator{}, nil
	default:
		return nil, fmt.Errorf("unknown cost basis method: %q", method)
	}
}
