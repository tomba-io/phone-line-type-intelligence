package linetype

import "sort"

const (
	mxBucketCount = 800 // 3-digit prefixes 200..999
	mxBucketBase  = 200
)

// MXAvailable reports whether a Mexican range table was loaded.
func MXAvailable() bool {
	ensureLoaded()
	return len(mxRangeList) > 0
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
	ensureLoaded()
	if len(mxRangeList) == 0 {
		return Unknown, Carrier{}
	}

	lo, hi := 0, len(mxRangeList)
	if len(mxBucketList) == mxBucketCount {
		prefix := int(n / 10_000_000)
		bi := prefix - mxBucketBase
		if bi < 0 || bi >= mxBucketCount {
			return Unknown, Carrier{}
		}
		bucket := mxBucketList[bi]
		lo = int(bucket.GetStartIndex())
		hi = int(bucket.GetEndIndex())
		if lo >= hi {
			return Unknown, Carrier{}
		}
	}

	i := lo + sort.Search(hi-lo, func(j int) bool {
		return mxRangeList[lo+j].GetStart() > n
	})
	if i == lo {
		return Unknown, Carrier{}
	}
	r := mxRangeList[i-1]
	if n < r.GetStart() || n > r.GetEnd() {
		return Unknown, Carrier{}
	}
	class := Class(r.GetLineClass())
	cidx := int(r.GetCarrierIndex())
	if cidx <= 0 || cidx >= len(mxCarriers) {
		return class, Carrier{}
	}
	return class, mxCarriers[cidx]
}
