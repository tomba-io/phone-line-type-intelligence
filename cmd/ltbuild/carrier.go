package main

import (
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"
)

// carrierRegistry interns OCNs so the per-block table can hold a 16-bit index
// instead of a 4-character code. Index 0 is reserved for "no carrier on file",
// which is what an unbuilt or sparse table reads as.
//
// The width is the whole reason this is a separate table from the class blob:
// a class fits in 4 bits, an OCN index needs 16, so carrying carrier data
// costs 2 bytes per block (16 MB) against the class table's 0.5 (4 MB). Keep
// them separate so callers who only want a line type do not pay for it.
type carrierRegistry struct {
	index   map[string]uint16
	ocns    []string
	names   []string
	samples []string // one NPANXX per OCN, so the worklist can be looked up
}

func newCarrierRegistry() *carrierRegistry {
	return &carrierRegistry{
		index: map[string]uint16{},
		// Slot 0 is the sentinel: no carrier on file.
		ocns:    []string{""},
		names:   []string{""},
		samples: []string{""},
	}
}

// intern returns the index for an OCN, allocating one on first sight.
// The first non-empty company name seen for an OCN wins; later rows for the
// same OCN carry per-state variants of the same name and would only churn.
// note records a representative NPANXX for an OCN. Used only by the worklist.
func (r *carrierRegistry) note(idx uint16, npanxx string) {
	if idx != 0 && int(idx) < len(r.samples) && r.samples[idx] == "" {
		r.samples[idx] = npanxx
	}
}

func (r *carrierRegistry) intern(ocn, name string) (uint16, error) {
	if ocn == "" {
		return 0, nil
	}
	if i, ok := r.index[ocn]; ok {
		if r.names[i] == "" && name != "" {
			r.names[i] = name
		}
		return i, nil
	}
	if len(r.ocns) > 0xFFFF {
		return 0, fmt.Errorf("more than %d distinct OCNs; the carrier index no longer fits in 16 bits", 0xFFFF)
	}
	i := uint16(len(r.ocns))
	r.index[ocn] = i
	r.ocns = append(r.ocns, ocn)
	r.names = append(r.names, name)
	r.samples = append(r.samples, "")
	return i, nil
}

