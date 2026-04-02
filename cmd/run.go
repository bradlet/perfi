package cmd

import (
	"fmt"
	"os"

	"github.com/bradlet/perfi/internal/config"
	"github.com/bradlet/perfi/internal/sheets"
	"github.com/bradlet/perfi/internal/storage"
	"github.com/bradlet/perfi/internal/workflow"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Combined workflow: pull, calc, and push in one step",
	Long: `Executes the full pipeline: pulls transaction data from Google Sheet,
runs cost basis calculation, and writes local transactions and results
back to the sheet. Equivalent to running pull, calc, and push sequentially.`,
	RunE: runRun,
}

func init() {
	runCmd.Flags().String("sheet-id", "", "Google Sheet ID (overrides config)")
	runCmd.Flags().String("read-range", "", "Sheet range to read (overrides config)")
	runCmd.Flags().String("write-range", "", "Target range for output (overrides config)")
	runCmd.Flags().String("method", "", "Cost basis method: fifo, average (overrides config)")
	runCmd.Flags().String("db", "", "SQLite database path (overrides config)")
	runCmd.Flags().Bool("dry-run", false, "Print output without writing to sheet")
	runCmd.Flags().Bool("fresh", false, "Delete all sheet-origin transactions for the asset before pulling")

	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	asset := cfg.AssetOrDefault(viper.GetString("asset"))
	if asset == "" {
		return fmt.Errorf("no asset specified: use --asset flag or set default_asset in config")
	}

	assetCfg, err := cfg.GetAssetConfig(asset)
	if err != nil {
		return err
	}

	method := cfg.Method
	if m, _ := cmd.Flags().GetString("method"); m != "" {
		method = m
	}

	sheetID := cfg.SheetID
	if sheetID == "" {
		return fmt.Errorf("no sheet_id configured")
	}

	readRange := assetCfg.ReadRange
	if r, _ := cmd.Flags().GetString("read-range"); r != "" {
		readRange = r
	}

	writeRange := assetCfg.WriteRange
	if r, _ := cmd.Flags().GetString("write-range"); r != "" {
		writeRange = r
	}

	dbPath := cfg.DBPath
	if d, _ := cmd.Flags().GetString("db"); d != "" {
		dbPath = d
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	fresh, _ := cmd.Flags().GetBool("fresh")

	client, err := sheets.NewGoogleSheetsClient(ctx, cfg.ServiceAccount)
	if err != nil {
		return fmt.Errorf("creating sheets client: %w", err)
	}

	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	runner := &workflow.Runner{
		SheetsClient: client,
		Store:        store,
		Out:          os.Stdout,
	}

	return runner.Run(ctx, workflow.RunParams{
		SpreadsheetID: sheetID,
		ReadRange:     readRange,
		WriteRange:    writeRange,
		Asset:         asset,
		Method:        method,
		DryRun:        dryRun,
		Fresh:         fresh,
	})
}
