# perfi-cli

See [README.md](README.md) for project overview, architecture, and CLI usage.
See [docs/data-model.md](docs/data-model.md) for the SQLite schema and cost basis pipeline.

## Commands

```bash
go build -o perfi .             # build
go test ./...                   # test all packages
go test ./internal/engine/ -v   # test a specific package
```

## Key constraints

- All monetary values use `shopspring/decimal` — never `float64`
- SQLite via `modernc.org/sqlite` (pure Go, no CGO) — all monetary columns stored as TEXT to preserve precision
- Add a cost basis method: implement `Calculator` interface → add case in `NewCalculator()` in `strategy.go`
