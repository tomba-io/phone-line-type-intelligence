// Package linetype provides Line Type Intelligence for North American (+1) and
// Mexican (+52) phone numbers.
//
// Use Line Type Intelligence to identify the carrier and phone line type, such
// as mobile, landline, fixed VoIP, non-fixed VoIP, toll free, and more — with
// no per-lookup API cost.
//
// Data is loaded at runtime from a Protocol Buffers file built from published
// numbering-plan allocation data (NANPA, CNAC, IFT).
//
// IMPORTANT SCOPE LIMIT: this package reflects *block assignment*, i.e. which
// carrier a number range was originally allocated to. It does not account for
// local number portability. See ACCURACY.md for the expected error rate.
package linetype

import (
	"errors"
	"fmt"
)

// Class is the assignment-derived line type of a number range.
type Class uint8

const (
	// Unknown means the block is unassigned, or the holding OCN is absent from
	// the classification map. Never treat Unknown as a synonym for any other
	// class — abstain instead.
	Unknown Class = iota
	Wireline // landline (ILEC, RBOC, traditional fixed-line)
	Wireless // mobile / cellular (PCS, wireless carriers)
	VoIP     // interconnected VoIP and CLEC-held ranges; frequently SMS-reachable
	TollFree // toll-free numbers (800, 888, 877, 866, 855, 844, 833)
	Invalid  // structurally not a dialable NANP geographic number
)

var classNames = [...]string{"unknown", "wireline", "wireless", "voip", "tollfree", "invalid"}

func (c Class) String() string {
	if int(c) >= len(classNames) {
		return "unknown"
	}
	return classNames[c]
}

// MarshalText renders the class as its lowercase name for JSON serialisation.
func (c Class) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// SMSReachable reports whether the class is plausibly able to receive SMS.
// VoIP counts: a large share of CLEC and interconnected-VoIP ranges are
// SMS-enabled, and dropping them discards reachable numbers.
func (c Class) SMSReachable() bool { return c == Wireless || c == VoIP }

// keySpace covers seven-digit prefixes 2000000..9999999 (NPA and NXX both
// start at 2 in the NANP), one nibble each, two per byte.
const (
	keyBase  = 2_000_000
	keyCount = 10_000_000 - keyBase // 8,000,000
	blobLen  = keyCount / 2         // 4,000,000 bytes
)

// ErrBlobSize is returned by Validate if the class table is the wrong size.
var ErrBlobSize = errors.New("linetype: class table has unexpected size")

// Validate checks the class table. Call it in main; a truncated or stale table
// is a deploy error, not a per-request condition.
func Validate() error {
	ensureLoaded()
	if loadErr != nil {
		return loadErr
	}
	if len(classData) != blobLen {
		return fmt.Errorf("%w: got %d bytes, want %d", ErrBlobSize, len(classData), blobLen)
	}
	return nil
}

// prefixKey extracts the seven-digit NPA-NXX-B prefix from an E.164 string of
// the form +1NPANXXXXXX.
func prefixKey(e164 string) (uint32, bool) {
	if len(e164) != 12 || e164[0] != '+' || e164[1] != '1' {
		return 0, false
	}
	for i := 2; i < 12; i++ {
		if c := e164[i]; c < '0' || c > '9' {
			return 0, false
		}
	}
	var k uint32
	for i := 2; i < 9; i++ {
		k = k*10 + uint32(e164[i]-'0')
	}
	if k < keyBase || (k/1_000)%10 < 2 {
		return 0, false
	}
	return k, true
}

// Lookup returns the assignment-derived class of an E.164 +1 number.
// It is safe for concurrent use.
func Lookup(e164 string) Class {
	ensureLoaded()
	k, ok := prefixKey(e164)
	if !ok {
		return Invalid
	}
	return classAt(k)
}

// LookupPrefix returns the class for an already-extracted seven-digit
// NPA-NXX-B prefix.
func LookupPrefix(prefix uint32) Class {
	ensureLoaded()
	if prefix < keyBase || prefix >= 10_000_000 || len(classData) != blobLen {
		return Unknown
	}
	i := prefix - keyBase
	b := classData[i>>1]
	if i&1 == 0 {
		return Class(b & 0x0f)
	}
	return Class(b >> 4)
}

// classAt is the shared body of Lookup and Describe.
func classAt(k uint32) Class {
	if len(classData) != blobLen {
		return Unknown
	}
	i := k - keyBase
	b := classData[i>>1]
	if i&1 == 0 {
		return Class(b & 0x0f)
	}
	return Class(b >> 4)
}
