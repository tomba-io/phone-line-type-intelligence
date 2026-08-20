package linetype

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

// CarrierAvailable reports whether a carrier table was loaded.
func CarrierAvailable() bool {
	ensureLoaded()
	return len(carrierNxxBase) > 0 && len(carrierByIdx) > 0
}

// LookupCarrier returns the company holding the range an E.164 +1 number falls in.
func LookupCarrier(e164 string) Carrier {
	ensureLoaded()
	k, ok := prefixKey(e164)
	if !ok {
		return Carrier{}
	}
	return carrierAt(k)
}

// LookupCarrierPrefix is LookupCarrier for a seven-digit NPA-NXX-B prefix.
func LookupCarrierPrefix(prefix uint32) Carrier {
	ensureLoaded()
	if prefix < keyBase || prefix >= 10_000_000 {
		return Carrier{}
	}
	return carrierAt(prefix)
}

func carrierAt(k uint32) Carrier {
	idx := carrierIdxCompact(k)
	return carrierAtIdx(idx)
}
