# Contributing

## Data Refresh Cycle

```bash
# 1. Download source data
lti fetch

# 2. Build US + Canada tables
lti build -- -co _build/linetype/<co-file> \
    -blocks _build/linetype/<blocks-file> \
    -ocn data/ocn.csv -out data/linetype.bin \
    -carrier-out data/carrier.bin -carriers-out data/carriers.csv \
    -region-out data/region.bin -regions-out data/regions.csv

# 3. Build Mexico table
lti build -- -mx _build/linetype/pnn_Publico_*.csv \
    -mx-out data/mx.bin -mx-carriers-out data/mx_carriers.csv

# 4. Re-embed and test
go build ./...
go test ./... -v
lti classify list.txt
lti audit --verbose
```

## Running Tests

```bash
go test ./... -v              # unit tests
go test -bench=. -benchmem    # benchmarks
go test -fuzz=FuzzLookup      # fuzz testing
lti audit                     # security audit
```

## Project Structure

```
├── cmd/
│   ├── lti/              # Cobra CLI entry point
│   └── ltbuild/          # Table builder (standalone)
├── internal/
│   └── cli/              # Cobra subcommands
│       ├── root.go       # Root command + version
│       ├── classify.go   # lti classify
│       ├── describe.go   # lti describe
│       ├── audit.go      # lti audit
│       └── build.go      # lti build + lti fetch
├── scripts/
│   ├── get-data.sh       # Source data downloader
│   └── .proxyrc.example  # Proxy template
├── data/                 # Embedded tables (go:embed)
├── examples/             # Usage examples
├── linetype.go           # Core: Class, Lookup, Validate
├── carrier.go            # Carrier table
├── mx.go                 # Mexico range table
├── region.go             # Region table
├── regionnames.go        # State/province names
├── describe.go           # Number struct, Describe()
├── doc.go                # Package documentation
└── linetype_test.go      # Tests, benchmarks, fuzz, examples
```

## Code Style

- Library code (root package) has **zero external dependencies**
- CLI code (internal/cli) uses Cobra — the only external dep
- Zero allocation in hot paths (`Lookup`, `LookupCarrier`, `LookupRegion`, `Describe`)
- `sync.Once` for lazy loading of CSV tables
- Graceful degradation: missing/truncated table → `Unknown`/empty, never panic