// writeTable emits the index -> OCN,name mapping that linetype embeds
// alongside the packed carrier blob.
func (r *carrierRegistry) writeTable(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("carrier table: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"index", "ocn", "name", "brand"}); err != nil {
		return fmt.Errorf("carrier table: %w", err)
	}
	for i := 1; i < len(r.ocns); i++ {
		row := []string{fmt.Sprint(i), r.ocns[i], collapseSpace(r.names[i]), ocnBrand[r.ocns[i]]}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("carrier table: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}

// writeBlob packs carrier data in compact format: NXX base table + sparse
// block-level exceptions. Most NXXs have all 10 blocks assigned to the same
// carrier; only pooled rate centres differ.
//
// Format (little-endian):
//
//	magic   "CXNB"    4 bytes
//	version uint16    2
//	exCount uint32    4
//	NXX base table:   800,000 * 2 bytes (uint16 per NXX)
//	Exceptions:       exCount * 6 bytes each:
//	  nxxOff  uint24  3 bytes (NXX offset from 200000)
//	  block   uint8   1 byte  (0-9)
//	  carrier uint16  2 bytes
//
// Exceptions are sorted by (nxxOff, block) for binary search.
func (r *carrierRegistry) writeBlob(carrier []uint16, path string) error {
	if len(carrier) != keyCnt {
		return fmt.Errorf("carrier table has %d slots, want %d", len(carrier), keyCnt)
	}

	const (
		nxxCount = 800_000
		hdrLen   = 10
		nxxTbl   = nxxCount * 2
		exSize   = 6
	)

	// Compute per-NXX majority carrier and collect exceptions.
	nxxBase := make([]uint16, nxxCount)
	type exception struct {
		nxxOff uint32
		block  byte
		idx    uint16
	}
	var exceptions []exception

	for nxx := uint32(0); nxx < nxxCount; nxx++ {
		baseOff := nxx * 10

		// Find the most common carrier in this NXX's 10 blocks.
		freq := map[uint16]int{}
		for b := uint32(0); b < 10; b++ {
			freq[carrier[baseOff+b]]++
		}
		best := carrier[baseOff]
		bestCount := 0
		for v, c := range freq {
			if c > bestCount || (c == bestCount && v < best) {
				bestCount = c
				best = v
			}
		}
		nxxBase[nxx] = best

		// Record exceptions: blocks that differ from the majority.
		for b := uint32(0); b < 10; b++ {
			if carrier[baseOff+b] != best {
				exceptions = append(exceptions, exception{nxxOff: nxx, block: byte(b), idx: carrier[baseOff+b]})
			}
		}
	}

	// Sort exceptions by (nxxOff, block).
	sort.Slice(exceptions, func(i, j int) bool {
		ki := exceptions[i].nxxOff*10 + uint32(exceptions[i].block)
		kj := exceptions[j].nxxOff*10 + uint32(exceptions[j].block)
		return ki < kj
	})

	totalLen := hdrLen + nxxTbl + len(exceptions)*exSize
	buf := make([]byte, totalLen)

	// Header.
	copy(buf, "CXNB")
	binary.LittleEndian.PutUint16(buf[4:], 1) // version
	binary.LittleEndian.PutUint32(buf[6:], uint32(len(exceptions)))

	// NXX base table.
	off := hdrLen
	for _, v := range nxxBase {
		binary.LittleEndian.PutUint16(buf[off:], v)
		off += 2
	}

	// Exceptions.
	for _, ex := range exceptions {
		buf[off] = byte(ex.nxxOff)
		buf[off+1] = byte(ex.nxxOff >> 8)
		buf[off+2] = byte(ex.nxxOff >> 16)
		buf[off+3] = ex.block
		binary.LittleEndian.PutUint16(buf[off+4:], ex.idx)
		off += exSize
	}

	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return fmt.Errorf("carrier blob: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  compact carrier: %d KB NXX base + %d exceptions (%d KB) = %d KB total\n",
		nxxTbl/1024, len(exceptions), len(exceptions)*exSize/1024, totalLen/1024)
	return nil
}

// collapseSpace squeezes the fixed-width padding out of the published company
// names. The CO code file pads Company to a fixed column width, so the raw
// value arrives with a long run of trailing spaces inside its quotes.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// topCarriers returns the OCNs holding the most blocks, most first. Used by
// -report so the unmapped worklist can be ranked by what it actually costs.
func topCarriers(carrier []uint16, r *carrierRegistry, classOf map[string]byte, n int) []carrierCost {
	blocks := make([]int, len(r.ocns))
	for _, idx := range carrier {
		if idx != 0 {
			blocks[idx]++
		}
	}
	var out []carrierCost
	for i := 1; i < len(r.ocns); i++ {
		if blocks[i] == 0 {
			continue
		}
		if _, mapped := classOf[r.ocns[i]]; mapped {
			continue
		}
		out = append(out, carrierCost{OCN: r.ocns[i], Name: collapseSpace(r.names[i]), Blocks: blocks[i]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Blocks != out[j].Blocks {
			return out[i].Blocks > out[j].Blocks
		}
		return out[i].OCN < out[j].OCN
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

type carrierCost struct {
	OCN    string
	Name   string
	Blocks int
}

// writeWorklist emits the unmapped OCNs ranked by blocks, with a representative
// NPANXX for each so the classification can be looked up without re-parsing the
// source files. This is the file the maintainer works top-down.
func (r *carrierRegistry) writeWorklist(carrier []uint16, classOf map[string]byte, path string) error {
	costs := topCarriers(carrier, r, classOf, 0)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("worklist: %w", err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"ocn", "class", "blocks", "cumulative_pct", "example_npanxx", "company"}); err != nil {
		return fmt.Errorf("worklist: %w", err)
	}
	total := 0
	for _, c := range costs {
		total += c.Blocks
	}
	run := 0
	for _, c := range costs {
		run += c.Blocks
		row := []string{c.OCN, "", fmt.Sprint(c.Blocks),
			fmt.Sprintf("%.2f", 100*float64(run)/float64(max(total, 1))),
			r.samples[r.index[c.OCN]], c.Name}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("worklist: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}
