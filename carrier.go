package linetype

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/csv"
	"io"
	"strconv"
	"sync"
)

// Legacy flat format: 2 bytes per block, 8M blocks = 16 MB.
const carrierBlobLenFlat = keyCount * 2

// Compact format constants.
const (
	carrierMagic      = "CXNB"
	carrierHdrLen     = 10 // magic(4) + version(2) + exCount(4)
	carrierNXXCount   = 800_000
	carrierNXXTblLen  = carrierNXXCount * 2 // uint16 per NXX
	carrierExSize     = 6                   // nxxOff(3) + block(1) + carrier(2)
)

//go:embed data/carrier.bin
var carrierBlob []byte

//go:embed data/carriers.csv
var carrierTableCSV []byte

// Carrier identifies the company holding a number range, as published in the
// assignment data. It is the *block holder*, not the current service provider.
type Carrier struct {
	OCN   string // operating company number, e.g. "6529"
	Name  string // company name as published, e.g. "T-MOBILE USA, INC."
	Brand string // optional human-facing name, e.g. "T-Mobile"
}

// Known reports whether any carrier was on file.
func (c Carrier) Known() bool { return c.OCN != "" }

// Label returns the brand if available, otherwise the published name.
func (c Carrier) Label() string {
	if c.Brand != "" {
		return c.Brand
	}
	return c.Name
}

func (c Carrier) String() string {
	if c.Brand != "" {
		return c.Brand
	}
	if c.Name != "" {
		return c.Name
	}
	if c.OCN != "" {
		return c.OCN
	}
	return "unknown"
}

var (
	carrierOnce    sync.Once
	carrierByIdx   []Carrier
	carrierFmt     int  // cached format: 0=invalid, 1=flat, 2=compact
	carrierFmtOK   bool // whether carrierFmt has been set
	carrierExCount int  // cached exception count for compact format
	carrierExStart int  // cached exception list offset
)

func loadCarriers() {
	carrierByIdx = []Carrier{{}}
	r := csv.NewReader(bytes.NewReader(carrierTableCSV))
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
		if len(rec) < 3 {
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
		for len(carrierByIdx) <= idx {
			carrierByIdx = append(carrierByIdx, Carrier{})
		}
		brand := ""
		if len(rec) > 3 {
			brand = rec[3]
		}
		carrierByIdx[idx] = Carrier{OCN: rec[1], Name: rec[2], Brand: brand}
	}
}

// CarrierAvailable reports whether a carrier table was built into this binary.
func CarrierAvailable() bool {
	return carrierFormat() != 0 && len(carrierTableCSV) > 0
}

// carrierFormat returns 1 for flat (legacy), 2 for compact, 0 for invalid.
// The result is cached after first call.
func carrierFormat() int {
	if carrierFmtOK {
		return carrierFmt
	}
	if len(carrierBlob) == carrierBlobLenFlat {
		carrierFmt = 1
	} else if len(carrierBlob) >= carrierHdrLen && string(carrierBlob[:4]) == carrierMagic {
		carrierFmt = 2
		carrierExCount = int(binary.LittleEndian.Uint32(carrierBlob[6:]))
		carrierExStart = carrierHdrLen + carrierNXXTblLen
	}
	carrierFmtOK = true
	return carrierFmt
}

// LookupCarrier returns the company holding the range an E.164 +1 number falls in.
func LookupCarrier(e164 string) Carrier {
	k, ok := prefixKey(e164)
	if !ok {
		return Carrier{}
	}
	return carrierAt(k)
}

// LookupCarrierPrefix is LookupCarrier for a seven-digit NPA-NXX-B prefix.
func LookupCarrierPrefix(prefix uint32) Carrier {
	if prefix < keyBase || prefix >= 10_000_000 {
		return Carrier{}
	}
	return carrierAt(prefix)
}

func carrierAt(k uint32) Carrier {
	var idx int
	switch carrierFormat() {
	case 1: // flat: 2 bytes per block
		i := (k - keyBase) * 2
		idx = int(carrierBlob[i]) | int(carrierBlob[i+1])<<8
	case 2: // compact: NXX base + exceptions
		idx = carrierCompact(k)
	default:
		return Carrier{}
	}
	if idx == 0 {
		return Carrier{}
	}
	carrierOnce.Do(loadCarriers)
	if idx >= len(carrierByIdx) {
		return Carrier{}
	}
	return carrierByIdx[idx]
}

// carrierCompact reads the compact format: NXX base table + sparse exceptions.
func carrierCompact(k uint32) int {
	nxxOff := (k - keyBase) / 10
	block := byte((k - keyBase) % 10)

	// Read NXX base carrier.
	off := uint32(carrierHdrLen) + nxxOff*2
	baseIdx := int(binary.LittleEndian.Uint16(carrierBlob[off:]))

	if carrierExCount == 0 {
		return baseIdx
	}

	// Binary search the exception list for (nxxOff, block).
	lo, hi := 0, carrierExCount
	for lo < hi {
		mid := lo + (hi-lo)/2
		off := carrierExStart + mid*carrierExSize
		// Read nxxOff as uint24 LE.
		eNxx := uint32(carrierBlob[off]) | uint32(carrierBlob[off+1])<<8 | uint32(carrierBlob[off+2])<<16
		eBlk := carrierBlob[off+3]
		key := eNxx*10 + uint32(eBlk)
		target := nxxOff*10 + uint32(block)
		if key == target {
			return int(binary.LittleEndian.Uint16(carrierBlob[off+4:]))
		}
		if key < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return baseIdx
}
