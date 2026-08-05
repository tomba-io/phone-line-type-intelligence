package main

import (
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Mexico uses a RANGE table, not a block table.
//
// NANPA allocates in whole NXXs and thousands-blocks, so a fixed-stride array
// indexed by NPANXXB is exact. IFT does not: of 177,996 published ranges,
// 16,988 start off a 1000 boundary and 28,781 are not a whole number of
// thousands. Rounding them to 1000-blocks would silently merge 68 blocks that
// hold both fixed and mobile numbering.
//
// Sorted ranges with a binary search are both exact and smaller than the block
// table would have been: ~3.4 MB against 4 MB, with no rounding at all.
//
// File layout, little-endian throughout:
//
//	magic   "MXPN"   4 bytes
//	version uint16   2
//	_       uint16   2   reserved, must be zero
//	count   uint32   4
//	then count records of 20 bytes:
//	  start   uint64  8   first number in the range, e.g. 5512340000
//	  end     uint64  8   last number, inclusive
//	  class   uint8   1   mirrors linetype.Class
//	  carrier uint16  2   index into the MX carrier table, 0 = none
//	  _       uint8   1   reserved
const (
	mxMagic      = "MXPN"
	mxVersion    = 1
	mxHeaderLen  = 12
	mxRecordSize = 20
)

// mxModalidad maps IFT's service modality to a line class.
//
//	CPP  "Calling Party Pays"  - mobile; the caller pays, the standard mobile
//	                             modality since Mexico retired the 044/045
//	                             dialling prefixes in 2019.
//	MPP  "Mobile Party Pays"   - also mobile; the subscriber pays for incoming.
//	FIJO                       - fixed line.
//
// Anything else is left unclassified rather than guessed, exactly as an
// unmapped OCN is on the NANPA side.
var mxModalidad = map[string]byte{
	"CPP":  cWireless,
	"MPP":  cWireless,
	"FIJO": cWireline,
}

type mxRange struct {
	start, end uint64
	class      byte
	carrier    uint16
}

// mxOperators interns operator names. IFT publishes the company directly, with
// no operating company number, so the name itself is the key.
type mxOperators struct {
	index map[string]uint16
	names []string
}

func newMXOperators() *mxOperators {
	return &mxOperators{index: map[string]uint16{}, names: []string{""}}
}

func (o *mxOperators) intern(name string) (uint16, error) {
	name = collapseSpace(name)
	if name == "" {
		return 0, nil
	}
	if i, ok := o.index[name]; ok {
		return i, nil
	}
	if len(o.names) > 0xFFFF {
		return 0, fmt.Errorf("more than %d Mexican operators; the index no longer fits in 16 bits", 0xFFFF)
	}
	i := uint16(len(o.names))
	o.index[name] = i
	o.names = append(o.names, name)
	return i, nil
}

func (o *mxOperators) writeTable(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("mx carrier table: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"index", "ocn", "name", "brand"}); err != nil {
		return fmt.Errorf("mx carrier table: %w", err)
	}
	for i := 1; i < len(o.names); i++ {
		// IFT publishes no OCN, so that column stays empty rather than being
		// filled with something invented.
		if err := w.Write([]string{fmt.Sprint(i), "", o.names[i], mxBrand[o.names[i]]}); err != nil {
			return fmt.Errorf("mx carrier table: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

// mxBrand is populated from the -mx-brands file, if one is supplied.
var mxBrand = map[string]string{}

// loadMXBrands reads an optional csv of `legal_name,brand`, letting the
// operator's trading name be shown instead of the registered company. Same
// contract as the brand column in ocn.csv: display only, never inferred.
func loadMXBrands(path string) error {
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("mx brands: %w", err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.Comment = '#'
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("mx brands: %w", err)
		}
		if len(rec) < 2 {
			continue
		}
		name := collapseSpace(rec[0])
		if strings.EqualFold(name, "legal_name") || name == "" {
			continue
		}
		mxBrand[name] = strings.TrimSpace(rec[1])
	}
	return nil
}

// buildMX parses the IFT Plan Nacional de Numeración into sorted ranges.
func buildMX(path string, ops *mxOperators) ([]mxRange, map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	for i := range header {
		header[i] = strings.TrimSpace(header[i])
	}
	iStart := findCol(header, []string{"numeracion_inicial"})
	iEnd := findCol(header, []string{"numeracion_final"})
	iMod := findCol(header, []string{"modalidad"})
	iOp := findCol(header, []string{"razon_social"})
	if iStart < 0 || iEnd < 0 || iMod < 0 {
		return nil, nil, fmt.Errorf("%s: need NUMERACION_INICIAL, NUMERACION_FINAL and MODALIDAD; "+
			"run -inspect %s. IFT changed this layout on 1 July 2025", path, path)
	}

	var out []mxRange
	skipped := map[string]int{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		if len(rec) <= iMod {
			continue
		}
		start, err1 := strconv.ParseUint(strings.TrimSpace(rec[iStart]), 10, 64)
		end, err2 := strconv.ParseUint(strings.TrimSpace(rec[iEnd]), 10, 64)
		if err1 != nil || err2 != nil || start > end {
			continue
		}
		mod := strings.ToUpper(strings.TrimSpace(rec[iMod]))
		cls, ok := mxModalidad[mod]
		if !ok {
			skipped[mod]++
			continue
		}
		var op uint16
		if iOp >= 0 && len(rec) > iOp {
			if op, err = ops.intern(rec[iOp]); err != nil {
				return nil, nil, err
			}
		}
		out = append(out, mxRange{start: start, end: end, class: cls, carrier: op})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].start < out[j].start })

	// Overlaps would make a binary search ambiguous. The published data has
	// none; if that ever changes, say so rather than silently picking one.
	for i := 1; i < len(out); i++ {
		if out[i].start <= out[i-1].end {
			return nil, nil, fmt.Errorf("%s: ranges %d-%d and %d-%d overlap; "+
				"the lookup would be ambiguous", path,
				out[i-1].start, out[i-1].end, out[i].start, out[i].end)
		}
	}
	return out, skipped, nil
}

const (
	mxIdxMagic    = "MXIX"
	mxIdxHdrLen   = 8   // magic(4) + bucketCount(2) + reserved(2)
	mxBucketSize  = 8   // startIdx(4) + endIdx(4)
	mxBucketCount = 800 // 3-digit prefixes 200..999
	mxBucketBase  = 200
)

func writeMXBlob(ranges []mxRange, path string) error {
	recordsLen := len(ranges) * mxRecordSize
	idxLen := mxIdxHdrLen + mxBucketCount*mxBucketSize
	buf := make([]byte, mxHeaderLen+recordsLen+idxLen)

	// Header.
	copy(buf, mxMagic)
	binary.LittleEndian.PutUint16(buf[4:], mxVersion)
	binary.LittleEndian.PutUint32(buf[8:], uint32(len(ranges)))

	// Records.
	for i, r := range ranges {
		o := mxHeaderLen + i*mxRecordSize
		binary.LittleEndian.PutUint64(buf[o:], r.start)
		binary.LittleEndian.PutUint64(buf[o+8:], r.end)
		buf[o+16] = r.class
		binary.LittleEndian.PutUint16(buf[o+17:], r.carrier)
	}

	// Prefix index: bucket per 3-digit prefix (200..999).
	idxOff := mxHeaderLen + recordsLen
	copy(buf[idxOff:], mxIdxMagic)
	binary.LittleEndian.PutUint16(buf[idxOff+4:], mxBucketCount)
	// reserved 2 bytes already zero

	// Compute bucket boundaries. Ranges are sorted by start.
	var buckets [mxBucketCount][2]uint32 // [startIdx, endIdx)
	ri := 0
	for b := 0; b < mxBucketCount; b++ {
		prefix := uint64(b+mxBucketBase) * 10_000_000
		nextPrefix := prefix + 10_000_000
		// Advance to first range that could overlap this prefix.
		for ri < len(ranges) && ranges[ri].end < prefix {
			ri++
		}
		buckets[b][0] = uint32(ri)
		// Find end: first range whose start >= nextPrefix.
		ei := ri
		for ei < len(ranges) && ranges[ei].start < nextPrefix {
			ei++
		}
		buckets[b][1] = uint32(ei)
	}

	off := idxOff + mxIdxHdrLen
	for _, bucket := range buckets {
		binary.LittleEndian.PutUint32(buf[off:], bucket[0])
		binary.LittleEndian.PutUint32(buf[off+4:], bucket[1])
		off += mxBucketSize
	}

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return fmt.Errorf("mx blob: %w", err)
	}
	return nil
}

func reportMX(ranges []mxRange, ops *mxOperators, skipped map[string]int) {
	var numbers [5]uint64
	for _, r := range ranges {
		if int(r.class) < len(numbers) {
			numbers[r.class] += r.end - r.start + 1
		}
	}
	names := map[byte]string{cWireline: "wireline", cWireless: "wireless"}
	fmt.Fprintf(os.Stderr, "\n=== Mexico (+52) ===\n")
	fmt.Fprintf(os.Stderr, "  %d ranges, %d operators\n", len(ranges), len(ops.names)-1)
	total := numbers[cWireline] + numbers[cWireless]
	for _, c := range []byte{cWireless, cWireline} {
		pct := 0.0
		if total > 0 {
			pct = 100 * float64(numbers[c]) / float64(total)
		}
		fmt.Fprintf(os.Stderr, "  %10s %14d numbers  %5.1f%%\n", names[c], numbers[c], pct)
	}
	for mod, n := range skipped {
		fmt.Fprintf(os.Stderr, "  skipped MODALIDAD %q on %d ranges (not a known service type)\n", mod, n)
	}
}
