package sheets

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"google.golang.org/api/impersonate"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Client abstracts Google Sheets API interactions for testability.
type Client interface {
	ReadRange(ctx context.Context, spreadsheetID, readRange string) ([][]interface{}, error)
	WriteRange(ctx context.Context, spreadsheetID, writeRange string, values [][]interface{}) error
	AppendRange(ctx context.Context, spreadsheetID, appendRange string, values [][]interface{}) error
}

// GoogleSheetsClient implements Client using the Google Sheets API v4.
// It authenticates by impersonating a service account via the caller's ADC.
type GoogleSheetsClient struct {
	service *sheets.Service
}

// newTokenSource creates an impersonated token source for the given service account.
// It is a package-level variable so tests can replace it.
var newTokenSource = defaultNewTokenSource

func defaultNewTokenSource(ctx context.Context, serviceAccountEmail string) (oauth2.TokenSource, error) {
	ts, err := impersonate.CredentialsTokenSource(ctx, impersonate.CredentialsConfig{
		TargetPrincipal: serviceAccountEmail,
		Scopes:          []string{sheets.SpreadsheetsScope},
	})
	if err != nil {
		return nil, err
	}
	return ts, nil
}

// NewGoogleSheetsClient creates a new client authenticated by impersonating the given service account.
// The caller must have the roles/iam.serviceAccountTokenCreator role on the service account,
// and must be authenticated via gcloud ADC (gcloud auth application-default login).
func NewGoogleSheetsClient(ctx context.Context, serviceAccountEmail string) (*GoogleSheetsClient, error) {
	if serviceAccountEmail == "" {
		return nil, fmt.Errorf("no service_account configured — run 'perfi init' to set up authentication")
	}

	ts, err := newTokenSource(ctx, serviceAccountEmail)
	if err != nil {
		return nil, fmt.Errorf("creating impersonated credentials for %s: %w", serviceAccountEmail, err)
	}

	srv, err := sheets.NewService(ctx, option.WithTokenSource(ts))
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

func (c *GoogleSheetsClient) AppendRange(ctx context.Context, spreadsheetID, appendRange string, values [][]interface{}) error {
	vr := &sheets.ValueRange{
		Values: values,
	}
	_, err := c.service.Spreadsheets.Values.Append(spreadsheetID, appendRange, vr).
		ValueInputOption("RAW").
		InsertDataOption("INSERT_ROWS").
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("appending to range %q: %w", appendRange, err)
	}
	return nil
}
