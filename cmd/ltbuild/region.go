package main

import (
	"fmt"
	"strings"

	linetypev1 "github.com/tomba-io/phone-line-type-intelligence/proto/linetype/v1"
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

// toProto builds a RegionTable protobuf from the in-memory region data.
func (r *regionRegistry) toProto(region []uint8) *linetypev1.RegionTable {
	regions := make([]*linetypev1.RegionInfo, len(r.codes))
	for i := range r.codes {
		cc, country := "US", "United States"
		if i > 0 && canadianProvinces[r.codes[i]] {
			cc, country = "CA", "Canada"
		}
		regions[i] = &linetypev1.RegionInfo{
			Code:        r.codes[i],
			CountryCode: cc,
			Country:     country,
		}
	}
	// Index 0 is the sentinel — empty code means "no region on file".
	if len(regions) > 0 {
		regions[0] = &linetypev1.RegionInfo{}
	}

	return &linetypev1.RegionTable{
		NxxIndex: region,
		Regions:  regions,
	}
}
