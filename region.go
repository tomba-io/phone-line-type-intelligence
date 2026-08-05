package linetype

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"io"
	"strconv"
	"sync"
)

const (
	regionBase    = 200_000
	regionCount   = 800_000
	regionBlobLen = regionCount
)

//go:embed data/region.bin
var regionBlob []byte

//go:embed data/regions.csv
var regionTableCSV []byte

// Region is the geography a number range is assigned to.
type Region struct {
	Code        string // state or province code, e.g. "MO", "ON"
	Name        string // full name, e.g. "Missouri", "Ontario"
	Country     string // e.g. "United States"
	CountryCode string // ISO 3166-1 alpha-2, e.g. "US"
}

// Known reports whether any region was on file.
func (r Region) Known() bool { return r.Code != "" }

var (
	regionOnce  sync.Once
	regionByIdx []Region
)

func loadRegions() {
	regionByIdx = []Region{{}}
	r := csv.NewReader(bytes.NewReader(regionTableCSV))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return
		}
		if len(rec) < 4 {
			continue
		}
		if first {
			first = false
			if rec[0] == "index" {
				continue
			}
		}
		idx, err := strconv.Atoi(rec[0])
		if err != nil || idx < 1 {
			continue
		}
		for len(regionByIdx) <= idx {
			regionByIdx = append(regionByIdx, Region{})
		}
		regionByIdx[idx] = Region{
			Code:        rec[1],
			Name:        regionNames[rec[1]],
			CountryCode: rec[2],
			Country:     rec[3],
		}
	}
}

// RegionAvailable reports whether a region table was built into this binary.
func RegionAvailable() bool {
	return len(regionBlob) == regionBlobLen && len(regionTableCSV) > 0
}

// LookupRegion returns the geography of an E.164 +1 number's NXX.
func LookupRegion(e164 string) Region {
	k, ok := prefixKey(e164)
	if !ok {
		return Region{}
	}
	if len(regionBlob) != regionBlobLen {
		return Region{}
	}
	i := k/10 - regionBase
	if i >= uint32(len(regionBlob)) {
		return Region{}
	}
	idx := int(regionBlob[i])
	if idx == 0 {
		return Region{}
	}
	regionOnce.Do(loadRegions)
	if idx >= len(regionByIdx) {
		return Region{}
	}
	return regionByIdx[idx]
}
