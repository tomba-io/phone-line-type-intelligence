// Command ltbuild constructs the packed line-type table from NANPA central
// office code assignment records, thousands-block pooling records, and an OCN
// classification map.
//
//	ltbuild -inspect cocus.txt
//	    Print the detected header row and exit. Run this FIRST — the column
//	    layout of the published files changes, and this tool refuses to guess.
//
//	ltbuild -co cocus.txt -blocks blocks.txt -ocn ocn.csv -proto-out data/phone_data.pb
//
// Precedence: thousands-block assignments override NXX-level assignments,
// because in a pooled rate centre a single NXX is split across up to ten
// carriers. Applying the NXX-level holder to all ten blocks is the single
// largest avoidable source of error in this kind of table.
package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	linetypev1 "github.com/tomba-io/phone-line-type-intelligence/proto/linetype/v1"
	"google.golang.org/protobuf/proto"
)

const (
	keyBase = 2_000_000
	keyCnt  = 8_000_000
	blobLen = keyCnt / 2
)

// classes mirror linetype.Class.
const (
	cUnknown  = 0
	cWireline = 1
	cWireless = 2
	cVoIP     = 3
	cTollFree = 4
)

// classFromName maps the class spellings accepted in ocn.csv.
//
// `clec` deliberately resolves to VoIP, not Wireline. A CLEC-held range is
// frequently SMS-reachable — Twilio and Bandwidth DIDs sit on wireline
// assignments — and filing them as wireline discards reachable numbers. This
// matches rule 3 in data/ocn.csv; the two must not drift apart.
var classFromName = map[string]byte{
	"wireline": cWireline, "landline": cWireline, "ilec": cWireline,
	"wireless": cWireless, "mobile": cWireless, "cmrs": cWireless, "pcs": cWireless,
	"voip": cVoIP, "ivoip": cVoIP, "clec": cVoIP,
	"tollfree": cTollFree, "toll_free": cTollFree,
}

// Column aliases. The published files have used several spellings over the
// years; add to these rather than hardcoding positions.
var (
	npaAliases    = []string{"npa", "area code", "areacode"}
	npanxxAliases = []string{"npa-nxx", "npanxx", "npa_nxx"}
	statusAliases = []string{"use", "status", "block status", "blockstatus", "assignment status"}
	nxxAliases    = []string{"nxx", "co code", "cocode", "central office code", "co code (nxx)"}
	blockAliases  = []string{"block", "block id", "thousands block", "block_id", "x"}
	// "block holder ocn" is the Canadian block file's spelling. That file also
	// has a "code holder ocn" column holding the NXX-level assignee; it is
	// deliberately NOT an alias, because matching it would silently undo the
	// block-level override this tool exists to apply.
	ocnAliases     = []string{"ocn", "operating company number", "company code", "block holder ocn"}
	companyAliases = []string{"company", "assigned to", "block holder", "company name", "code holder"}
)

// liveStatus is the set of status values meaning "a carrier holds this range".
// Anything absent from this set is skipped: classifying a vacant, reserved or
// available range invents an operator for numbers nobody can be reached on.
//
//	US files use two-letter codes; only AS is live. VC vacant, RV reserved,
//	PR protected, UA unavailable, AV/AP/AF available and RT retained are not.
//	Canadian CO codes spell it out: "In Service" and "Assigned" are live,
//	while Available, Protected, Stranded Code, Being Recovered, Recovered/
//	Aging, Moratorium on Assignment, Plant Test, For Special Use, New NPA,
//	Temporarily Unavailable and Not Available are not.
//	Canadian block codes use AS plus IS (in service).
var liveStatus = map[string]bool{
	"AS":         true,
	"IS":         true,
	"IN SERVICE": true,
	"ASSIGNED":   true,
}

// pseudoOCN lists codes that appear in the OCN column but are not operators.
// MULT is the published placeholder for "multiple OCN listing" - a block whose
// ownership the source itself declines to state. Treating it as a carrier
// invents a company called MULTIPLE OCN LISTING and puts it on the worklist
// forever, where no lookup can ever resolve it. It must read as unknown.
var pseudoOCN = map[string]bool{
	"MULT": true,
}

