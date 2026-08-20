package linetype

const (
	regionBase    = 200_000
	regionCount   = 800_000
	regionBlobLen = regionCount
)

// Region is the geography a number range is assigned to.
type Region struct {
	Code        string // state or province code, e.g. "MO", "ON"
	Name        string // full name, e.g. "Missouri", "Ontario"
	Country     string // e.g. "United States"
	CountryCode string // ISO 3166-1 alpha-2, e.g. "US"
}

// Known reports whether any region was on file.
func (r Region) Known() bool { return r.Code != "" }

// RegionAvailable reports whether a region table was loaded.
func RegionAvailable() bool {
	ensureLoaded()
	return len(regionNxxIdx) == regionBlobLen && len(regionByIdx) > 0
}

// LookupRegion returns the geography of an E.164 +1 number's NXX.
func LookupRegion(e164 string) Region {
	ensureLoaded()
	k, ok := prefixKey(e164)
	if !ok {
		return Region{}
	}
	if len(regionNxxIdx) != regionBlobLen {
		return Region{}
	}
	i := k/10 - regionBase
	if i >= uint32(len(regionNxxIdx)) {
		return Region{}
	}
	idx := int(regionNxxIdx[i])
	return regionAtIdx(idx)
}
