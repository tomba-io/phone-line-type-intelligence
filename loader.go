package linetype

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	linetypev1 "github.com/tomba-io/phone-line-type-intelligence/proto/linetype/v1"
	"google.golang.org/protobuf/proto"
)

var (
	dataOnce sync.Once
	loadErr  error
	dataPath string

	// Cached slices set once by loadData for zero-indirection lookups.
	classData      []byte
	carrierNxxBase []byte
	carrierExList  []*linetypev1.CarrierException
	carrierByIdx   []Carrier
	regionNxxIdx   []byte
	regionByIdx    []Region
	mxRangeList    []*linetypev1.MXRange
	mxBucketList   []*linetypev1.MXBucket
	mxCarriers     []Carrier
)

// SetDataPath configures the path to phone_data.pb. Must be called before any
// Lookup function. If not called, the path is resolved from LINETYPE_DATA_PATH
// env var or defaults to data/phone_data.pb relative to the executable.
func SetDataPath(path string) {
	dataPath = path
}

// LoadData explicitly loads and parses the protobuf data file. It is safe to
// call from multiple goroutines; the first call does the work.
func LoadData() error {
	ensureLoaded()
	return loadErr
}

func ensureLoaded() {
	dataOnce.Do(func() {
		loadErr = doLoad()
	})
}

func resolveDataPath() string {
	if dataPath != "" {
		return dataPath
	}
	if env := os.Getenv("LINETYPE_DATA_PATH"); env != "" {
		return env
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "data", "phone_data.pb")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join("data", "phone_data.pb")
}

func doLoad() error {
	raw, err := os.ReadFile(resolveDataPath())
	if err != nil {
		return fmt.Errorf("linetype: %w", err)
	}
	pd := &linetypev1.PhoneData{}
	if err := proto.Unmarshal(raw, pd); err != nil {
		return fmt.Errorf("linetype: unmarshal phone_data.pb: %w", err)
	}
	populateCache(pd)
	return nil
}

func populateCache(pd *linetypev1.PhoneData) {
	// Class table.
	if ct := pd.GetClassTable(); ct != nil {
		classData = ct.GetData()
	}

	// Carrier table.
	if ct := pd.GetCarrierTable(); ct != nil {
		carrierNxxBase = ct.GetNxxBase()
		carrierExList = ct.GetExceptions()
		pbCarriers := ct.GetCarriers()
		carrierByIdx = make([]Carrier, len(pbCarriers))
		for i, c := range pbCarriers {
			carrierByIdx[i] = Carrier{OCN: c.GetOcn(), Name: c.GetName(), Brand: c.GetBrand()}
		}
	}

	// Region table.
	if rt := pd.GetRegionTable(); rt != nil {
		regionNxxIdx = rt.GetNxxIndex()
		pbRegions := rt.GetRegions()
		regionByIdx = make([]Region, len(pbRegions))
		for i, r := range pbRegions {
			regionByIdx[i] = Region{
				Code:        r.GetCode(),
				Name:        regionNames[r.GetCode()],
				CountryCode: r.GetCountryCode(),
				Country:     r.GetCountry(),
			}
		}
	}

	// MX table.
	if mt := pd.GetMxTable(); mt != nil {
		mxRangeList = mt.GetRanges()
		mxBucketList = mt.GetPrefixIndices()
		pbCarriers := mt.GetCarriers()
		mxCarriers = make([]Carrier, len(pbCarriers))
		for i, c := range pbCarriers {
			mxCarriers[i] = Carrier{OCN: c.GetOcn(), Name: c.GetName(), Brand: c.GetBrand()}
		}
	}
}

// carrierAtIdx returns the Carrier at index idx, or empty Carrier if out of range.
func carrierAtIdx(idx int) Carrier {
	if idx <= 0 || idx >= len(carrierByIdx) {
		return Carrier{}
	}
	return carrierByIdx[idx]
}

// regionAtIdx returns the Region at index idx, or empty Region if out of range.
func regionAtIdx(idx int) Region {
	if idx <= 0 || idx >= len(regionByIdx) {
		return Region{}
	}
	return regionByIdx[idx]
}

// carrierIdxCompact looks up the carrier index for key k using the NXX-base +
// exceptions format from the protobuf CarrierTable.
func carrierIdxCompact(k uint32) int {
	if len(carrierNxxBase) == 0 {
		return 0
	}
	nxxOff := (k - keyBase) / 10
	block := (k - keyBase) % 10

	// Read NXX base carrier (uint16 LE).
	off := nxxOff * 2
	if int(off+2) > len(carrierNxxBase) {
		return 0
	}
	baseIdx := int(binary.LittleEndian.Uint16(carrierNxxBase[off:]))

	// Binary search exceptions for (nxxOff, block).
	target := nxxOff*10 + block
	lo, hi := 0, len(carrierExList)
	for lo < hi {
		mid := lo + (hi-lo)/2
		ex := carrierExList[mid]
		key := ex.GetNxxOffset()*10 + ex.GetBlock()
		if key == target {
			return int(ex.GetCarrierIndex())
		}
		if key < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return baseIdx
}