// ocnBrand holds the optional brand column from ocn.csv, keyed by OCN.
var ocnBrand = map[string]string{}

func isLive(status string) bool {
	return liveStatus[strings.ToUpper(strings.TrimSpace(status))]
}

func main() {
	inspect := flag.String("inspect", "", "print header row of a file and exit")
	coPath := flag.String("co", "", "central office code assignment file(s), comma-separated (US NANPA and/or Canadian CNAC)")
	blockPath := flag.String("blocks", "", "thousands-block assignment file(s), comma-separated (optional but strongly recommended)")
	ocnPath := flag.String("ocn", "ocn.csv", "OCN classification map (csv: ocn,class)")
	mxPath := flag.String("mx", "", "IFT Plan Nacional de Numeracion CSV (Mexico, +52)")
	mxBrands := flag.String("mx-brands", "", "optional csv of legal_name,brand for Mexican operators")
	protoOut := flag.String("proto-out", "data/phone_data.pb", "write combined protobuf data file here")
	worklistPath := flag.String("worklist-out", "", "write the ranked unmapped-OCN worklist here; off by default")
	report := flag.Bool("report", true, "print coverage and unmapped-OCN report to stderr")
	flag.Parse()

	if *inspect != "" {
		if err := doInspect(*inspect); err != nil {
			die(err)
		}
		return
	}
	var ranges []mxRange
	var ops *mxOperators
	if *mxPath != "" {
		if err := loadMXBrands(*mxBrands); err != nil {
			die(err)
		}
		ops = newMXOperators()
		var skipped map[string]int
		var err error
		ranges, skipped, err = buildMX(*mxPath, ops)
		if err != nil {
			die(err)
		}
		if *report {
			reportMX(ranges, ops, skipped)
		}
		if *coPath == "" {
			return
		}
	}
	if *coPath == "" {
		die(fmt.Errorf("-co is required (or use -inspect or -mx)"))
	}

	ocn, err := loadOCN(*ocnPath)
	if err != nil {
		die(err)
	}
	fmt.Fprintf(os.Stderr, "loaded %d OCN classifications\n", len(ocn))

	table := make([]byte, keyCnt)     // one byte per key while building; packed at the end
	carrier := make([]uint16, keyCnt) // OCN index per key, 0 = none on file
	unmapped := map[string]int{}
	reg := newCarrierRegistry()
	region := make([]uint8, regionCnt)
	rreg := newRegionRegistry()

	// Pass 1: NXX-level assignments fill all ten blocks. Every -co file is
	// applied before any -blocks file, so block-level data always wins
	// regardless of the order the files were listed in.
	for _, path := range splitPaths(*coPath) {
		rows, err := applyFile(path, ocn, table, unmapped, false, carrier, reg, region, rreg)
		if err != nil {
			die(err)
		}
		fmt.Fprintf(os.Stderr, "applied %d NXX-level rows from %s\n", rows, path)
	}

	// Pass 2: block-level assignments override.
	blockPaths := splitPaths(*blockPath)
	for _, path := range blockPaths {
		rows, err := applyFile(path, ocn, table, unmapped, true, carrier, reg, region, rreg)
		if err != nil {
			die(err)
		}
		fmt.Fprintf(os.Stderr, "applied %d block-level rows (override) from %s\n", rows, path)
	}
	if len(blockPaths) == 0 {
		fmt.Fprintln(os.Stderr, "WARNING: no -blocks file. Pooled rate centres will be "+
			"misclassified wherever an NXX is split between carriers. This is the "+
			"dominant error source in dense metros.")
	}

	if err := writeProto(table, carrier, reg, region, rreg, ranges, ops, *protoOut); err != nil {
		die(err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", *protoOut)

	if *worklistPath != "" {
		if err := reg.writeWorklist(carrier, ocn, *worklistPath); err != nil {
			die(err)
		}
		fmt.Fprintf(os.Stderr, "wrote %s\n", *worklistPath)
	}
	if *report {
		printReport(table, unmapped)
		printCarrierWorklist(carrier, reg, ocn)
	}
}

// splitPaths turns a comma-separated flag value into a list, dropping blanks.
// Several sources feed one table: NANPA covers the US and its territories,
// CNAC covers Canada, and both live in the same +1 key space.
func splitPaths(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "ltbuild:", err)
	os.Exit(1)
}

// splitter detects tab vs comma from the header line.
func splitter(header string) func(string) []string {
	if strings.Count(header, "\t") >= strings.Count(header, ",") {
		return func(s string) []string { return strings.Split(s, "\t") }
	}
	// Company names are quoted when they contain special characters, so a
	// naive comma split corrupts every field after them.
	return func(s string) []string {
		r := csv.NewReader(strings.NewReader(s))
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		rec, err := r.Read()
		if err != nil {
			return strings.Split(s, ",")
		}
		return rec
	}
}

func doInspect(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for n := 0; n < 3 && sc.Scan(); n++ {
		split := splitter(sc.Text())
		fmt.Printf("--- line %d ---\n", n+1)
		for i, c := range split(sc.Text()) {
			fmt.Printf("  [%2d] %q\n", i, strings.TrimSpace(c))
		}
	}
	return sc.Err()
}

func findCol(header []string, aliases []string) int {
	for i, h := range header {
		clean := strings.ToLower(strings.TrimSpace(h))
		for _, a := range aliases {
			if clean == a {
				return i
			}
		}
	}
	return -1
}

// applyFile parses one assignment file and writes classes into table.
// If blockLevel is true, a block column is required and only that block is set.
func applyFile(path string, ocn map[string]byte, table []byte, unmapped map[string]int, blockLevel bool,
	carrier []uint16, reg *carrierRegistry, region []uint8, rreg *regionRegistry) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	if !sc.Scan() {
		return 0, fmt.Errorf("%s: empty file", path)
	}
	split := splitter(sc.Text())
	header := split(sc.Text())

	iNPA := findCol(header, npaAliases)
	iNXX := findCol(header, nxxAliases)
	iNN := findCol(header, npanxxAliases) // combined NPA-NXX, used by the CO code file
	iOCN := findCol(header, ocnAliases)
	iBlk := findCol(header, blockAliases)
	iStatus := findCol(header, statusAliases)
	iCompany := findCol(header, companyAliases)
	iState := findCol(header, stateAliases)

	if iOCN < 0 || (iNN < 0 && (iNPA < 0 || iNXX < 0)) {
		return 0, fmt.Errorf("%s: could not locate NPA-NXX (or NPA+NXX) and OCN columns; "+
			"run -inspect %s and extend the alias lists in main.go", path, path)
	}
	if iStatus < 0 {
		fmt.Fprintf(os.Stderr, "  WARNING: %s has no status column. Vacant, reserved and "+
			"unavailable codes cannot be filtered out and will be treated as live.\n", path)
	}
	if blockLevel && iBlk < 0 {
		return 0, fmt.Errorf("%s: block file has no block column; run -inspect", path)
	}

	n, skippedStatus := 0, 0
	for sc.Scan() {
		rec := split(sc.Text())
		if len(rec) <= iOCN {
			continue
		}
		// Only assigned resources are live. AS covers both files; everything
		// else (VC vacant, RV reserved, PR protected, UA unavailable, AV/AP/AF
		// available, RT retained) must not be classified.
		if iStatus >= 0 {
			if len(rec) <= iStatus {
				continue
			}
			if !isLive(rec[iStatus]) {
				skippedStatus++
				continue
			}
		}

		var npa, nxx string
		if iNN >= 0 {
			if len(rec) <= iNN {
				continue
			}
			// Combined field, published as NPA-NXX; tolerate an absent dash.
			nn := strings.ReplaceAll(strings.TrimSpace(rec[iNN]), "-", "")
			if len(nn) != 6 {
				continue
			}
			npa, nxx = nn[:3], nn[3:]
		} else {
			if len(rec) <= iNPA || len(rec) <= iNXX {
				continue
			}
			npa = strings.TrimSpace(rec[iNPA])
			nxx = strings.TrimSpace(rec[iNXX])
		}
		if len(npa) != 3 || len(nxx) != 3 {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(rec[iOCN]))
		if pseudoOCN[code] {
			code = ""
		}

		// The class depends on ocn.csv, but the carrier does not: the holding
		// company is right there in the file. Record it either way, so an
		// unmapped OCN still reports WHO holds the range even though it cannot
		// yet say what kind of line it is.
		cls, ok := ocn[code]
		if !ok || cls == cUnknown {
			if code != "" {
				unmapped[code]++
			}
			cls = cUnknown
		}

		var company string
		if iCompany >= 0 && len(rec) > iCompany {
			company = collapseSpace(strings.Trim(strings.TrimSpace(rec[iCompany]), `"`))
		}
		cIdx, err := reg.intern(code, company)
		if err != nil {
			return n, err
		}
		reg.note(cIdx, npa+nxx)

		base, err := strconv.Atoi(npa + nxx)
		if err != nil || base < 200_000 {
			continue
		}

		// Region is per NXX. The first file to supply one wins, so the CO code
		// files (which cover every NXX) set it and the block files only fill
		// gaps rather than churning it.
		if iState >= 0 && len(rec) > iState {
			if ri, err := rreg.intern(rec[iState]); err != nil {
				return n, err
			} else if ri != 0 && base-regionBase >= 0 && base-regionBase < regionCnt {
				if region[base-regionBase] == 0 {
					region[base-regionBase] = ri
				}
			}
		}

		if blockLevel {
			b := strings.TrimSpace(rec[iBlk])
			if len(b) != 1 || b[0] < '0' || b[0] > '9' {
				continue
			}
			idx := base*10 + int(b[0]-'0') - keyBase
			if idx >= 0 && idx < keyCnt {
				// Unconditional write, including cUnknown. A block assigned to
				// a carrier we cannot classify must NOT keep the NXX holder's
				// class: the block demonstrably belongs to somebody else, and
				// inheriting produces a confident wrong answer where unknown
				// is the honest one.
				table[idx] = cls
				carrier[idx] = cIdx
				n++
			}
		} else {
			for d := 0; d < 10; d++ {
				idx := base*10 + d - keyBase
				if idx >= 0 && idx < keyCnt {
					table[idx] = cls
					carrier[idx] = cIdx
				}
			}
			n++
		}
	}
	if skippedStatus > 0 {
		fmt.Fprintf(os.Stderr, "  skipped %d non-assigned (vacant/reserved/available/retained) rows\n",
			skippedStatus)
	}
	return n, sc.Err()
}

