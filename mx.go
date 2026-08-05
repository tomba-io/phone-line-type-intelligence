package linetype

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/csv"
	"io"
	"sort"
	"strconv"
	"sync"
)

const (
	mxMagic      = "MXPN"
	mxHeaderLen  = 12
	mxRecordSize = 20

	// Prefix index appended after the records.
	mxIdxMagic     = "MXIX"
	mxIdxHeaderLen = 8         // magic(4) + bucketCount(2) + reserved(2)
	mxBucketSize   = 8         // startIdx(4) + endIdx(4)
	mxBucketCount  = 800       // 3-digit prefixes 200..999
	mxBucketBase   = 200       // first bucket prefix
)

//go:embed data/mx.bin
var mxBlob []byte

//go:embed data/mx_carriers.csv
var mxCarrierCSV []byte

var (
	mxOnce      sync.Once
	mxCount     int
	mxCarriers  []Carrier
	mxUsable    bool
	mxCarrierOK bool
	mxBuckets   [mxBucketCount][2]uint32 // [start, end) per 3-digit prefix
	mxHasIndex  bool
)

func mxInit() {
	if len(mxBlob) < mxHeaderLen || string(mxBlob[:4]) != mxMagic {
		return
	}
	count := int(binary.LittleEndian.Uint32(mxBlob[8:]))
	recordsEnd := mxHeaderLen + count*mxRecordSize
	if len(mxBlob) != recordsEnd && len(mxBlob) != recordsEnd+mxIdxHeaderLen+mxBucketCount*mxBucketSize {
		return
	}
	mxCount = count
	mxUsable = true

	// Read prefix index if present.
	if len(mxBlob) >= recordsEnd+mxIdxHeaderLen+mxBucketCount*mxBucketSize {
		idxOff := recordsEnd
		if string(mxBlob[idxOff:idxOff+4]) == mxIdxMagic {
			bc := int(binary.LittleEndian.Uint16(mxBlob[idxOff+4:]))
			if bc == mxBucketCount {
				off := idxOff + mxIdxHeaderLen
				for i := 0; i < mxBucketCount; i++ {
					mxBuckets[i][0] = binary.LittleEndian.Uint32(mxBlob[off:])
					mxBuckets[i][1] = binary.LittleEndian.Uint32(mxBlob[off+4:])
					off += mxBucketSize
				}
				mxHasIndex = true
			}
		}
	}

	mxCarriers = []Carrier{{}}
	r := csv.NewReader(bytes.NewReader(mxCarrierCSV))
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
		for len(mxCarriers) <= idx {
			mxCarriers = append(mxCarriers, Carrier{})
		}
		brand := ""
		if len(rec) > 3 {
			brand = rec[3]
		}
		mxCarriers[idx] = Carrier{OCN: rec[1], Name: rec[2], Brand: brand}
	}
	mxCarrierOK = len(mxCarriers) > 1
}

func mxRecord(i int) (start, end uint64, class Class, carrier uint16) {
	o := mxHeaderLen + i*mxRecordSize
	start = binary.LittleEndian.Uint64(mxBlob[o:])
	end = binary.LittleEndian.Uint64(mxBlob[o+8:])
	class = Class(mxBlob[o+16])
	carrier = binary.LittleEndian.Uint16(mxBlob[o+17:])
	return
}

// MXAvailable reports whether a Mexican range table was built into this binary.
func MXAvailable() bool {
	mxOnce.Do(mxInit)
	return mxUsable
}

func mxNumber(e164 string) (uint64, bool) {
	if len(e164) != 13 || e164[0] != '+' || e164[1] != '5' || e164[2] != '2' {
		return 0, false
	}
	var n uint64
	for i := 3; i < 13; i++ {
		c := e164[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	if n < 2_000_000_000 {
		return 0, false
	}
	return n, true
}

func lookupMX(n uint64) (Class, Carrier) {
	mxOnce.Do(mxInit)
	if !mxUsable {
		return Unknown, Carrier{}
	}

	lo, hi := 0, mxCount
	if mxHasIndex {
		// Use 3-digit prefix to narrow the search range.
		prefix := int(n / 10_000_000) // e.g. 5512340000 -> 551
		bi := prefix - mxBucketBase
		if bi < 0 || bi >= mxBucketCount {
			return Unknown, Carrier{}
		}
		lo = int(mxBuckets[bi][0])
		hi = int(mxBuckets[bi][1])
		if lo >= hi {
			return Unknown, Carrier{}
		}
	}

	i := lo + sort.Search(hi-lo, func(j int) bool {
		start, _, _, _ := mxRecord(lo + j)
		return start > n
	})
	if i == lo {
		return Unknown, Carrier{}
	}
	start, end, class, carrier := mxRecord(i - 1)
	if n < start || n > end {
		return Unknown, Carrier{}
	}
	if !mxCarrierOK || int(carrier) >= len(mxCarriers) {
		return class, Carrier{}
	}
	return class, mxCarriers[carrier]
}
