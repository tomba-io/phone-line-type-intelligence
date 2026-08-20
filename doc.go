// Package linetype provides Phone Line Type Intelligence for North American
// and Mexican phone numbers.
//
// Use Line Type Intelligence to identify the carrier and phone line type —
// mobile, landline, fixed VoIP, non-fixed VoIP, toll free, and more — with no
// per-lookup API cost. Data is loaded at runtime from a Protocol Buffers file
// built from published numbering-plan allocation data (NANPA, CNAC, IFT).
//
// # Quick start
//
//	linetype.SetDataPath("data/phone_data.pb") // optional; auto-resolved
//	n := linetype.Describe("+18168037763")
//	fmt.Println(n.Class)           // wireless
//	fmt.Println(n.SMSReachable)    // true
//	fmt.Println(n.Carrier.Label()) // T-Mobile
//	fmt.Println(n.Region.Name)     // Missouri
//
// # Direct lookups
//
//	cls := linetype.Lookup("+14155551234")        // class only
//	cr  := linetype.LookupCarrier("+14155551234") // carrier info
//	rg  := linetype.LookupRegion("+14155551234")  // geographic region
//
// # Supported countries
//
//   - +1 United States and Canada (NANPA block-level, O(1) array lookup)
//   - +52 Mexico (IFT range table, O(log n) binary search)
//
// # Scope limit
//
// This package reflects block assignment, not current line type. It does not
// account for local number portability. See ACCURACY.md for error analysis.
package linetype