func loadOCN(path string) (map[string]byte, error) {
	ocnBrand = map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	// This file is hand-maintained and full of prose comments, which routinely
	// contain apostrophes and quoted company names. Treat '#' lines as comments
	// and never let a stray quote inside one abort the whole build.
	r.Comment = '#'
	r.LazyQuotes = true
	out := map[string]byte{}
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 2 {
			continue
		}
		code := strings.ToUpper(strings.TrimSpace(rec[0]))
		name := strings.ToLower(strings.TrimSpace(rec[1]))
		if first && (code == "OCN" || name == "class") {
			first = false
			continue
		}
		first = false
		if strings.HasPrefix(code, "#") || code == "" {
			continue
		}
		cls, ok := classFromName[name]
		if !ok {
			return nil, fmt.Errorf("ocn map: unknown class %q for OCN %s", name, code)
		}
		out[code] = cls
		// Optional third column: the operator's human-facing brand. Purely a
		// display concern; it never affects classification.
		if len(rec) > 2 {
			if b := strings.TrimSpace(rec[2]); b != "" {
				ocnBrand[code] = b
			}
		}
	}
	return out, nil
}

func printReport(table []byte, unmapped map[string]int) {
	counts := map[byte]int{}
	for _, c := range table {
		counts[c]++
	}
	fmt.Fprintln(os.Stderr, "\n=== coverage ===")
	names := map[byte]string{cUnknown: "unknown", cWireline: "wireline",
		cWireless: "wireless", cVoIP: "voip", cTollFree: "tollfree"}
	for c := byte(0); c <= cTollFree; c++ {
		fmt.Fprintf(os.Stderr, "%10s %10d  %5.2f%%\n",
			names[c], counts[c], 100*float64(counts[c])/float64(keyCnt))
	}

	if len(unmapped) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d distinct OCNs are unmapped. Ranked worklist below.\n", len(unmapped))
	}
}

