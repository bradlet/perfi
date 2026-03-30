# perfi-cli

A Go CLI-based personal finance tracking and management tool. Currently solely used for tracking cost basis of financial assets using Google Sheets as the data source. Supports FIFO and average cost methods, multiple asset types, and persists data locally in SQLite.

## Potential Features

In the future, this CLI could help in more general personal finance tracking and tax preparation use-cases. It could bubble up reports and integrate with LLMs to offer unofficial financial management advice.

## Overview

`perfi-cli` replaces manual spreadsheet-based cost basis tracking with a single CLI that:

1. **Syncs** transaction data from a Google Sheet into a local SQLite database
2. **Calculates** cost basis using FIFO or average cost methods
3. **Pushes** formatted results (gain/loss, holding period) back to the sheet

It's designed for personal tax reporting on crypto and other asset transactions.

## Prerequisites

- **Go 1.23+** — [install instructions](https://go.dev/doc/install)
- **Google Cloud CLI (`gcloud`)** — [install instructions](https://cloud.google.com/sdk/docs/install)
- A Google Sheet containing your transaction data

## GCP Setup

### 1. Create or select a GCP project

If you don't have a GCP project, create one:

```bash
gcloud projects create my-perfi --name="Cost Basis Tracker"
gcloud config set project my-perfi
```

Or select an existing project:

```bash
gcloud config set project YOUR_PROJECT_ID
```

### 2. Enable the Google Sheets API

```bash
gcloud services enable sheets.googleapis.com
```

You can verify it's enabled:

```bash
gcloud services list --enabled | grep sheets
```

### 3. Set up Application Default Credentials

Authenticate with the Sheets API scope so `perfi` can read and write your spreadsheets:

```bash
gcloud auth application-default login \
  --scopes=https://www.googleapis.com/auth/spreadsheets
```

This stores credentials at `~/.config/gcloud/application_default_credentials.json`. The CLI uses these automatically — no service account keys or OAuth client IDs needed.

### 4. Verify access

Open your target Google Sheet in a browser. Note the **Sheet ID** from the URL:

```
https://docs.google.com/spreadsheets/d/SHEET_ID_HERE/edit
```

You'll need this for the configuration file.

## Installation

### From source

```bash
git clone https://github.com/bradlet/perfci-cli.git
cd perfi-cli
go build -o perfi-cli .
```

Optionally move the binary to your PATH:

```bash
mv perfi ~/go/bin/
```

### Using `go install`

```bash
go install github.com/bradlet/perfi-cli@latest
```

## Configuration

Create a `.perfi.yaml` file in your home directory or the directory where you run the CLI:

```yaml
# Google Sheet ID (from the URL)
sheet_id: "1ABCdef123456789"

# Default asset to operate on
default_asset: SOL

# Cost basis method: "fifo" or "average"
method: fifo

# SQLite database path
db_path: "./perfi.db"

# Per-asset Google Sheets ranges
assets:
  SOL:
    # Range to read transaction data from (columns: Source, Date, Quantity, Price, Total)
    read_range: "Solana Cost Basis 2024!A:E"
    # Range to write calculated results to
    write_range: "Solana Cost Basis 2024!K1"
  ETH:
    read_range: "ETH Cost Basis 2024!A:E"
    write_range: "ETH Cost Basis 2024!K1"
```

All config values can be overridden via CLI flags or environment variables with the `perfi_` prefix (e.g., `perfi_SHEET_ID`).

### Expected sheet format

Your Google Sheet should have these columns in order:

| Column | Content | Example |
|--------|---------|---------|
| A | Source (exchange name) | `Coinbase` |
| B | Date (Excel serial number) | `45292` (= 2024-01-01) |
| C | Quantity (positive = buy, negative = sell) | `12.15` or `-3.5` |
| D | Price per unit | `82.25` |
| E | Transaction total (USD) | `1000.00` |

Header rows are automatically detected and skipped.

## Usage

### Sync transactions from Google Sheet

```bash
# Sync default asset
perfi sync

# Sync a specific asset
perfi sync --asset ETH

# Override the sheet range
perfi sync --range "My Sheet!A2:E500"
```

### Calculate cost basis

```bash
# Calculate using the configured method (default: FIFO)
perfi calc

# Use average cost method
perfi calc --method average

# Verbose output shows per-sale details
perfi calc --verbose
```

### Push results to Google Sheet

```bash
# Preview what would be written
perfi push --dry-run

# Write results to the configured range
perfi push

# Override the target range
perfi push --range "Results!A1"
```

### Combined workflow

Run sync, calc, and push in a single command:

```bash
# Full pipeline
perfi run

# Full pipeline with dry-run (doesn't write to sheet)
perfi run --dry-run

# Full pipeline for a specific asset and method
perfi run --asset ETH --method average
```

### Global flags

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Config file path | `$HOME/.perfi.yaml` |
| `--asset` | Asset type (e.g., SOL, ETH) | From config `default_asset` |
| `--verbose` | Enable verbose output | `false` |

## Supported Cost Basis Methods

### FIFO (First-In, First-Out)

The default method. When you sell an asset, the **earliest purchased** lots are consumed first. This is the most common method for tax reporting.

**Example:** Buy 10 SOL at $100, then buy 10 SOL at $200. Sell 15 SOL — the cost basis is (10 × $100) + (5 × $200) = $2,000.

### Average Cost

Each sale's cost basis is the **weighted average price** of all units held at the time of sale. The average is recalculated after each sale.

**Example:** Buy 10 SOL at $100 ($1,000), then buy 10 SOL at $200 ($2,000). Average cost = $3,000 / 20 = $150/unit. Sell 15 SOL — cost basis = 15 × $150 = $2,250.

### Holding Period

Both methods classify gains as **long-term** (held > 365 days) or **short-term**. For FIFO, this is based on the specific lots consumed. For average cost, the earliest lots are used for holding period determination.

## Development

### Project structure

```
perfi/
├── main.go                      # Entry point
├── cmd/                         # Cobra CLI commands
│   ├── root.go                  # Root command + Viper config
│   ├── sync.go                  # Pull sheet → SQLite
│   ├── calc.go                  # Run cost basis calculation
│   ├── push.go                  # Write results → sheet
│   └── run.go                   # Combined workflow
├── internal/
│   ├── config/                  # Viper config struct
│   ├── engine/                  # Cost basis calculators (FIFO, average)
│   ├── sheets/                  # Google Sheets API client + parser + writer
│   ├── storage/                 # SQLite persistence layer
│   └── workflow/                # Pipeline orchestrator
└── testdata/                    # Test fixtures
```

### Running tests

```bash
# All tests
go test ./...

# Verbose output
go test ./... -v

# Specific package
go test ./internal/engine/ -v

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Key design decisions

- **`shopspring/decimal`** for all monetary values — never `float64` for money
- **`modernc.org/sqlite`** (pure Go) — no CGO dependency, simplifies cross-compilation
- **Strategy pattern** for cost basis methods — add new methods by implementing the `Calculator` interface
- **All monetary values stored as TEXT** in SQLite to preserve decimal precision
- **Interface-based design** for sheets client and storage — enables testing without live API calls

### Adding a new cost basis method

1. Create `internal/engine/yourmethod.go` implementing `Calculator`
2. Add a case to `NewCalculator()` in `strategy.go`
3. Add tests in `internal/engine/yourmethod_test.go`
