package cmd

import (
	"fmt"

	"github.com/bradlet/costbasis/internal/config"
	"github.com/bradlet/costbasis/internal/sheets"
	"github.com/bradlet/costbasis/internal/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull transaction data from Google Sheet into local SQLite",
	Long: `Reads transaction data from the configured Google Sheet range and
upserts it into the local SQLite database. Existing transactions with
matching (asset, source, date, quantity) keys are updated rather than
duplicated.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().String("sheet-id", "", "Google Sheet ID (overrides config)")
	syncCmd.Flags().String("range", "", "Sheet range to read (overrides config)")
	syncCmd.Flags().String("db", "", "SQLite database path (overrides config)")

	viper.BindPFlag("sheet_id", syncCmd.Flags().Lookup("sheet-id"))
	viper.BindPFlag("db_path", syncCmd.Flags().Lookup("db"))

	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
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

	readRange := assetCfg.ReadRange
	if r, _ := cmd.Flags().GetString("range"); r != "" {
		readRange = r
	}
	if readRange == "" {
		return fmt.Errorf("no read range configured for asset %q", asset)
	}

	sheetID := cfg.SheetID
	if sheetID == "" {
		return fmt.Errorf("no sheet_id configured")
	}

	client, err := sheets.NewGoogleSheetsClient(ctx)
	if err != nil {
		return fmt.Errorf("creating sheets client: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Syncing %s transactions from sheet...\n", asset)

	rows, err := client.ReadRange(ctx, sheetID, readRange)
	if err != nil {
		return fmt.Errorf("reading sheet: %w", err)
	}

	txns, err := sheets.ParseSheetRows(rows, asset)
	if err != nil {
		return fmt.Errorf("parsing sheet data: %w", err)
	}

	store, err := storage.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	if err := store.UpsertTransactions(ctx, asset, txns); err != nil {
		return fmt.Errorf("upserting transactions: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Synced %d %s transactions.\n", len(txns), asset)
	return nil
}
