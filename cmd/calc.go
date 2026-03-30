package cmd

import (
	"fmt"

	"github.com/bradlet/perfi/internal/config"
	"github.com/bradlet/perfi/internal/engine"
	"github.com/bradlet/perfi/internal/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var calcCmd = &cobra.Command{
	Use:   "calc",
	Short: "Run cost basis calculation on synced transaction data",
	Long: `Loads transactions from the local SQLite database and runs the
specified cost basis calculation method (default: FIFO). Results are
saved back to the database for later retrieval or pushing to a sheet.`,
	RunE: runCalc,
}

func init() {
	calcCmd.Flags().String("method", "", "Cost basis method: fifo, average (overrides config)")
	calcCmd.Flags().String("db", "", "SQLite database path (overrides config)")

	rootCmd.AddCommand(calcCmd)
}

func runCalc(cmd *cobra.Command, args []string) error {
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
	if m, _ := cmd.Flags().GetString("method"); m != "" {
		method = m
	}

	dbPath := cfg.DBPath
	if d, _ := cmd.Flags().GetString("db"); d != "" {
		dbPath = d
	}

	calculator, err := engine.NewCalculator(method)
	if err != nil {
		return err
	}

	store, err := storage.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer store.Close()

	if err := store.Init(ctx); err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	txns, err := store.GetTransactions(ctx, asset)
	if err != nil {
		return fmt.Errorf("loading transactions: %w", err)
	}

	if len(txns) == 0 {
		return fmt.Errorf("no transactions found for asset %q — run 'perfi sync' first", asset)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Calculating %s cost basis for %d %s transactions...\n",
		method, len(txns), asset)

	result, err := calculator.Calculate(ctx, txns)
	if err != nil {
		return fmt.Errorf("calculating cost basis: %w", err)
	}

	if err := store.SaveResults(ctx, result); err != nil {
		return fmt.Errorf("saving results: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Calculated %d sale summaries, %d lot consumptions.\n",
		len(result.SaleSummaries), len(result.Consumptions))

	if viper.GetBool("verbose") {
		for _, s := range result.SaleSummaries {
			term := "short-term"
			if s.IsLongTerm {
				term = "long-term"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: sold %s %s | proceeds %s | basis %s | gain/loss %s (%s)\n",
				s.Date.Format("2006-01-02"), s.QuantitySold, asset,
				s.Proceeds, s.CostBasis, s.GainLoss, term)
		}
	}

	return nil
}
