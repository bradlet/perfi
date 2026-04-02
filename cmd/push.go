package cmd

import (
	"fmt"

	"github.com/bradlet/perfi/internal/config"
	"github.com/bradlet/perfi/internal/engine"
	"github.com/bradlet/perfi/internal/sheets"
	"github.com/bradlet/perfi/internal/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Write local transactions and calculation results to Google Sheet",
	Long: `Appends any locally-recorded transactions (from 'perfi sell') to the
transaction log in the Google Sheet, then writes the latest cost basis
results to the configured output range. Use --dry-run to preview what
would be written without making changes.`,
	RunE: runPush,
}

func init() {
	pushCmd.Flags().String("sheet-id", "", "Google Sheet ID (overrides config)")
	pushCmd.Flags().String("range", "", "Target range for output (overrides config)")
	pushCmd.Flags().String("db", "", "SQLite database path (overrides config)")
	pushCmd.Flags().Bool("dry-run", false, "Print output without writing to sheet")

	rootCmd.AddCommand(pushCmd)
}

func runPush(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	asset := cfg.AssetOrDefault(viper.GetString("asset"))
	if asset == "" {
		return fmt.Errorf("no asset specified: use --asset flag or set default_asset in config")
	}

	method := cfg.Method

	assetCfg, err := cfg.GetAssetConfig(asset)
	if err != nil {
		return err
	}

	writeRange := assetCfg.WriteRange
	if r, _ := cmd.Flags().GetString("range"); r != "" {
		writeRange = r
	}
	if writeRange == "" {
		return fmt.Errorf("no write range configured for asset %q", asset)
	}

	readRange := assetCfg.ReadRange

	sheetID := cfg.SheetID
	if sheetID == "" {
		return fmt.Errorf("no sheet_id configured")
	}

	dbPath := cfg.DBPath
	if d, _ := cmd.Flags().GetString("db"); d != "" {
		dbPath = d
	}

	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Append local transactions to the sheet.
	localTxns, err := store.GetLocalTransactions(ctx, asset)
	if err != nil {
		return fmt.Errorf("getting local transactions: %w", err)
	}

	hasLocalTxns := len(localTxns) > 0
	if hasLocalTxns {
		if readRange == "" {
			return fmt.Errorf("no read range configured for asset %q — needed to append local transactions", asset)
		}

		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "Dry run — would append %d local transactions to %s.\n", len(localTxns), readRange)
		}
	}

	// Load transactions and re-run calculation to get full summaries.
	txns, err := store.GetTransactions(ctx, asset)
	if err != nil {
		return fmt.Errorf("loading transactions: %w", err)
	}

	if len(txns) == 0 {
		return fmt.Errorf("no transactions found for asset %q — run 'perfi pull' first", asset)
	}

	calculator, err := engine.NewCalculator(method)
	if err != nil {
		return err
	}

	result, err := calculator.Calculate(ctx, txns)
	if err != nil {
		return fmt.Errorf("calculating for output: %w", err)
	}

	if len(result.SaleSummaries) == 0 && !hasLocalTxns {
		fmt.Fprintf(cmd.OutOrStdout(), "No sales found — nothing to push.\n")
		return nil
	}

	rows := sheets.FormatSaleSummaries(result.SaleSummaries)

	if dryRun {
		if len(result.SaleSummaries) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Dry run — would write %d rows to %s:\n", len(rows), writeRange)
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "  %v\n", row)
			}
		}
		return nil
	}

	client, err := sheets.NewGoogleSheetsClient(ctx, cfg.ServiceAccount)
	if err != nil {
		return fmt.Errorf("creating sheets client: %w", err)
	}

	// Write local transactions first.
	if hasLocalTxns {
		appendRows := sheets.FormatTransactionsForSheet(localTxns)
		if err := client.AppendRange(ctx, sheetID, readRange, appendRows); err != nil {
			return fmt.Errorf("appending local transactions to sheet: %w", err)
		}

		ids := make([]int64, len(localTxns))
		for i, t := range localTxns {
			ids[i] = t.ID
		}
		if err := store.MarkTransactionsSynced(ctx, ids); err != nil {
			return fmt.Errorf("marking transactions as synced: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Appended %d local transactions to sheet.\n", len(localTxns))
	}

	// Write sale results.
	if len(result.SaleSummaries) > 0 {
		if err := client.WriteRange(ctx, sheetID, writeRange, rows); err != nil {
			return fmt.Errorf("writing to sheet: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Pushed %d sale summaries to sheet.\n", len(result.SaleSummaries))
	}

	return nil
}
