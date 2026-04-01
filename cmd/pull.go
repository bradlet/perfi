package cmd

import (
	"fmt"

	"github.com/bradlet/perfi/internal/config"
	"github.com/bradlet/perfi/internal/sheets"
	"github.com/bradlet/perfi/internal/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull transaction data from Google Sheet into local SQLite",
	Long: `Reads transaction data from the configured Google Sheet range and
upserts it into the local SQLite database. Existing transactions with
matching (asset, source, date, quantity) keys are updated rather than
duplicated.`,
	RunE: runPull,
}

func init() {
	pullCmd.Flags().String("sheet-id", "", "Google Sheet ID (overrides config)")
	pullCmd.Flags().String("range", "", "Sheet range to read (overrides config)")
	pullCmd.Flags().String("db", "", "SQLite database path (overrides config)")
	pullCmd.Flags().Bool("fresh", false, "Delete all sheet-origin transactions for the asset before pulling")

	viper.BindPFlag("sheet_id", pullCmd.Flags().Lookup("sheet-id"))
	viper.BindPFlag("db_path", pullCmd.Flags().Lookup("db"))

	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
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

	store, err := storage.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	client, err := sheets.NewGoogleSheetsClient(ctx)
	if err != nil {
		return fmt.Errorf("creating sheets client: %w", err)
	}

	fresh, _ := cmd.Flags().GetBool("fresh")
	if fresh {
		if err := store.Reset(ctx); err != nil {
			return fmt.Errorf("resetting database: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Database reset.\n")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pulling %s transactions from sheet...\n", asset)

	rows, err := client.ReadRange(ctx, sheetID, readRange)
	if err != nil {
		return fmt.Errorf("reading sheet: %w", err)
	}

	txns, err := sheets.ParseSheetRows(rows, asset)
	if err != nil {
		return fmt.Errorf("parsing sheet data: %w", err)
	}

	if err := store.UpsertTransactions(ctx, asset, txns); err != nil {
		return fmt.Errorf("upserting transactions: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Pulled %d %s transactions.\n", len(txns), asset)
	return nil
}
