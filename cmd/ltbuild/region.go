package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

// Region data is keyed per NXX, not per block.
//
// A state or province is a property of the rate centre, and every thousands
// block inside one NXX shares a rate centre — a pooled NXX is split between
// carriers, never between provinces. Storing it per block would cost 8 MB to
// hold ten identical copies of each value; per NXX it costs 800 KB.
const (
	regionBase = 200_000 // lowest NPANXX: NPA 200, NXX 200
	regionCnt  = 800_000 // through 999999
)

// canadianProvinces distinguishes CNAC rows from NANPA rows by their region
// code. This is a closed, stable set, so it is a lookup and not a guess.
// Everything else is US or a US territory (PR, VI, GU, AS, MP).
var canadianProvinces = map[string]bool{
	"AB": true, "BC": true, "MB": true, "NB": true, "NL": true, "NS": true,
	"NT": true, "NU": true, "ON": true, "PE": true, "QC": true, "SK": true,
	"YT": true,
}

var stateAliases = []string{"state", "province", "state/province", "st"}

// regionRegistry interns region codes into a byte index. There are ~60 US
// states and territories plus 13 provinces, so a byte is ample; index 0 means
// no region on file.
type regionRegistry struct {
	index map[string]uint8
	codes []string
}

func newRegionRegistry() *regionRegistry {
	return &regionRegistry{index: map[string]uint8{}, codes: []string{""}}
}

func (r *regionRegistry) intern(code string) (uint8, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return 0, nil
	}
	if i, ok := r.index[code]; ok {
		return i, nil
	}
	if len(r.codes) > 0xFF {
		return 0, fmt.Errorf("more than %d distinct regions; the index no longer fits in a byte", 0xFF)
	}
	i := uint8(len(r.codes))
	r.index[code] = i
	r.codes = append(r.codes, code)
	return i, nil
}

func (r *regionRegistry) writeBlob(region []uint8, path string) error {
	if len(region) != regionCnt {
		return fmt.Errorf("region table has %d slots, want %d", len(region), regionCnt)
	}
	if err := os.WriteFile(path, region, 0o644); err != nil {
		return fmt.Errorf("region blob: %w", err)
	}
	return nil
}

// writeTable emits index -> code, country, country_code. The country is
// derived from the region code, not from which file the row came from, so a
// Canadian row appearing in a US file (or vice versa) still lands correctly.
func (r *regionRegistry) writeTable(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("region table: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"index", "region", "country_code", "country"}); err != nil {
		return fmt.Errorf("region table: %w", err)
	}
	for i := 1; i < len(r.codes); i++ {
		cc, country := "US", "United States"
		if canadianProvinces[r.codes[i]] {
			cc, country = "CA", "Canada"
		}
		if err := w.Write([]string{fmt.Sprint(i), r.codes[i], cc, country}); err != nil {
			return fmt.Errorf("region table: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}
