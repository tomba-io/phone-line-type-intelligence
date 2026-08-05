# Phone Line Type Intelligence

[![Go Reference](https://pkg.go.dev/badge/github.com/tomba-io/phone-line-type-intelligence.svg)](https://pkg.go.dev/github.com/tomba-io/phone-line-type-intelligence)
[![CI](https://github.com/tomba-io/phone-line-type-intelligence/actions/workflows/ci.yml/badge.svg)](https://github.com/tomba-io/phone-line-type-intelligence/actions/workflows/ci.yml)

Use Line Type Intelligence to identify the carrier and phone line type, such as
mobile, landline, fixed VoIP, non-fixed VoIP, toll free, and more — with no
per-lookup API cost.

Classifies +1 (US, Canada) and +52 (Mexico) numbers from published
numbering-plan allocation data embedded directly in the binary.

## Line Types

| Type         | Description                                                               | SMS Reachable |
| ------------ | ------------------------------------------------------------------------- | :-----------: |
| **wireless** | Mobile / cellular (PCS, wireless carriers)                                |      Yes      |
| **wireline** | Landline (ILEC, RBOC, traditional fixed-line)                             |      No       |
| **voip**     | VoIP and CLEC-held ranges (interconnected VoIP, fixed and non-fixed VoIP) |      Yes      |
| **tollfree** | Toll-free numbers (800, 888, 877, 866, 855, 844, 833)                     |      No       |
| **unknown**  | No assignment on file or unmapped OCN — never a synonym for landline      |       —       |

## Installation

```bash
go get github.com/tomba-io/phone-line-type-intelligence
```

## Quick Start

```go
package main

import (
    "fmt"
    linetype "github.com/tomba-io/phone-line-type-intelligence"
)

func main() {
    n := linetype.Describe("+18168037763")

    fmt.Println(n.Class)           // wireless
    fmt.Println(n.SMSReachable)    // true
    fmt.Println(n.Carrier.Label()) // AT&T
    fmt.Println(n.Region.Code)     // MO
    fmt.Println(n.Region.Name)     // Missouri
    fmt.Println(n.Country)         // United States
    fmt.Println(n.International()) // +1 816-803-7763
    fmt.Println(n.National())      // (816) 803-7763
}
```

### Direct Lookups

```go
// Line type only (zero allocation, 11 ns)
cls := linetype.Lookup("+14155551234") // linetype.Wireless

// Carrier info (zero allocation, 60 ns)
cr := linetype.LookupCarrier("+14155551234")
fmt.Println(cr.Label()) // "AT&T"
fmt.Println(cr.OCN)     // "6006"

// Geographic region (zero allocation, 12 ns)
rg := linetype.LookupRegion("+14155551234")
fmt.Println(rg.Code, rg.Name) // "CA" "California"
```

### Mexico

```go
mx := linetype.Describe("+525510001234")
fmt.Println(mx.Class)   // wireless or wireline
fmt.Println(mx.Country) // Mexico
fmt.Println(mx.NPA)     // 55 (Mexico City)
```

### CLI

A single `lti` binary powered by [Cobra](https://github.com/spf13/cobra):

```bash
# Install
go install github.com/tomba-io/phone-line-type-intelligence/cmd/lti@latest

# Describe a single number
$ lti describe +18168037763
Number:        +18168037763
International: +1 816-803-7763
National:      (816) 803-7763
Line Type:     wireless
SMS Reachable: Yes
Carrier:       AT&T
OCN:           6534
Region:        Missouri
State:         MO
Country:       United States (US)

# JSON output
$ lti describe --json +18168037763
```

```json
{
  "block": "7",
  "carrier_brand": "AT\u0026T",
  "carrier_label": "AT\u0026T",
  "carrier_name": "NEW CINGULAR WIRELESS PCS, LLC - IL",
  "carrier_ocn": "6534",
  "country": "United States",
  "country_code": "US",
  "e164": "+18168037763",
  "international": "+1 816-803-7763",
  "line_type": "wireless",
  "national": "(816) 803-7763",
  "npa": "816",
  "nxx": "803",
  "region_code": "MO",
  "region_name": "Missouri",
  "sms_reachable": true,
  "valid": true
}
```

```bash
# Classify a file
$ lti classify list.txt
lti classify summary
   input lines         2377   in 1ms
   ---
   wireless        1326    55.8%  #####.....
   voip             191     8.0%  ..........
   wireline         840    35.3%  ###.......
   unknown           20     0.8%  ..........
   ---
   emitted             2377

# Security audit
$ lti audit --verbose

# Download source data
$ lti fetch

# Build tables
$ lti build -- -co cocodes.txt -blocks blocks.txt -out data/linetype.bin

# Shell completions
$ lti completion bash > /etc/bash_completion.d/lti
$ lti completion zsh > "${fpath[1]}/_lti"
```

## Coverage

### +1 (US and Canada) — complete

|               | Assigned blocks |    Classified |    Coverage |
| ------------- | --------------: | ------------: | ----------: |
| United States |       1,980,190 |     1,980,190 | **100.00%** |
| Canada        |         286,668 |       286,668 | **100.00%** |
| **Total**     |   **2,267,088** | **2,267,088** | **100.00%** |

All 3,117 OCNs holding blocks are classified.

Class distribution:

|          |    US | Canada |
| -------- | ----: | -----: |
| wireless | 37.0% |  29.5% |
| voip     | 36.8% |  44.3% |
| wireline | 26.2% |  26.2% |

### +52 (Mexico) — complete

|                        |           Numbers |    Share |
| ---------------------- | ----------------: | -------: |
| Mobile (`CPP` + `MPP`) |       876,678,000 |    85.8% |
| Fixed (`FIJO`)         |       145,582,662 |    14.2% |
| **Total classified**   | **1,022,260,662** | **100%** |

## Data Sources

| Country    | Source                                                             | Line Type                       | Coverage                |
| ---------- | ------------------------------------------------------------------ | ------------------------------- | ----------------------- |
| **US**     | [NANPA](https://www.nationalnanpa.com) CO codes + thousands blocks | Derived from OCN category       | 100% of assigned blocks |
| **Canada** | [CNAC](https://cnac.ca) CO codes + blocks                          | Derived from OCN category       | 100% of assigned blocks |
| **Mexico** | [IFT](https://sns.ift.org.mx) Plan Nacional de Numeracion          | Published directly (Fijo/Movil) | 100% (1B+ numbers)      |

## Embedded Tables

| Table          | Size     | Format                                                      |
| -------------- | -------- | ----------------------------------------------------------- |
| `linetype.bin` | 4 MB     | 4-bit class per thousands-block, 8M slots, O(1)             |
| `carrier.bin`  | 4.5 MB   | NXX base + block exceptions (compact), O(1)/O(log n)        |
| `carriers.csv` | 128 KB   | OCN to name and brand mapping                               |
| `region.bin`   | 800 KB   | 1 byte per NXX, state/province                              |
| `mx.bin`       | 3.5 MB   | Sorted ranges + 3-digit prefix index, O(log n)              |

## Performance

All lookups are zero-allocation. Formatting methods (`International()`,
`National()`) are computed lazily — you only pay for them when called.

| Operation           | Time   | Allocations | Notes                                       |
| ------------------- | ------ | ----------- | ------------------------------------------- |
| `Lookup()`          | 11 ns  | 0           | Line type only, single array index          |
| `LookupCarrier()`   | 60 ns  | 0           | NXX base + exception search (compact)       |
| `LookupRegion()`    | 12 ns  | 0           | State/province per NXX                      |
| `Describe()`        | 90 ns  | 0           | Full result: class + carrier + region       |
| `Describe()` Mexico | 74 ns  | 0           | Binary search with 3-digit prefix index     |
| Bulk (2377 numbers) | 585 us | 0           | 4 million classifications/sec               |
| Parallel (6 cores)  | 17 ns  | 0           | No contention on concurrent access          |

## Tests

```bash
go test ./... -v        # 22 tests + fuzz seeds + example tests
go test -fuzz=FuzzLookup -fuzztime=30s   # fuzz testing
```

| Test                        | What it verifies                                                   |
| --------------------------- | ------------------------------------------------------------------ |
| `TestValidate`              | Embedded linetype.bin is exactly 4,000,000 bytes                   |
| `TestCarrierAvailable`      | Carrier table loaded correctly                                     |
| `TestRegionAvailable`       | Region table (800 KB) loaded correctly                             |
| `TestMXAvailable`           | Mexico range table has valid MXPN magic header                     |
| `TestLookupInvalid`         | 11 invalid inputs all return `Invalid` class                       |
| `TestLookupPrefix`          | Out-of-range prefixes return `Unknown`                             |
| `TestDescribeUSFormatting`  | NPA/NXX/Block parsing, International/National format               |
| `TestDescribeInvalid`       | Non-NANP/MX numbers return `Valid=false`                           |
| `TestDescribeMexico`        | 5 Mexican area codes (55, 56, 81, 33, 998)                         |
| `TestMexicoInvalid`         | Wrong-length and 0-leading MX numbers rejected                     |
| `TestMexicoRangeEdges`      | Boundary values at range start/end                                 |
| `TestClassString`           | All Class values serialize correctly                               |
| `TestClassMarshalText`      | JSON marshaling matches String()                                   |
| `TestSMSReachable`          | Wireless+VoIP reachable, others not                                |
| `TestLookupCarrier`         | Carrier lookup returns valid types                                 |
| `TestCarrierString`         | Brand > Name > OCN > "unknown" precedence                          |
| `TestCarrierLabel`          | Brand > Name precedence                                            |
| `TestLookupRegion`          | NY/New York/US for 212 numbers                                     |
| `TestHelpers`               | YesNo() and Or() utility functions                                 |
| `TestGoldenListTxt`         | **Exact match**: 1326 wireless, 191 voip, 840 wireline, 20 unknown |
| `TestConcurrency`           | 10 goroutines hitting all paths simultaneously                     |
| `FuzzLookup`                | Random inputs never panic (6.7M+ executions)                       |
| `ExampleDescribe`           | Verified example in go doc                                         |
| `ExampleLookup`             | Verified example in go doc                                         |
| `ExampleLookup_invalid`     | Verified example in go doc                                         |
| `ExampleClass_SMSReachable` | Verified example in go doc                                         |

## Benchmarks

```bash
go test -bench=. -benchmem -count=3
```

```
BenchmarkLookup-6           100000000    11.0 ns/op    0 B/op   0 allocs/op
BenchmarkDescribe-6          13000000    90.1 ns/op    0 B/op   0 allocs/op
BenchmarkDescribeMexico-6    16200000    74.4 ns/op    0 B/op   0 allocs/op
BenchmarkLookupCarrier-6     20000000    59.6 ns/op    0 B/op   0 allocs/op
BenchmarkLookupRegion-6     100000000    11.6 ns/op    0 B/op   0 allocs/op
BenchmarkBulkThroughput-6        2156   585.2 us/op    0 B/op   0 allocs/op   2377 numbers/op
BenchmarkParallel-6          62000000    17.0 ns/op    0 B/op   0 allocs/op
```

## Building the Tables

```bash
# Using make (recommended)
make data-fetch     # download NANPA, CNAC, IFT source data
make data-build     # build all tables (US, CA, MX)
make test           # verify
make audit          # integrity check

# Or manually
lti fetch
lti build -- -co _build/linetype/CoCodeAssignment_*.txt \
    -blocks _build/linetype/ThousandsBlockAssignment_*.txt \
    -ocn data/ocn.csv -out data/linetype.bin \
    -carrier-out data/carrier.bin -carriers-out data/carriers.csv \
    -region-out data/region.bin -regions-out data/regions.csv

lti build -- -mx _build/linetype/pnn_Publico_*.csv \
    -mx-out data/mx.bin -mx-carriers-out data/mx_carriers.csv

go build ./...
```

### Geographic Blocking (.proxyrc)

`reports.nanpa.com` sits behind Imperva, which refuses requests from certain
countries with HTTP 403. If `lti fetch` fails:

```bash
cp scripts/.proxyrc.example scripts/.proxyrc
# edit .proxyrc with your proxy credentials — NEVER commit this file
lti fetch
```

## Accuracy

See [ACCURACY.md](ACCURACY.md) for the full accuracy analysis, measurement
protocol, and the distinction between assignment accuracy and porting error.

> **This is block assignment data, not current line type.** It reports the
> original carrier allocation and is wrong on every ported number. `unknown`
> means no assignment is on file and is never a synonym for landline. Not a
> basis for TCPA or DNC claims.

## License

Please see the [Apache 2.0 license](http://www.apache.org/licenses/LICENSE-2.0.html) file for more information.
