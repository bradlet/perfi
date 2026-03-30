package cmd

import (
	"fmt"
	"time"

	"github.com/bradlet/perfi/internal/config"
	"github.com/bradlet/perfi/internal/engine"
	"github.com/bradlet/perfi/internal/storage"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var sellCmd = &cobra.Command{
	Use:   "sell",
	Short: "Record a sell transaction and calculate cost basis",
	Long: `Creates a sell transaction in the local SQLite database and runs cost
basis calculation. The transaction will be appended to the Google Sheet
transaction log on the next push.`,
	RunE: runSell,
}

func init() {
	sellCmd.Flags().Float64("quantity", 0, "Quantity sold (positive number)")
	sellCmd.Flags().Float64("price", 0, "Price per unit at sale time")
	sellCmd.Flags().Float64("total", 0, "Total sale proceeds (optional; defaults to quantity * price)")
	sellCmd.Flags().String("source", "manual", "Source label for the transaction")
	sellCmd.Flags().String("date", "", "Sale date in YYYY-MM-DD format (default: today)")
	sellCmd.Flags().String("method", "", "Cost basis method: fifo, average (overrides config)")
	sellCmd.Flags().String("db", "", "SQLite database path (overrides config)")

	rootCmd.AddCommand(sellCmd)
}

func runSell(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	asset := cfg.AssetOrDefault(viper.GetString("asset"))
	if asset == "" {
		return fmt.Errorf("no asset specified: use --asset flag or set default_asset in config")
	}

	qty, _ := cmd.Flags().GetFloat64("quantity")
	if qty <= 0 {
		return fmt.Errorf("--quantity is required and must be positive")
	}

	price, _ := cmd.Flags().GetFloat64("price")
	total, _ := cmd.Flags().GetFloat64("total")
	if price <= 0 && total <= 0 {
		return fmt.Errorf("--price or --total is required")
	}

	quantity := decimal.NewFromFloat(qty).Neg()
	pricePerUnit := decimal.NewFromFloat(price)
	var totalValue decimal.Decimal
	if total > 0 {
		totalValue = decimal.NewFromFloat(total)
	} else {
		totalValue = decimal.NewFromFloat(qty).Mul(pricePerUnit)
	}

	source, _ := cmd.Flags().GetString("source")

	saleDate := time.Now().UTC().Truncate(24 * time.Hour)
	if dateStr, _ := cmd.Flags().GetString("date"); dateStr != "" {
		saleDate, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			return fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
		}
	}

	method := cfg.Method
	if m, _ := cmd.Flags().GetString("method"); m != "" {
		method = m
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

	txn := engine.Transaction{
		Source:       source,
		Date:         saleDate,
		Asset:        asset,
		Quantity:     quantity,
		PricePerUnit: pricePerUnit,
		TotalValue:   totalValue,
	}

	id, err := store.InsertLocalTransaction(ctx, asset, txn)
	if err != nil {
		return fmt.Errorf("recording sell transaction: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Recorded sell: %s %s @ %s on %s (ID %d)\n",
		quantity.Abs(), asset, pricePerUnit, saleDate.Format("2006-01-02"), id)

	// Run cost basis calculation.
	txns, err := store.GetTransactions(ctx, asset)
	if err != nil {
		return fmt.Errorf("loading transactions: %w", err)
	}

	calculator, err := engine.NewCalculator(method)
	if err != nil {
		return err
	}

	result, err := calculator.Calculate(ctx, txns)
	if err != nil {
		return fmt.Errorf("calculating cost basis: %w", err)
	}

	if err := store.SaveResults(ctx, result); err != nil {
		return fmt.Errorf("saving results: %w", err)
	}

	// Print the sale summary for the new sell.
	for _, s := range result.SaleSummaries {
		if s.TransactionID == id {
			term := "short-term"
			if s.IsLongTerm {
				term = "long-term"
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"\nCost basis (%s):\n  Quantity:  %s %s\n  Proceeds:  $%s\n  Cost basis: $%s\n  Gain/Loss:  $%s (%s)\n",
				method, s.QuantitySold, asset, s.Proceeds.StringFixed(2),
				s.CostBasis.StringFixed(2), s.GainLoss.StringFixed(2), term)
			break
		}
	}

	return nil
}
