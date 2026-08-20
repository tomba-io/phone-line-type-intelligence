package linetype

import "strings"

// Number is everything this package knows about one number, assembled from the
// class, carrier and region tables.
type Number struct {
	Valid         bool
	Country       string // "United States", "Canada", "Mexico"
	CountryCode   string // "US", "CA", "MX"
	E164          string // +18168037763
	NPA           string // 816
	NXX           string // 803
	Block         string // 7
	Class         Class
	SMSReachable  bool
	Carrier       Carrier
	Region        Region

	// body caches the national digits for lazy formatting.
	body string // e.g. "8168037763" for +1, "5510001234" for +52
	cc   byte   // 1 or 52
}

// International returns the E.164 number in international format.
// For +1: "+1 816-803-7763". For +52: "+52 55 1000 1234".
func (n Number) International() string {
	if !n.Valid {
		return n.E164
	}
	if n.cc == 52 {
		rest := n.body[len(n.NPA):]
		return "+52 " + n.NPA + " " + rest[:len(rest)-4] + " " + rest[len(rest)-4:]
	}
	// +1: use a fixed buffer to avoid allocation
	var buf [16]byte // "+1 NNN-NNN-NNNN" = 16 chars
	buf[0] = '+'
	buf[1] = '1'
	buf[2] = ' '
	copy(buf[3:6], n.body[0:3])
	buf[6] = '-'
	copy(buf[7:10], n.body[3:6])
	buf[10] = '-'
	copy(buf[11:15], n.body[6:10])
	return string(buf[:15])
}

// National returns the number in national format.
// For +1: "(816) 803-7763". For +52: "55 1000 1234".
func (n Number) National() string {
	if !n.Valid {
		return n.E164
	}
	if n.cc == 52 {
		rest := n.body[len(n.NPA):]
		return n.NPA + " " + rest[:len(rest)-4] + " " + rest[len(rest)-4:]
	}
	var buf [14]byte // "(NNN) NNN-NNNN" = 14 chars
	buf[0] = '('
	copy(buf[1:4], n.body[0:3])
	buf[4] = ')'
	buf[5] = ' '
	copy(buf[6:9], n.body[3:6])
	buf[9] = '-'
	copy(buf[10:14], n.body[6:10])
	return string(buf[:])
}

// Describe assembles everything known about an E.164 +1 or +52 number in one
// pass. Formatting strings (International, National) are computed lazily via
// methods to keep the hot path zero-allocation.
func Describe(e164 string) Number {
	ensureLoaded()
	n := Number{E164: e164}

	if mx, ok := mxNumber(e164); ok {
		n.Valid = true
		n.cc = 52
		n.CountryCode, n.Country = "MX", "Mexico"
		n.body = e164[3:]
		n.NPA = mxAreaCode(n.body)
		n.Class, n.Carrier = lookupMX(mx)
		n.SMSReachable = n.Class.SMSReachable()
		n.Region = Region{Country: "Mexico", CountryCode: "MX"}
		return n
	}

	k, ok := prefixKey(e164)
	if !ok {
		return n
	}
	n.Valid = true
	n.cc = 1
	n.body = e164[2:]
	n.NPA, n.NXX, n.Block = n.body[0:3], n.body[3:6], n.body[6:7]
	n.Class = classAt(k)
	n.SMSReachable = n.Class.SMSReachable()
	n.Carrier = carrierAt(k)
	n.Region = LookupRegion(e164)
	if n.Region.CountryCode != "" {
		n.CountryCode, n.Country = n.Region.CountryCode, n.Region.Country
	}
	return n
}

// CarrierLabel is the name to show a human.
func (n Number) CarrierLabel() string { return n.Carrier.Label() }

func mxAreaCode(body string) string {
	if len(body) >= 2 {
		switch body[:2] {
		case "55", "56", "81", "33":
			return body[:2]
		}
	}
	if len(body) >= 3 {
		return body[:3]
	}
	return ""
}

// YesNo renders a bool for human-facing reports.
func YesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// Or returns fallback when s is empty.
func Or(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
