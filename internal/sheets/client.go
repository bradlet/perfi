package sheets

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Client abstracts Google Sheets API interactions for testability.
type Client interface {
	ReadRange(ctx context.Context, spreadsheetID, readRange string) ([][]interface{}, error)
	WriteRange(ctx context.Context, spreadsheetID, writeRange string, values [][]interface{}) error
}

// GoogleSheetsClient implements Client using the Google Sheets API v4.
// It uses Application Default Credentials for authentication.
type GoogleSheetsClient struct {
	service *sheets.Service
}

// NewGoogleSheetsClient creates a new client authenticated via ADC.
// Requires: gcloud auth application-default login --scopes=https://www.googleapis.com/auth/spreadsheets
func NewGoogleSheetsClient(ctx context.Context) (*GoogleSheetsClient, error) {
	srv, err := sheets.NewService(ctx, option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		return nil, fmt.Errorf("creating sheets service: %w", err)
	}
	return &GoogleSheetsClient{service: srv}, nil
}

func (c *GoogleSheetsClient) ReadRange(ctx context.Context, spreadsheetID, readRange string) ([][]interface{}, error) {
	resp, err := c.service.Spreadsheets.Values.Get(spreadsheetID, readRange).
		ValueRenderOption("UNFORMATTED_VALUE").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("reading range %q: %w", readRange, err)
	}
	return resp.Values, nil
}

func (c *GoogleSheetsClient) WriteRange(ctx context.Context, spreadsheetID, writeRange string, values [][]interface{}) error {
	vr := &sheets.ValueRange{
		Values: values,
	}
	_, err := c.service.Spreadsheets.Values.Update(spreadsheetID, writeRange, vr).
		ValueInputOption("RAW").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("writing range %q: %w", writeRange, err)
	}
	return nil
}
