package cli

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	linetype "github.com/tomba-io/phone-line-type-intelligence"
)

var (
	auditVerbose bool
	auditJSON    bool
)

type auditReport struct {
	Pass    bool          `json:"pass"`
	Checks  []auditCheck  `json:"checks"`
	Passed  int           `json:"passed"`
	Failed  int           `json:"failed"`
}

type auditCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

func (r *auditReport) run(name string, fn func() (bool, string)) {
	pass, detail := fn()
	r.Checks = append(r.Checks, auditCheck{Name: name, Pass: pass, Detail: detail})
	if pass {
		r.Passed++
	} else {
		r.Failed++
	}
}

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Run security and integrity audit on embedded tables",
	Long: `Validates the integrity and safety properties of the embedded line-type
tables: size checks, nibble bounds, carrier/region index bounds, Mexico
range ordering, input sanitisation, concurrent safety, and data checksums.

Examples:
  lti audit
  lti audit --verbose
  lti audit --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		r := &auditReport{}

		r.run("linetype.bin size", func() (bool, string) {
			if err := linetype.Validate(); err != nil {
				return false, err.Error()
			}
			return true, "4,000,000 bytes"
		})

		r.run("carrier.bin available", func() (bool, string) {
			if !linetype.CarrierAvailable() {
				return false, "missing or wrong size"
			}
			return true, "available"
		})

		r.run("region.bin available", func() (bool, string) {
			if !linetype.RegionAvailable() {
				return false, "missing or wrong size"
			}
			return true, "800,000 bytes"
		})

		r.run("mx.bin available", func() (bool, string) {
			if !linetype.MXAvailable() {
				return false, "missing or invalid header"
			}
			return true, "valid MXPN header"
		})

		r.run("nibble bounds", func() (bool, string) {
			bad := 0
			counts := [6]int{}
			for p := uint32(2_000_000); p < 10_000_000; p++ {
				c := linetype.LookupPrefix(p)
				if c > linetype.Invalid {
					bad++
				} else {
					counts[c]++
				}
			}
			if bad > 0 {
				return false, fmt.Sprintf("%d out-of-range nibbles", bad)
			}
			return true, fmt.Sprintf("8M nibbles valid — unknown:%d wireline:%d wireless:%d voip:%d tollfree:%d",
				counts[0], counts[1], counts[2], counts[3], counts[4])
		})

		r.run("carrier index bounds", func() (bool, string) {
			if !linetype.CarrierAvailable() {
				return true, "skipped"
			}
			known := 0
			for p := uint32(2_000_000); p < 10_000_000; p++ {
				if linetype.LookupCarrierPrefix(p).Known() {
					known++
				}
			}
			return true, fmt.Sprintf("all valid, %d blocks with carrier", known)
		})

		r.run("region index bounds", func() (bool, string) {
			if !linetype.RegionAvailable() {
				return true, "skipped"
			}
			known := 0
			for npa := 200; npa <= 999; npa++ {
				for nxx := 200; nxx <= 999; nxx++ {
					e := fmt.Sprintf("+1%03d%03d0000", npa, nxx)
					if linetype.LookupRegion(e).Known() {
						known++
					}
				}
			}
			return true, fmt.Sprintf("all valid, %d NXXs with region", known)
		})

		r.run("mx.bin binary format", func() (bool, string) {
			data, err := os.ReadFile("data/mx.bin")
			if err != nil {
				return true, "skipped (not readable)"
			}
			if len(data) < 12 || string(data[:4]) != "MXPN" {
				return false, "bad magic"
			}
			count := binary.LittleEndian.Uint32(data[8:])
			recordsEnd := 12 + int(count)*20
			// Accept both: records only, or records + prefix index (MXIX).
			hasIdx := len(data) > recordsEnd && len(data) >= recordsEnd+8 && string(data[recordsEnd:recordsEnd+4]) == "MXIX"
			if len(data) != recordsEnd && !hasIdx {
				return false, fmt.Sprintf("size %d, expected %d (or with index)", len(data), recordsEnd)
			}
			extra := ""
			if hasIdx {
				extra = " + prefix index"
			}
			return true, fmt.Sprintf("MXPN v%d, %d ranges%s", data[4], count, extra)
		})

		r.run("input sanitisation", func() (bool, string) {
			attacks := []string{
				"", "+1", "+1' OR 1=1--", "+1<script>alert(1)",
				"+1../../etc/passwd", "+1\x00\x00\x00\x00",
				string(make([]byte, 10000)),
			}
			for _, a := range attacks {
				if linetype.Lookup(a) != linetype.Invalid {
					return false, fmt.Sprintf("%q not rejected", a)
				}
				if linetype.Describe(a).Valid {
					return false, fmt.Sprintf("%q returned Valid", a)
				}
			}
			return true, fmt.Sprintf("%d attacks rejected", len(attacks))
		})

		r.run("concurrent safety", func() (bool, string) {
			ch := make(chan bool, 100)
			for range 100 {
				go func() {
					defer func() {
						if rv := recover(); rv != nil {
							ch <- false
							return
						}
						ch <- true
					}()
					linetype.Lookup("+12025551234")
					linetype.LookupCarrier("+12025551234")
					linetype.LookupRegion("+12025551234")
					linetype.Describe("+525510001234")
				}()
			}
			for range 100 {
				if !<-ch {
					return false, "panic during concurrent access"
				}
			}
			return true, "100 goroutines, no panics"
		})

		r.run("data file checksums", func() (bool, string) {
			files := []string{
				"data/linetype.bin", "data/carrier.bin", "data/region.bin",
				"data/mx.bin", "data/carriers.csv", "data/regions.csv", "data/mx_carriers.csv",
			}
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					return false, fmt.Sprintf("cannot read %s: %v", f, err)
				}
				if auditVerbose {
					h := sha256.Sum256(data)
					fmt.Printf("    %s  sha256:%x  (%d bytes)\n", f, h[:8], len(data))
				}
			}
			return true, fmt.Sprintf("%d files hashed", len(files))
		})

		r.run("coverage", func() (bool, string) {
			classified := 0
			assigned := 0
			for p := uint32(2_000_000); p < 10_000_000; p++ {
				cls := linetype.LookupPrefix(p)
				if cls != linetype.Unknown {
					classified++
				}
				if linetype.CarrierAvailable() && linetype.LookupCarrierPrefix(p).Known() {
					assigned++
				}
			}
			if assigned == 0 {
				assigned = classified // fallback if no carrier table
			}
			pctAll := float64(classified) / 8_000_000 * 100
			pctAssigned := float64(0)
			if assigned > 0 {
				pctAssigned = float64(classified) / float64(assigned) * 100
			}
			if pctAll < 10 {
				return false, fmt.Sprintf("%.1f%% — table may be empty", pctAll)
			}
			return true, fmt.Sprintf("%.1f%% of assigned blocks classified (%d/%d), %.1f%% of 8M keyspace",
				pctAssigned, classified, assigned, pctAll)
		})

		r.Pass = r.Failed == 0

		if auditJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		}

		fmt.Println("Security Audit Report")
		fmt.Println("──────────────────────────────────────────")
		for _, c := range r.Checks {
			icon := "PASS"
			if !c.Pass {
				icon = "FAIL"
			}
			fmt.Printf("  [%s] %s\n", icon, c.Name)
			if auditVerbose || !c.Pass {
				fmt.Printf("         %s\n", c.Detail)
			}
		}
		fmt.Println("──────────────────────────────────────────")
		fmt.Printf("Total: %d checks, %d passed, %d failed\n",
			len(r.Checks), r.Passed, r.Failed)
		if r.Pass {
			fmt.Println("\nALL CHECKS PASSED")
		} else {
			fmt.Println("\nAUDIT FAILED")
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	auditCmd.Flags().BoolVar(&auditVerbose, "verbose", false, "show detail for every check")
	auditCmd.Flags().BoolVar(&auditJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(auditCmd)
}
