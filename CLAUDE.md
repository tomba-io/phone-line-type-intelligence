# CLAUDE.md

## Project

Phone Line Type Intelligence — classifies +1 (US/Canada) and +52 (Mexico) phone numbers by line type (wireless, wireline, VoIP, toll-free) using numbering-plan data stored as Protocol Buffers. Zero per-lookup API cost.

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
make proto                              # regenerate protobuf Go code
go run ./cmd/lti classify list.txt      # classify numbers
go run ./cmd/lti describe +18168037763  # single number
go run ./cmd/lti audit --verbose        # security audit
go run ./cmd/lti version                # print version
```

## Project structure

```
├── cmd/
│   ├── lti/                  # Cobra CLI entry point (single binary)
│   ├── ltbuild/              # Table builder (standalone, no cobra)
│   └── convert-to-proto/     # One-time migration tool (legacy → proto)
├── proto/
│   └── linetype/v1/          # Protocol Buffers definitions
│       ├── linetype.proto    # Message/enum definitions
│       └── linetype.pb.go   # Generated Go code (committed)
├── internal/
│   └── cli/                  # Cobra subcommands (classify, describe, audit, build, fetch)
├── scripts/
│   ├── get-data.sh           # Download NANPA/CNAC/IFT data
│   └── .proxyrc.example      # Proxy template for Imperva bypass
├── data/
│   ├── phone_data.pb         # All lookup tables (protobuf, loaded at runtime)
│   ├── ocn.csv               # OCN classification map (build-time input)
│   └── mx_brands.csv         # Mexico operator brands (build-time input)
├── examples/                 # Usage examples
│   ├── basic/
│   └── csv-classify/
├── linetype.go               # Core: Class enum, Lookup(), Validate()
├── loader.go                 # Data loading: SetDataPath(), LoadData(), ensureLoaded()
├── carrier.go                # Carrier struct, LookupCarrier()
├── mx.go                     # Mexico range lookup (binary search)
├── region.go                 # Region struct, LookupRegion()
├── regionnames.go            # State/province name map
├── describe.go               # Number struct, Describe()
├── doc.go                    # Package documentation
└── linetype_test.go          # Tests, benchmarks, fuzz, examples
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

## Data format (Protocol Buffers)

All lookup data is in `data/phone_data.pb` — a single `PhoneData` message containing:
- `ClassTable` — nibble-packed US/CA class data (4 MB bytes field)
- `CarrierTable` — NXX-base + exceptions + carrier directory
- `RegionTable` — per-NXX region index + region directory
- `MXTable` — sorted ranges + prefix index + carrier directory

Data path resolution: `SetDataPath()` > `LINETYPE_DATA_PATH` env > `<exe_dir>/data/phone_data.pb` > `./data/phone_data.pb`

Proto definitions: `proto/linetype/v1/linetype.proto`

## Key invariants

- **Abstention over guessing** — `Unknown` is never a synonym for landline
- **CLEC → VoIP** — not wireline; CLECs are predominantly SMS-reachable today
- **Block-level writes are unconditional** — block data always overrides NXX data
- **Zero allocation in hot paths** — Lookup, LookupCarrier, LookupRegion, Describe
- **Protobuf bytes fields for bulk data** — ClassTable.data and RegionTable.nxx_index are raw byte arrays for O(1) lookup
