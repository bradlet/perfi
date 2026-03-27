package cmd

import (
	"fmt"

	"github.com/bradlet/costbasis/internal/config"
	"github.com/bradlet/costbasis/internal/engine"
	"github.com/bradlet/costbasis/internal/sheets"
	"github.com/bradlet/costbasis/internal/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Write calculation results back to Google Sheet",
	Long: `Reads the latest cost basis results from the local SQLite database
and writes them to the configured Google Sheet range. Use --dry-run
to preview what would be written without making changes.`,
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

	// Load transactions and re-run calculation to get full summaries with dates/proceeds.
	txns, err := store.GetTransactions(ctx, asset)
	if err != nil {
		return fmt.Errorf("loading transactions: %w", err)
	}

	if len(txns) == 0 {
		return fmt.Errorf("no transactions found for asset %q — run 'costbasis sync' first", asset)
	}

	calculator, err := engine.NewCalculator(method)
	if err != nil {
		return err
	}

	result, err := calculator.Calculate(ctx, txns)
	if err != nil {
		return fmt.Errorf("calculating for output: %w", err)
	}

	if len(result.SaleSummaries) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No sales found — nothing to push.\n")
		return nil
	}

	rows := sheets.FormatSaleSummaries(result.SaleSummaries)

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — would write %d rows to %s:\n", len(rows), writeRange)
		for _, row := range rows {
			fmt.Fprintf(cmd.OutOrStdout(), "  %v\n", row)
		}
		return nil
	}

	client, err := sheets.NewGoogleSheetsClient(ctx)
	if err != nil {
		return fmt.Errorf("creating sheets client: %w", err)
	}

	if err := client.WriteRange(ctx, sheetID, writeRange, rows); err != nil {
		return fmt.Errorf("writing to sheet: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pushed %d sale summaries to sheet.\n", len(result.SaleSummaries))
	return nil
}
