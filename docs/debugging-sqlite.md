# Debugging the SQLite Database

## CLI Setup

perfi uses SQLite (not PostgreSQL — `psql` won't work here). Use the `sqlite3` CLI.

**Install sqlite3:**

```bash
# macOS (usually pre-installed, or via Homebrew)
brew install sqlite

# Verify
sqlite3 --version
```

## Connecting to the Database

The default database path is `./perfi.db` in your working directory. It can be overridden via the `--db` flag or `PERFI_DB_PATH` env var.

```bash
# From the project root
sqlite3 perfi.db

# Or with an explicit path
sqlite3 /path/to/perfi.db
```

**Useful sqlite3 meta-commands:**

```sql
.tables              -- list all tables
.schema              -- show full schema for all tables
.schema transactions -- show schema for a specific table
.headers on          -- show column names in output
.mode column         -- align columns for readability
.quit                -- exit
```

## Table Overview

| Table | Purpose |
|---|---|
| `transactions` | All buy/sell transactions (from sheets or added locally via `sell`) |
| `lot_consumptions` | Cost basis calculation results — which lots were consumed by each sale |
| `calc_runs` | Audit log of each time `calc` was run |

## Helpful Debugging Queries

### Inspect transactions

```sql
-- All transactions, newest first
SELECT id, asset, source, date, quantity, price_per_unit, total_value, origin, synced_at
FROM transactions
ORDER BY date DESC;

-- Transactions for a specific asset
SELECT * FROM transactions WHERE asset = 'BTC' ORDER BY date;

-- Count by asset and origin
SELECT asset, origin, COUNT(*) as count
FROM transactions
GROUP BY asset, origin
ORDER BY asset;
```

### Inspect lot consumptions (cost basis results)

```sql
-- All lot consumptions
SELECT * FROM lot_consumptions ORDER BY calculated_at DESC;

-- Lot consumptions for a specific asset and method
SELECT
    lc.id,
    lc.asset,
    lc.method,
    lc.quantity_used,
    lc.cost_basis,
    sale.date  AS sale_date,
    sale.total_value AS sale_total,
    lot.date   AS lot_date,
    lot.price_per_unit AS lot_price
FROM lot_consumptions lc
JOIN transactions sale ON sale.id = lc.sale_transaction_id
JOIN transactions lot  ON lot.id  = lc.lot_transaction_id
WHERE lc.asset = 'BTC' AND lc.method = 'fifo'
ORDER BY sale.date, lot.date;

-- Count lot consumptions per asset
SELECT asset, method, COUNT(*) as count FROM lot_consumptions GROUP BY asset, method;
```

### Inspect calc runs

```sql
-- All calc runs, newest first
SELECT * FROM calc_runs ORDER BY calculated_at DESC;

-- Most recent calc run per asset
SELECT asset, method, MAX(calculated_at) as last_run, txn_count, sale_count
FROM calc_runs
GROUP BY asset, method;
```

## Running Queries Non-Interactively

```bash
# Run a single query from the shell
sqlite3 perfi.db "SELECT asset, COUNT(*) FROM transactions GROUP BY asset;"

# Export to CSV
sqlite3 -csv -header perfi.db "SELECT * FROM transactions;" > transactions.csv

# Run a .sql file
sqlite3 perfi.db < my_queries.sql
```
