package main

import (
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strings"

	linetypev1 "github.com/tomba-io/phone-line-type-intelligence/proto/linetype/v1"
)

// carrierRegistry interns OCNs so the per-block table can hold a 16-bit index
// instead of a 4-character code. Index 0 is reserved for "no carrier on file",
// which is what an unbuilt or sparse table reads as.
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

// toProto builds a CarrierTable protobuf from the in-memory carrier data.
func (r *carrierRegistry) toProto(carrier []uint16) *linetypev1.CarrierTable {
	const nxxCount = 800_000

	// Compute per-NXX majority carrier and collect exceptions.
	nxxBase := make([]byte, nxxCount*2)
	var exceptions []*linetypev1.CarrierException

	for nxx := uint32(0); nxx < nxxCount; nxx++ {
		baseOff := nxx * 10

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
		binary.LittleEndian.PutUint16(nxxBase[nxx*2:], best)

		for b := uint32(0); b < 10; b++ {
			if carrier[baseOff+b] != best {
				exceptions = append(exceptions, &linetypev1.CarrierException{
					NxxOffset:    nxx,
					Block:        b,
					CarrierIndex: uint32(carrier[baseOff+b]),
				})
			}
		}
	}

	// Sort exceptions by (nxx_offset*10 + block).
	sort.Slice(exceptions, func(i, j int) bool {
		ki := exceptions[i].NxxOffset*10 + exceptions[i].Block
		kj := exceptions[j].NxxOffset*10 + exceptions[j].Block
		return ki < kj
	})

	// Build carrier directory.
	carriers := make([]*linetypev1.CarrierInfo, len(r.ocns))
	for i := range r.ocns {
		carriers[i] = &linetypev1.CarrierInfo{
			Ocn:   r.ocns[i],
			Name:  collapseSpace(r.names[i]),
			Brand: ocnBrand[r.ocns[i]],
		}
	}

	return &linetypev1.CarrierTable{
		NxxBase:    nxxBase,
		Exceptions: exceptions,
		Carriers:   carriers,
	}
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
