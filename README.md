# perfi

A Go CLI-based personal finance tracking and management tool. Currently solely used for tracking cost basis of financial assets using Google Sheets as the data source. Supports FIFO and average cost methods, multiple asset types, and persists data locally in SQLite.

## Potential Features

In the future, this CLI could help in more general personal finance tracking and tax preparation use-cases. It could bubble up reports and integrate with LLMs to offer unofficial financial management advice.

## Overview

`perfi` replaces manual spreadsheet-based cost basis tracking with a single CLI that:

1. **Pulls** transaction data from a Google Sheet into a local SQLite database
2. **Calculates** cost basis using FIFO or average cost methods
3. **Pushes** locally-recorded transactions and formatted results (gain/loss, holding period) back to the sheet

It's designed for personal tax reporting on crypto and other asset transactions.

## Prerequisites

- **Go 1.23+** — [install instructions](https://go.dev/doc/install)
- **Google Cloud CLI (`gcloud`)** — [install instructions](https://cloud.google.com/sdk/docs/install)
- A Google Sheet containing your transaction data

## GCP Setup

Run the interactive setup command:

```bash
perfi init
```

This will:
- Create or select a GCP project
- Create or select a service account
- Enable the Google Sheets API
- Grant your account permission to impersonate the service account

You can also provide an existing project and/or service account via flags:

```bash
# Use an existing project, create a new service account
perfi init --project my-existing-project

# Use both an existing project and service account
perfi init --project my-project --service-account perfi-sheets@my-project.iam.gserviceaccount.com
```

### Share your spreadsheet with the service account

After running `perfi init`, share your Google Sheet with the service account email printed at the end of setup. Open your sheet, click **Share**, and add the service account email as an **Editor**.

### Authenticate

If you haven't already, authenticate with Google Cloud:

```bash
gcloud auth application-default login
```

`perfi` authenticates by impersonating the service account using your local credentials — no service account key files are needed.

### Note your Sheet ID

Open your target Google Sheet in a browser and copy the **Sheet ID** from the URL:

```
https://docs.google.com/spreadsheets/d/SHEET_ID_HERE/edit
```

You'll need this for the configuration file.

## Installation

### From source

```bash
git clone https://github.com/bradlet/perfi.git
cd perfi
go build -o perfi .
```

Optionally move the binary to your PATH:

```bash
mv perfi ~/go/bin/
```

### Using `go install`

```bash
go install github.com/bradlet/perfi@latest
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
| B | Date (Excel serial number or `MM/DD/YYYY`) | `45292` (= 2024-01-01) or `01/01/2024` |
| C | Quantity (positive = buy, negative = sell) | `12.15` or `-3.5` |
| D | Price per unit | `82.25` |
| E | Transaction total (USD) | `1000.00` |

Header rows are automatically detected and skipped.

### Output format

When you run `perfi push`, the results are written back to your Google Sheet as a table with these columns:

| Column | Content | Example |
|--------|---------|---------|
| A | Date of the sale | `06/15/2024` |
| B | Quantity sold | `5.50000000` |
| C | Total proceeds (USD) | `550.00` |
| D | Total cost basis (USD) | `400.00` |
| E | Gain/Loss (USD) | `150.00` |
| F | Holding period | `Long-term` or `Short-term` |

**Where it gets written:** The `write_range` in your config specifies the starting cell. For example, `"Solana Cost Basis 2024!K1"` writes the header row to column K, and data rows below it. The output is written with a header row, followed by one row per sell transaction.

**Per-asset configuration:** Each asset has its own `write_range` in your config, since you may want results on different sheets or columns:

```yaml
assets:
  SOL:
    write_range: "Solana Cost Basis 2024!K1"   # Results for SOL here
  ETH:
    write_range: "ETH Cost Basis 2024!K1"       # Results for ETH here
```

**Override the write location:** Use the `--range` flag to write to a different location:

```bash
perfi push --range "Results!A1"                        # Override write location
perfi run --asset SOL --write-range "Summary!A1"       # Override for the full pipeline
```

## Usage

### Global flags

These flags are available on all commands:

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Config file path | `$HOME/.perfi.yaml` |
| `--asset` | Asset type (e.g., SOL, ETH) | From config `default_asset` |
| `--verbose` | Enable verbose output | `false` |

### `perfi pull`

Reads transaction data from the configured Google Sheet range and upserts it into the local SQLite database. Existing transactions with matching (asset, source, date, quantity) keys are updated rather than duplicated.

```bash
# Pull default asset
perfi pull

# Pull a specific asset
perfi pull --asset ETH

# Override the sheet range
perfi pull --range "My Sheet!A2:E500"

# Reset local database to match the sheet exactly
perfi pull --fresh
```

| Flag | Description | Default |
|------|-------------|---------|
| `--sheet-id` | Google Sheet ID | From config |
| `--range` | Sheet range to read | From config (`read_range`) |
| `--db` | SQLite database path | From config |
| `--fresh` | Delete all sheet-origin transactions for the asset before pulling | `false` |

By default, `pull` upserts — it updates existing rows and adds new ones, but does not remove transactions that were deleted from the sheet. Use `--fresh` to clear and re-import from the sheet. Unsynced local transactions (from `perfi sell`) are preserved.

### `perfi calc`

Loads transactions from the local SQLite database and runs cost basis calculation. Results are saved back to the database for later retrieval or pushing to a sheet.

```bash
# Calculate using the configured method (default: FIFO)
perfi calc

# Use average cost method
perfi calc --method average

# Verbose output shows per-sale details
perfi calc --verbose
```

| Flag | Description | Default |
|------|-------------|---------|
| `--method` | Cost basis method: `fifo`, `average` | From config |
| `--db` | SQLite database path | From config |

### `perfi push`

Appends any locally-recorded transactions (from `perfi sell`) to the transaction log in the Google Sheet, then writes the latest cost basis results to the configured output range.

```bash
# Preview what would be written
perfi push --dry-run

# Write local transactions and results to the configured ranges
perfi push

# Override the output target range
perfi push --range "Results!A1"
```

| Flag | Description | Default |
|------|-------------|---------|
| `--sheet-id` | Google Sheet ID | From config |
| `--range` | Target range for output | From config (`write_range`) |
| `--db` | SQLite database path | From config |
| `--dry-run` | Print output without writing to sheet | `false` |

### `perfi sell`

Records a sell transaction in the local SQLite database and immediately runs cost basis calculation. The transaction will be appended to the Google Sheet transaction log on the next `perfi push`.

```bash
# Sell 5 SOL at $150/unit (total defaults to quantity × price)
perfi sell --quantity 5 --price 150

# Specify total proceeds explicitly
perfi sell --quantity 5 --total 750

# Specify asset, date, and source
perfi sell --asset ETH --quantity 2 --price 3000 --date 2024-06-15 --source Coinbase

# Override cost basis method
perfi sell --quantity 5 --price 150 --method average
```

Either `--price` or `--total` must be provided.

| Flag | Description | Default |
|------|-------------|---------|
| `--quantity` | Quantity sold (required, positive number) | — |
| `--price` | Price per unit at sale time | — |
| `--total` | Total sale proceeds (optional; defaults to `quantity × price`) | — |
| `--source` | Source label for the transaction | `manual` |
| `--date` | Sale date in `YYYY-MM-DD` format | Today |
| `--method` | Cost basis method: `fifo`, `average` | From config |
| `--db` | SQLite database path | From config |

### `perfi run`

Executes the full pipeline: pulls transaction data from Google Sheet, runs cost basis calculation, and writes local transactions and results back to the sheet. Equivalent to running `pull`, `calc`, and `push` sequentially.

```bash
# Full pipeline
perfi run

# Full pipeline with dry-run (doesn't write to sheet)
perfi run --dry-run

# Full pipeline for a specific asset and method
perfi run --asset ETH --method average

# Full pipeline with a fresh pull from the sheet
perfi run --fresh
```

| Flag | Description | Default |
|------|-------------|---------|
| `--sheet-id` | Google Sheet ID | From config |
| `--read-range` | Sheet range to read | From config (`read_range`) |
| `--write-range` | Target range for output | From config (`write_range`) |
| `--method` | Cost basis method: `fifo`, `average` | From config |
| `--db` | SQLite database path | From config |
| `--dry-run` | Print output without writing to sheet | `false` |
| `--fresh` | Delete all sheet-origin transactions for the asset before pulling | `false` |

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
│   ├── init.go                  # GCP project + service account setup
│   ├── pull.go                  # Pull sheet → SQLite
│   ├── calc.go                  # Run cost basis calculation
│   ├── push.go                  # Write local txns + results → sheet
│   ├── run.go                   # Combined workflow
│   └── sell.go                  # Record a sell transaction locally
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