// printCarrierWorklist ranks the unmapped OCNs by the number of BLOCKS they
// hold, not the number of source rows. The distinction matters: one NXX-level
// row covers ten blocks while one block-level row covers one, so ranking by
// rows understates NXX-heavy carriers tenfold and sends you down the list in
// the wrong order.
func printCarrierWorklist(carrier []uint16, reg *carrierRegistry, classOf map[string]byte) {
	const show = 25
	costs := topCarriers(carrier, reg, classOf, 0)
	if len(costs) == 0 {
		fmt.Fprintln(os.Stderr, "\nEvery OCN holding blocks is classified in ocn.csv.")
		return
	}

	totalUnmapped := 0
	for _, c := range costs {
		totalUnmapped += c.Blocks
	}
	assigned := 0
	for _, idx := range carrier {
		if idx != 0 {
			assigned++
		}
	}

	fmt.Fprintf(os.Stderr, "\n=== unmapped OCN worklist: %d OCNs, %d of %d assigned blocks (%.1f%%) ===\n",
		len(costs), totalUnmapped, assigned, 100*float64(totalUnmapped)/float64(max(assigned, 1)))
	fmt.Fprintln(os.Stderr, "Ranked by blocks, most costly first. Add rows to ocn.csv top-down.")
	fmt.Fprintf(os.Stderr, "  %-6s %9s %7s  %s\n", "OCN", "BLOCKS", "CUM%", "COMPANY")

	run := 0
	for i, c := range costs {
		if i >= show {
			break
		}
		run += c.Blocks
		name := c.Name
		if len(name) > 44 {
			name = name[:44]
		}
		fmt.Fprintf(os.Stderr, "  %-6s %9d %6.1f%%  %s\n",
			c.OCN, c.Blocks, 100*float64(run)/float64(max(totalUnmapped, 1)), name)
	}
	if len(costs) > show {
		fmt.Fprintf(os.Stderr, "  ... and %d more. Use -worklist-out to get the full list.\n",
			len(costs)-show)
	}
}

// writeProto builds a PhoneData protobuf and writes it to path.
func writeProto(table []byte, carrier []uint16, reg *carrierRegistry,
	region []uint8, rreg *regionRegistry,
	mxRanges []mxRange, mxOps *mxOperators, path string) error {

	pd := &linetypev1.PhoneData{
		ClassTable:   buildClassTableProto(table),
		CarrierTable: reg.toProto(carrier),
		RegionTable:  rreg.toProto(region),
		BuildDate:    time.Now().UTC().Format(time.RFC3339),
		Version:      "1",
	}
	if mxOps != nil {
		pd.MxTable = mxToProto(mxRanges, mxOps)
	}

	data, err := proto.Marshal(pd)
	if err != nil {
		return fmt.Errorf("proto marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("proto write: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  proto: %d KB\n", len(data)/1024)
	return nil
}

// buildClassTableProto packs the per-key class table into nibbles.
func buildClassTableProto(table []byte) *linetypev1.ClassTable {
	packed := make([]byte, blobLen)
	for i := 0; i < keyCnt; i += 2 {
		lo := table[i] & 0x0f
		hi := table[i+1] & 0x0f
		packed[i>>1] = lo | hi<<4
	}
	return &linetypev1.ClassTable{Data: packed}
}
