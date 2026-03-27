package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// AssetConfig holds the Google Sheets ranges for a specific asset.
type AssetConfig struct {
	ReadRange  string `mapstructure:"read_range"`
	WriteRange string `mapstructure:"write_range"`
}

// Config holds all application configuration.
type Config struct {
	DefaultAsset string                 `mapstructure:"default_asset"`
	SheetID      string                 `mapstructure:"sheet_id"`
	Assets       map[string]AssetConfig `mapstructure:"assets"`
	Method       string                 `mapstructure:"method"`
	DBPath       string                 `mapstructure:"db_path"`
}

// Load reads the Viper-bound configuration into a Config struct,
// applying defaults for any unset values.
func Load() (*Config, error) {
	viper.SetDefault("method", "fifo")
	viper.SetDefault("db_path", "./costbasis.db")

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// If --asset flag was set, use it as the default asset
	if asset := viper.GetString("asset"); asset != "" {
		cfg.DefaultAsset = asset
	}

	return &cfg, nil
}

// AssetOrDefault returns the provided asset if non-empty, otherwise the default.
func (c *Config) AssetOrDefault(asset string) string {
	if asset != "" {
		return asset
	}
	return c.DefaultAsset
}

// GetAssetConfig returns the sheet config for a given asset, or an error if not found.
func (c *Config) GetAssetConfig(asset string) (AssetConfig, error) {
	ac, ok := c.Assets[asset]
	if !ok {
		return AssetConfig{}, fmt.Errorf("no configuration found for asset %q", asset)
	}
	return ac, nil
}
