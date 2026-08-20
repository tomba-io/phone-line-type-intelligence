package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	linetypev1 "github.com/tomba-io/phone-line-type-intelligence/proto/linetype/v1"
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

const (
	mxBucketCount = 800 // 3-digit prefixes 200..999
	mxBucketBase  = 200
)

// mxModalidad maps IFT's service modality to a line class.
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

// mxBrand is populated from the -mx-brands file, if one is supplied.
var mxBrand = map[string]string{}

// loadMXBrands reads an optional csv of `legal_name,brand`, letting the
// operator's trading name be shown instead of the registered company.
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

	for i := 1; i < len(out); i++ {
		if out[i].start <= out[i-1].end {
			return nil, nil, fmt.Errorf("%s: ranges %d-%d and %d-%d overlap; "+
				"the lookup would be ambiguous", path,
				out[i-1].start, out[i-1].end, out[i].start, out[i].end)
		}
	}
	return out, skipped, nil
}

// mxToProto builds an MXTable protobuf from the in-memory MX data.
func mxToProto(ranges []mxRange, ops *mxOperators) *linetypev1.MXTable {
	pbRanges := make([]*linetypev1.MXRange, len(ranges))
	for i, r := range ranges {
		pbRanges[i] = &linetypev1.MXRange{
			Start:        r.start,
			End:          r.end,
			LineClass:    linetypev1.LineClass(r.class),
			CarrierIndex: uint32(r.carrier),
		}
	}

	// Build prefix index.
	pbBuckets := make([]*linetypev1.MXBucket, mxBucketCount)
	ri := 0
	for b := 0; b < mxBucketCount; b++ {
		prefix := uint64(b+mxBucketBase) * 10_000_000
		nextPrefix := prefix + 10_000_000
		for ri < len(ranges) && ranges[ri].end < prefix {
			ri++
		}
		startIdx := uint32(ri)
		ei := ri
		for ei < len(ranges) && ranges[ei].start < nextPrefix {
			ei++
		}
		pbBuckets[b] = &linetypev1.MXBucket{
			StartIndex: startIdx,
			EndIndex:   uint32(ei),
		}
	}

	// Build carrier directory.
	pbCarriers := make([]*linetypev1.CarrierInfo, len(ops.names))
	for i := range ops.names {
		pbCarriers[i] = &linetypev1.CarrierInfo{
			Name:  ops.names[i],
			Brand: mxBrand[ops.names[i]],
		}
	}
	if len(pbCarriers) > 0 {
		pbCarriers[0] = &linetypev1.CarrierInfo{}
	}

	return &linetypev1.MXTable{
		Ranges:      pbRanges,
		PrefixIndices: pbBuckets,
		Carriers:    pbCarriers,
	}
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
