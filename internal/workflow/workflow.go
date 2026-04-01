package workflow

import (
	"context"
	"fmt"
	"io"

	"github.com/bradlet/perfi/internal/engine"
	"github.com/bradlet/perfi/internal/sheets"
	"github.com/bradlet/perfi/internal/storage"
)

// Runner orchestrates the full pull -> calc -> push pipeline.
type Runner struct {
	SheetsClient sheets.Client
	Store        storage.Store
	Out          io.Writer
}

// RunParams holds the parameters for a full pipeline run.
type RunParams struct {
	SpreadsheetID string
	ReadRange     string
	WriteRange    string
	Asset         string
	Method        string
	DryRun        bool
	Fresh         bool
}

// Run executes the complete pipeline: pull from sheet, calculate cost basis,
// push local transactions and results back to the sheet.
func (r *Runner) Run(ctx context.Context, p RunParams) error {
	// Step 1: Pull transactions from the sheet.
	if p.Fresh {
		if err := r.Store.Reset(ctx); err != nil {
			return fmt.Errorf("resetting database: %w", err)
		}
		fmt.Fprintf(r.Out, "Database reset.\n")
	}

	fmt.Fprintf(r.Out, "Pulling %s transactions from sheet...\n", p.Asset)
	rows, err := r.SheetsClient.ReadRange(ctx, p.SpreadsheetID, p.ReadRange)
	if err != nil {
		return fmt.Errorf("reading sheet: %w", err)
	}

	txns, err := sheets.ParseSheetRows(rows, p.Asset)
	if err != nil {
		return fmt.Errorf("parsing sheet data: %w", err)
	}

	if err := r.Store.UpsertTransactions(ctx, p.Asset, txns); err != nil {
		return fmt.Errorf("upserting transactions: %w", err)
	}
	fmt.Fprintf(r.Out, "Pulled %d transactions.\n", len(txns))

	// Step 2: Calculate
	calculator, err := engine.NewCalculator(p.Method)
	if err != nil {
		return err
	}

	// Re-load from store to get IDs assigned by the database.
	storedTxns, err := r.Store.GetTransactions(ctx, p.Asset)
	if err != nil {
		return fmt.Errorf("loading transactions: %w", err)
	}

	fmt.Fprintf(r.Out, "Calculating %s cost basis...\n", p.Method)
	result, err := calculator.Calculate(ctx, storedTxns)
	if err != nil {
		return fmt.Errorf("calculating cost basis: %w", err)
	}

	if err := r.Store.SaveResults(ctx, result); err != nil {
		return fmt.Errorf("saving results: %w", err)
	}
	fmt.Fprintf(r.Out, "Calculated %d sale summaries.\n", len(result.SaleSummaries))

	// Step 3: Push — append local transactions, then write results.
	localTxns, err := r.Store.GetLocalTransactions(ctx, p.Asset)
	if err != nil {
		return fmt.Errorf("getting local transactions: %w", err)
	}

	if len(localTxns) > 0 {
		if p.DryRun {
			fmt.Fprintf(r.Out, "Dry run — would append %d local transactions to sheet.\n", len(localTxns))
		} else {
			appendRows := sheets.FormatTransactionsForSheet(localTxns)
			if err := r.SheetsClient.AppendRange(ctx, p.SpreadsheetID, p.ReadRange, appendRows); err != nil {
				return fmt.Errorf("appending local transactions to sheet: %w", err)
			}

			ids := make([]int64, len(localTxns))
			for i, t := range localTxns {
				ids[i] = t.ID
			}
			if err := r.Store.MarkTransactionsSynced(ctx, ids); err != nil {
				return fmt.Errorf("marking transactions as synced: %w", err)
			}

			fmt.Fprintf(r.Out, "Appended %d local transactions to sheet.\n", len(localTxns))
		}
	}

	if len(result.SaleSummaries) == 0 {
		fmt.Fprintf(r.Out, "No sales found — nothing to push.\n")
		return nil
	}

	output := sheets.FormatSaleSummaries(result.SaleSummaries)

	if p.DryRun {
		fmt.Fprintf(r.Out, "Dry run — would write %d rows to %s.\n", len(output), p.WriteRange)
		return nil
	}

	if err := r.SheetsClient.WriteRange(ctx, p.SpreadsheetID, p.WriteRange, output); err != nil {
		return fmt.Errorf("writing to sheet: %w", err)
	}
	fmt.Fprintf(r.Out, "Pushed %d sale summaries to sheet.\n", len(result.SaleSummaries))

	return nil
}
