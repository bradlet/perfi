package workflow

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/bradlet/perfi/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSheetsClient implements sheets.Client for testing.
type mockSheetsClient struct {
	readData   [][]interface{}
	readErr    error
	writeErr   error
	writeCount int
}

func (m *mockSheetsClient) ReadRange(_ context.Context, _, _ string) ([][]interface{}, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	return m.readData, nil
}

func (m *mockSheetsClient) WriteRange(_ context.Context, _, _ string, _ [][]interface{}) error {
	m.writeCount++
	return m.writeErr
}

func TestRunner_Run_FullPipeline(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.Init(context.Background()))
	defer store.Close()

	mock := &mockSheetsClient{
		readData: [][]interface{}{
			{"Coinbase", 45292.0, 10.0, 100.0, 1000.0},
			{"Coinbase", 45322.0, -5.0, 120.0, 600.0},
		},
	}

	var buf bytes.Buffer
	runner := &Runner{
		SheetsClient: mock,
		Store:        store,
		Out:          &buf,
	}

	err = runner.Run(context.Background(), RunParams{
		SpreadsheetID: "test-sheet",
		ReadRange:     "A:E",
		WriteRange:    "J1",
		Asset:         "SOL",
		Method:        "fifo",
	})
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Synced 2 transactions")
	assert.Contains(t, output, "Calculated 1 sale summaries")
	assert.Contains(t, output, "Pushed 1 sale summaries")
	assert.Equal(t, 1, mock.writeCount)
}

func TestRunner_Run_DryRun(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.Init(context.Background()))
	defer store.Close()

	mock := &mockSheetsClient{
		readData: [][]interface{}{
			{"Coinbase", 45292.0, 10.0, 100.0, 1000.0},
			{"Coinbase", 45322.0, -5.0, 120.0, 600.0},
		},
	}

	var buf bytes.Buffer
	runner := &Runner{
		SheetsClient: mock,
		Store:        store,
		Out:          &buf,
	}

	err = runner.Run(context.Background(), RunParams{
		SpreadsheetID: "test-sheet",
		ReadRange:     "A:E",
		WriteRange:    "J1",
		Asset:         "SOL",
		Method:        "fifo",
		DryRun:        true,
	})
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "Dry run")
	assert.Equal(t, 0, mock.writeCount, "dry run should not write")
}

func TestRunner_Run_NoSales(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.Init(context.Background()))
	defer store.Close()

	mock := &mockSheetsClient{
		readData: [][]interface{}{
			{"Coinbase", 45292.0, 10.0, 100.0, 1000.0},
		},
	}

	var buf bytes.Buffer
	runner := &Runner{
		SheetsClient: mock,
		Store:        store,
		Out:          &buf,
	}

	err = runner.Run(context.Background(), RunParams{
		SpreadsheetID: "test-sheet",
		ReadRange:     "A:E",
		WriteRange:    "J1",
		Asset:         "SOL",
		Method:        "fifo",
	})
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No sales found")
	assert.Equal(t, 0, mock.writeCount)
}

func TestRunner_Run_ReadError(t *testing.T) {
	store, err := storage.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, store.Init(context.Background()))
	defer store.Close()

	mock := &mockSheetsClient{
		readErr: fmt.Errorf("API error"),
	}

	runner := &Runner{
		SheetsClient: mock,
		Store:        store,
		Out:          &bytes.Buffer{},
	}

	err = runner.Run(context.Background(), RunParams{
		SpreadsheetID: "test-sheet",
		ReadRange:     "A:E",
		WriteRange:    "J1",
		Asset:         "SOL",
		Method:        "fifo",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading sheet")
}
