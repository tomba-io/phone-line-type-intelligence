# CLAUDE.md

## Project

Phone Line Type Intelligence — classifies +1 (US/Canada) and +52 (Mexico) phone numbers by line type (wireless, wireline, VoIP, toll-free) using embedded numbering-plan data. Zero per-lookup API cost.

## Module

```
github.com/tomba-io/phone-line-type-intelligence
```

Package name is `linetype`. Import as:
```go
import linetype "github.com/tomba-io/phone-line-type-intelligence"
```

## Build & test

```bash
go build ./...                          # build everything
go test ./... -v                        # run all tests
go test -bench=. -benchmem             # run benchmarks
go test -fuzz=FuzzLookup -fuzztime=30s # fuzz test
go run ./cmd/lti classify list.txt      # classify numbers
go run ./cmd/lti describe +18168037763  # single number
go run ./cmd/lti audit --verbose        # security audit
go run ./cmd/lti version                # print version
```

## Project structure

```
├── cmd/
│   ├── lti/              # Cobra CLI entry point (single binary)
│   └── ltbuild/          # Table builder (standalone, no cobra)
├── internal/
│   └── cli/              # Cobra subcommands (classify, describe, audit, build, fetch)
├── scripts/
│   ├── get-data.sh       # Download NANPA/CNAC/IFT data
│   └── .proxyrc.example  # Proxy template for Imperva bypass
├── data/                 # Embedded binary tables (go:embed)
├── examples/             # Usage examples
│   ├── basic/
│   └── csv-classify/
├── linetype.go           # Core: Class enum, Lookup(), Validate()
├── carrier.go            # Carrier table (16 MB, lazy CSV)
├── mx.go                 # Mexico range table (binary search)
├── region.go             # Region table (per-NXX, lazy CSV)
├── regionnames.go        # State/province name map
├── describe.go           # Number struct, Describe()
├── doc.go                # Package documentation
└── linetype_test.go      # Tests, benchmarks, fuzz, examples
```

## CLI commands (Cobra)

```
lti classify [file]       Classify numbers from file/stdin
lti describe <e164>       Describe a single number (--json)
lti audit                 Security audit (--verbose, --json)
lti build -- [flags]      Build tables via ltbuild
lti fetch                 Download source data
lti version               Print version
lti completion [shell]    Generate shell completions
```

## Key invariants

- **Abstention over guessing** — `Unknown` is never a synonym for landline
- **CLEC → VoIP** — not wireline; CLECs are predominantly SMS-reachable today
- **Block-level writes are unconditional** — block data always overrides NXX data
- **Zero allocation in hot paths** — Lookup, LookupCarrier, LookupRegion, Describe
- **No external dependencies in library code** — cobra is CLI-only (internal/cli)
