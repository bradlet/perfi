# Data Model

perfi stores all data in a local SQLite database (`perfi.db` by default). There are three tables.

---

## Tables

### `transactions`

The source of truth for all buy and sell events. Rows come from two origins:

| Column          | Type   | Description |
|-----------------|--------|-------------|
| `id`            | INT PK | Auto-assigned row ID |
| `asset`         | TEXT   | Ticker symbol, e.g. `SOL` |
| `source`        | TEXT   | Where the transaction came from, e.g. `Coinbase`, `manual` |
| `date`          | TEXT   | RFC3339 timestamp of the transaction |
| `quantity`      | TEXT   | Positive = buy, negative = sell |
| `price_per_unit`| TEXT   | Price at time of transaction |
| `total_value`   | TEXT   | Absolute value of the transaction in USD |
| `synced_at`     | TEXT   | When this row was last written |
| `origin`        | TEXT   | `'sheet'` (pulled from Google Sheets) or `'local'` (added via `sell` command) |

**Natural key**: `(asset, source, date, quantity)` — upserts from the sheet use this key to avoid duplicates.

**`local` vs `sheet` origin**: Transactions added with `perfi sell` are stored with `origin='local'` until the next `run` or `sync`, at which point they are appended to the Google Sheet and their origin is updated to `'sheet'`.

---

### `lot_consumptions`

Records the detailed lot-matching work done by the cost basis calculator. Each row answers: *"For this sale, how much came from which purchase lot, and what did that cost?"*

| Column                | Type   | Description |
|-----------------------|--------|-------------|
| `id`                  | INT PK | Auto-assigned row ID |
| `asset`               | TEXT   | Ticker symbol |
| `method`              | TEXT   | Cost basis method used: `fifo` or `average` |
| `sale_transaction_id` | INT FK | References `transactions.id` for the sell event |
| `lot_transaction_id`  | INT FK | References `transactions.id` for the matched buy lot |
| `quantity_used`       | TEXT   | How many units from this lot were applied to this sale |
| `cost_basis`          | TEXT   | `price_per_unit × quantity_used` for this lot slice |
| `calculated_at`       | TEXT   | When this calculation was run |

**Natural key**: `(asset, method, sale_transaction_id, lot_transaction_id)` — re-running a calculation replaces old rows.

---

### `calc_runs`

A lightweight audit log. One row is written per `Calculate()` call, recording high-level metadata about what was computed.

| Column          | Type   | Description |
|-----------------|--------|-------------|
| `id`            | INT PK | Auto-assigned row ID |
| `asset`         | TEXT   | Ticker symbol |
| `method`        | TEXT   | Cost basis method |
| `calculated_at` | TEXT   | Timestamp of the run |
| `txn_count`     | INT    | Total number of lot consumption rows produced |
| `sale_count`    | INT    | Number of distinct sale transactions summarized |

---

## Relationships

```
transactions (buy)  ──┐
                      ├── lot_consumptions.lot_transaction_id
transactions (sell) ──┤
                      └── lot_consumptions.sale_transaction_id
```

Both foreign keys in `lot_consumptions` point back to `transactions`. This means `lot_consumptions` (and `calc_runs`) must be deleted before `transactions` when resetting the database — see `storage.Reset()`.

---

## How `lot_consumptions` become `SaleSummary` values

The calculator processes all transactions in chronological order, maintaining a queue of open buy lots. When it encounters a sell:

1. **Lot matching** — it pulls quantity from the front of the lot queue (FIFO) or uses the running average cost (average method), recording one `LotConsumption` row per lot slice consumed. A single sale that spans multiple lots produces multiple `LotConsumption` rows, all sharing the same `sale_transaction_id`.

2. **Aggregation** — after all transactions are processed, `buildSaleSummaries` groups `LotConsumption` rows by `sale_transaction_id`. For each sale it computes:
   - `CostBasis` = sum of all `LotConsumption.CostBasis` values for that sale
   - `Proceeds` = `total_value` from the sell transaction
   - `GainLoss` = `Proceeds − CostBasis`
   - `IsLongTerm` = true only if *every* consumed lot was held for more than 365 days before the sale date

3. **Output** — the aggregated `SaleSummary` values are what get written back to Google Sheets. The underlying `LotConsumption` rows are what get persisted to SQLite (in the `lot_consumptions` table), so the per-lot detail is available for auditing even though only the summaries are pushed to the sheet.

### Example

Given two buys and one sell:

| Date   | Type | Qty   | Price | Total |
|--------|------|-------|-------|-------|
| Jan 1  | Buy  | +10   | $100  | $1000 |
| Feb 1  | Buy  | +5    | $120  | $600  |
| Mar 1  | Sell | −8    | $130  | $1040 |

**FIFO lot matching** produces two `LotConsumption` rows for the Mar 1 sale:

| `lot_transaction_id` | `quantity_used` | `cost_basis` |
|----------------------|-----------------|--------------|
| Jan 1 buy ID         | 8               | $800 (8×$100) |

(The Jan 1 lot covers the full 8 units, so the Feb 1 lot is untouched.)

**`SaleSummary` for the Mar 1 sale**:
- `CostBasis` = $800
- `Proceeds` = $1040
- `GainLoss` = +$240
- `IsLongTerm` = false (Jan 1 → Mar 1 is only 2 months)
