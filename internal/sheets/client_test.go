package sheets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClient implements the Client interface for testing command logic
// without hitting the real Google Sheets API.
type mockClient struct {
	readData    [][]interface{}
	readErr     error
	writeErr    error
	appendErr   error
	writeCalls  []writeCall
	appendCalls []writeCall
}

type writeCall struct {
	spreadsheetID string
	writeRange    string
	values        [][]interface{}
}

func (m *mockClient) ReadRange(_ context.Context, spreadsheetID, readRange string) ([][]interface{}, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	return m.readData, nil
}

func (m *mockClient) WriteRange(_ context.Context, spreadsheetID, writeRange string, values [][]interface{}) error {
	m.writeCalls = append(m.writeCalls, writeCall{spreadsheetID, writeRange, values})
	return m.writeErr
}

func (m *mockClient) AppendRange(_ context.Context, spreadsheetID, appendRange string, values [][]interface{}) error {
	m.appendCalls = append(m.appendCalls, writeCall{spreadsheetID, appendRange, values})
	return m.appendErr
}

func TestMockClient_ReadRange(t *testing.T) {
	mock := &mockClient{
		readData: [][]interface{}{
			{"Coinbase", 45292.0, 12.15, 82.25, 1000.0},
		},
	}

	data, err := mock.ReadRange(context.Background(), "sheet-id", "A:E")
	require.NoError(t, err)
	require.Len(t, data, 1)
	assert.Equal(t, "Coinbase", data[0][0])
}

func TestMockClient_ReadRange_Error(t *testing.T) {
	mock := &mockClient{
		readErr: assert.AnError,
	}

	_, err := mock.ReadRange(context.Background(), "sheet-id", "A:E")
	require.Error(t, err)
}

func TestMockClient_WriteRange(t *testing.T) {
	mock := &mockClient{}

	values := [][]interface{}{{"test", 123}}
	err := mock.WriteRange(context.Background(), "sheet-id", "J1", values)
	require.NoError(t, err)
	require.Len(t, mock.writeCalls, 1)
	assert.Equal(t, "sheet-id", mock.writeCalls[0].spreadsheetID)
	assert.Equal(t, "J1", mock.writeCalls[0].writeRange)
}

func TestMockClient_WriteRange_Error(t *testing.T) {
	mock := &mockClient{
		writeErr: assert.AnError,
	}

	err := mock.WriteRange(context.Background(), "sheet-id", "J1", nil)
	require.Error(t, err)
}

// Verify mockClient satisfies the Client interface at compile time.
var _ Client = (*mockClient)(nil)
