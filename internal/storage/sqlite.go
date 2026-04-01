package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bradlet/perfi/internal/engine"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS transactions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    asset          TEXT NOT NULL,
    source         TEXT NOT NULL,
    date           TEXT NOT NULL,
    quantity       TEXT NOT NULL,
    price_per_unit TEXT NOT NULL,
    total_value    TEXT NOT NULL,
    synced_at      TEXT NOT NULL,
    origin         TEXT NOT NULL DEFAULT 'sheet',
    UNIQUE(asset, source, date, quantity)
);

CREATE INDEX IF NOT EXISTS idx_transactions_asset_date
    ON transactions(asset, date);

CREATE TABLE IF NOT EXISTS lot_consumptions (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    asset               TEXT NOT NULL,
    method              TEXT NOT NULL,
    sale_transaction_id INTEGER NOT NULL REFERENCES transactions(id),
    lot_transaction_id  INTEGER NOT NULL REFERENCES transactions(id),
    quantity_used       TEXT NOT NULL,
    cost_basis          TEXT NOT NULL,
    calculated_at       TEXT NOT NULL,
    UNIQUE(asset, method, sale_transaction_id, lot_transaction_id)
);

CREATE TABLE IF NOT EXISTS calc_runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    asset         TEXT NOT NULL,
    method        TEXT NOT NULL,
    calculated_at TEXT NOT NULL,
    txn_count     INTEGER NOT NULL,
    sale_count    INTEGER NOT NULL
);
`

// Store abstracts transaction persistence operations.
type Store interface {
	Init(ctx context.Context) error
	UpsertTransactions(ctx context.Context, asset string, txns []engine.Transaction) error
	InsertLocalTransaction(ctx context.Context, asset string, txn engine.Transaction) (int64, error)
	GetTransactions(ctx context.Context, asset string) ([]engine.Transaction, error)
	GetLocalTransactions(ctx context.Context, asset string) ([]engine.Transaction, error)
	Reset(ctx context.Context) error
	MarkTransactionsSynced(ctx context.Context, ids []int64) error
	SaveResults(ctx context.Context, result *engine.CostBasisResult) error
	GetResults(ctx context.Context, asset string, method string) (*engine.CostBasisResult, error)
	Close() error
}

// SQLiteStore implements Store using a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens or creates a SQLite database at the given path.
// Use ":memory:" for an in-memory database (useful for testing).
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("initializing schema: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpsertTransactions(ctx context.Context, asset string, txns []engine.Transaction) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO transactions (asset, source, date, quantity, price_per_unit, total_value, synced_at, origin)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'sheet')
		ON CONFLICT(asset, source, date, quantity)
		DO UPDATE SET price_per_unit=excluded.price_per_unit,
		              total_value=excluded.total_value,
		              synced_at=excluded.synced_at,
		              origin='sheet'
	`)
	if err != nil {
		return fmt.Errorf("preparing upsert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range txns {
		_, err := stmt.ExecContext(ctx,
			asset,
			t.Source,
			t.Date.Format(time.RFC3339),
			t.Quantity.String(),
			t.PricePerUnit.String(),
			t.TotalValue.String(),
			now,
		)
		if err != nil {
			return fmt.Errorf("upserting transaction: %w", err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetTransactions(ctx context.Context, asset string) ([]engine.Transaction, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, asset, source, date, quantity, price_per_unit, total_value
		FROM transactions
		WHERE asset = ?
		ORDER BY date ASC, id ASC
	`, asset)
	if err != nil {
		return nil, fmt.Errorf("querying transactions: %w", err)
	}
	defer rows.Close()

	var txns []engine.Transaction
	for rows.Next() {
		var r transactionRow
		if err := rows.Scan(&r.ID, &r.Asset, &r.Source, &r.Date, &r.Quantity, &r.PricePerUnit, &r.TotalValue); err != nil {
			return nil, fmt.Errorf("scanning transaction: %w", err)
		}
		t, err := r.toTransaction()
		if err != nil {
			return nil, fmt.Errorf("converting transaction row: %w", err)
		}
		txns = append(txns, t)
	}
	return txns, rows.Err()
}

func (s *SQLiteStore) InsertLocalTransaction(ctx context.Context, asset string, txn engine.Transaction) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO transactions (asset, source, date, quantity, price_per_unit, total_value, synced_at, origin)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'local')
	`,
		asset,
		txn.Source,
		txn.Date.Format(time.RFC3339),
		txn.Quantity.String(),
		txn.PricePerUnit.String(),
		txn.TotalValue.String(),
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting local transaction: %w", err)
	}
	return result.LastInsertId()
}

func (s *SQLiteStore) GetLocalTransactions(ctx context.Context, asset string) ([]engine.Transaction, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, asset, source, date, quantity, price_per_unit, total_value
		FROM transactions
		WHERE asset = ? AND origin = 'local'
		ORDER BY date ASC, id ASC
	`, asset)
	if err != nil {
		return nil, fmt.Errorf("querying local transactions: %w", err)
	}
	defer rows.Close()

	var txns []engine.Transaction
	for rows.Next() {
		var r transactionRow
		if err := rows.Scan(&r.ID, &r.Asset, &r.Source, &r.Date, &r.Quantity, &r.PricePerUnit, &r.TotalValue); err != nil {
			return nil, fmt.Errorf("scanning local transaction: %w", err)
		}
		t, err := r.toTransaction()
		if err != nil {
			return nil, fmt.Errorf("converting local transaction row: %w", err)
		}
		txns = append(txns, t)
	}
	return txns, rows.Err()
}

func (s *SQLiteStore) Reset(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	for _, table := range []string{"lot_consumptions", "calc_runs", "transactions"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("truncating %s: %w", table, err)
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) MarkTransactionsSynced(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `UPDATE transactions SET origin = 'sheet' WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("preparing update: %w", err)
	}
	defer stmt.Close()

	for _, id := range ids {
		if _, err := stmt.ExecContext(ctx, id); err != nil {
			return fmt.Errorf("marking transaction %d as synced: %w", id, err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) SaveResults(ctx context.Context, result *engine.CostBasisResult) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)

	// Clear previous results for this asset/method.
	_, err = tx.ExecContext(ctx,
		"DELETE FROM lot_consumptions WHERE asset = ? AND method = ?",
		result.Asset, result.Method,
	)
	if err != nil {
		return fmt.Errorf("clearing old results: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO lot_consumptions (asset, method, sale_transaction_id, lot_transaction_id, quantity_used, cost_basis, calculated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing insert: %w", err)
	}
	defer stmt.Close()

	for _, c := range result.Consumptions {
		_, err := stmt.ExecContext(ctx,
			result.Asset, result.Method,
			c.SaleTransactionID, c.LotTransactionID,
			c.QuantityUsed.String(), c.CostBasis.String(),
			now,
		)
		if err != nil {
			return fmt.Errorf("inserting lot consumption: %w", err)
		}
	}

	// Record the calc run.
	_, err = tx.ExecContext(ctx,
		"INSERT INTO calc_runs (asset, method, calculated_at, txn_count, sale_count) VALUES (?, ?, ?, ?, ?)",
		result.Asset, result.Method, now, len(result.Consumptions), len(result.SaleSummaries),
	)
	if err != nil {
		return fmt.Errorf("recording calc run: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetResults(ctx context.Context, asset string, method string) (*engine.CostBasisResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sale_transaction_id, lot_transaction_id, quantity_used, cost_basis
		FROM lot_consumptions
		WHERE asset = ? AND method = ?
		ORDER BY id ASC
	`, asset, method)
	if err != nil {
		return nil, fmt.Errorf("querying results: %w", err)
	}
	defer rows.Close()

	result := &engine.CostBasisResult{
		Asset:  asset,
		Method: method,
	}

	for rows.Next() {
		var c engine.LotConsumption
		var qtyStr, basisStr string
		if err := rows.Scan(&c.SaleTransactionID, &c.LotTransactionID, &qtyStr, &basisStr); err != nil {
			return nil, fmt.Errorf("scanning lot consumption: %w", err)
		}
		var parseErr error
		c.QuantityUsed, parseErr = engine.DecimalFromString(qtyStr)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing quantity_used: %w", parseErr)
		}
		c.CostBasis, parseErr = engine.DecimalFromString(basisStr)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing cost_basis: %w", parseErr)
		}
		result.Consumptions = append(result.Consumptions, c)
	}

	return result, rows.Err()
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
