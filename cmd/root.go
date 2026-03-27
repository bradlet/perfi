package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "costbasis",
	Short: "A CLI tool for tracking cost basis of financial assets",
	Long: `costbasis is a personal financial tooling CLI that reads transaction data
from Google Sheets, calculates cost basis using various methods (FIFO, average cost),
and writes results back to the sheet.

It supports multiple asset types and persists data locally in SQLite for
flexible querying and offline calculations.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.costbasis.yaml)")
	rootCmd.PersistentFlags().String("asset", "", "asset type to operate on (e.g. SOL, ETH)")
	rootCmd.PersistentFlags().Bool("verbose", false, "enable verbose logging")

	viper.BindPFlag("asset", rootCmd.PersistentFlags().Lookup("asset"))
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".costbasis")
	}

	viper.SetEnvPrefix("COSTBASIS")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if viper.GetBool("verbose") {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}
