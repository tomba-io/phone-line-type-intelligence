package linetype

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Table availability
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}
}

func TestCarrierAvailable(t *testing.T) {
	if !CarrierAvailable() {
		t.Error("CarrierAvailable() = false; expected true with real data")
	}
}

func TestRegionAvailable(t *testing.T) {
	if !RegionAvailable() {
		t.Error("RegionAvailable() = false; expected true with real data")
	}
}

func TestMXAvailable(t *testing.T) {
	if !MXAvailable() {
		t.Error("MXAvailable() = false; expected true with real data")
	}
}

// ---------------------------------------------------------------------------
// Lookup — input validation
// ---------------------------------------------------------------------------

func TestLookupInvalid(t *testing.T) {
	tests := []struct {
		e164 string
		want Class
	}{
		{"+4420712345", Invalid},     // not +1
		{"12025551234", Invalid},     // missing +
		{"+1202555123", Invalid},     // too short (9 digits)
		{"+120255512345", Invalid},   // too long (11 digits)
		{"+1202555123A", Invalid},    // non-digit in body
		{"+1002551234", Invalid},     // NPA starts with 0
		{"+1200551234", Invalid},     // NXX starts with 0 (200 is valid but 0xx not)
		{"+1212055xxxx", Invalid},    // non-digits
		{"", Invalid},                // empty
		{"+", Invalid},               // just a plus
		{"+1", Invalid},              // just country code
	}
	for _, tt := range tests {
		got := Lookup(tt.e164)
		if got != tt.want {
			t.Errorf("Lookup(%q) = %v, want %v", tt.e164, got, tt.want)
		}
	}
}

func TestLookupPrefix(t *testing.T) {
	if got := LookupPrefix(1_999_999); got != Unknown {
		t.Errorf("LookupPrefix(1999999) = %v, want Unknown", got)
	}
	if got := LookupPrefix(10_000_000); got != Unknown {
		t.Errorf("LookupPrefix(10000000) = %v, want Unknown", got)
	}
	// Valid range should not panic
	_ = LookupPrefix(2_000_000)
	_ = LookupPrefix(9_999_999)
}

// ---------------------------------------------------------------------------
// Describe — US/CA
// ---------------------------------------------------------------------------

func TestDescribeUSFormatting(t *testing.T) {
	n := Describe("+12025551234")
	if !n.Valid {
		t.Fatal("returned Valid=false")
	}
	if n.NPA != "202" {
		t.Errorf("NPA = %q, want 202", n.NPA)
	}
	if n.NXX != "555" {
		t.Errorf("NXX = %q, want 555", n.NXX)
	}
	if n.Block != "1" {
		t.Errorf("Block = %q, want 1", n.Block)
	}
	if n.International() != "+1 202-555-1234" {
		t.Errorf("International = %q", n.International())
	}
	if n.National() != "(202) 555-1234" {
		t.Errorf("National = %q", n.National())
	}
}

func TestDescribeInvalid(t *testing.T) {
	n := Describe("+442071234567")
	if n.Valid {
		t.Error("Describe(+442071234567) should be Valid=false")
	}
	if n.E164 != "+442071234567" {
		t.Error("E164 should preserve input even when invalid")
	}
}

// ---------------------------------------------------------------------------
// Describe — Mexico
// ---------------------------------------------------------------------------

func TestDescribeMexico(t *testing.T) {
	if !MXAvailable() {
		t.Skip("Mexico table not available")
	}
	tests := []struct {
		e164    string
		npa     string
		country string
	}{
		// Two-digit area codes
		{"+525510001234", "55", "Mexico"}, // Mexico City
		{"+525610001234", "56", "Mexico"}, // Mexico City alt
		{"+528110001234", "81", "Mexico"}, // Monterrey
		{"+523310001234", "33", "Mexico"}, // Guadalajara
		// Three-digit area code
		{"+529981110500", "998", "Mexico"}, // Cancun
	}
	for _, tt := range tests {
		n := Describe(tt.e164)
		if !n.Valid {
			t.Errorf("Describe(%q) returned Valid=false", tt.e164)
			continue
		}
		if n.CountryCode != "MX" {
			t.Errorf("Describe(%q).CountryCode = %q, want MX", tt.e164, n.CountryCode)
		}
		if n.NPA != tt.npa {
			t.Errorf("Describe(%q).NPA = %q, want %q", tt.e164, n.NPA, tt.npa)
		}
		if n.Country != tt.country {
			t.Errorf("Describe(%q).Country = %q, want %q", tt.e164, n.Country, tt.country)
		}
	}
}

func TestMexicoInvalid(t *testing.T) {
	// +52 with wrong length
	n := Describe("+52551000123") // 9 digits
	if n.Valid {
		t.Error("9-digit MX number should be invalid")
	}
	n = Describe("+5255100012345") // 11 digits
	if n.Valid {
		t.Error("11-digit MX number should be invalid")
	}
	// National number starting with 0 or 1
	n = Describe("+520110001234")
	if n.Valid {
		t.Error("MX number starting with 0 should be invalid")
	}
}

// ---------------------------------------------------------------------------
// Mexico range edge cases
// ---------------------------------------------------------------------------

func TestMexicoRangeEdges(t *testing.T) {
	if !MXAvailable() {
		t.Skip("Mexico table not available")
	}
	// Test that the first and last numbers in a known range return a class,
	// and one past the end returns Unknown or a different class.
	// We can't hardcode specific ranges since data changes, but we can verify
	// that Describe doesn't panic on boundary values.
	boundaries := []string{
		"+522000000000", // lowest possible national number
		"+529999999999", // highest possible national number
	}
	for _, e := range boundaries {
		n := Describe(e)
		if !n.Valid {
			t.Errorf("Describe(%q) should be Valid", e)
		}
		// Class should be a known value (not panic)
		_ = n.Class.String()
	}
}

// ---------------------------------------------------------------------------
// Class methods
// ---------------------------------------------------------------------------

func TestClassString(t *testing.T) {
	for _, tt := range []struct {
		c    Class
		want string
	}{
		{Unknown, "unknown"}, {Wireline, "wireline"}, {Wireless, "wireless"},
		{VoIP, "voip"}, {TollFree, "tollfree"}, {Invalid, "invalid"},
		{Class(99), "unknown"},
	} {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("Class(%d).String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}

func TestClassMarshalText(t *testing.T) {
	for _, c := range []Class{Unknown, Wireline, Wireless, VoIP, TollFree, Invalid} {
		b, err := c.MarshalText()
		if err != nil {
			t.Errorf("MarshalText(%v) error: %v", c, err)
		}
		if string(b) != c.String() {
			t.Errorf("MarshalText(%v) = %q, want %q", c, b, c.String())
		}
	}
}

func TestSMSReachable(t *testing.T) {
	want := map[Class]bool{
		Unknown: false, Wireline: false, Wireless: true,
		VoIP: true, TollFree: false, Invalid: false,
	}
	for c, expected := range want {
		if c.SMSReachable() != expected {
			t.Errorf("%v.SMSReachable() = %v, want %v", c, c.SMSReachable(), expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Carrier
// ---------------------------------------------------------------------------

func TestLookupCarrier(t *testing.T) {
	if !CarrierAvailable() {
		t.Skip("Carrier table not available")
	}
	c := LookupCarrier("+12025551234")
	_ = c.Label()
	_ = c.String()
	_ = c.Known()

	// Invalid input returns empty carrier
	c2 := LookupCarrier("+44123")
	if c2.Known() {
		t.Error("invalid number should return unknown carrier")
	}
}

func TestCarrierString(t *testing.T) {
	if (Carrier{}).String() != "unknown" {
		t.Error("empty carrier should be 'unknown'")
	}
	if (Carrier{OCN: "1234"}).String() != "1234" {
		t.Error("OCN-only carrier should show OCN")
	}
	if (Carrier{OCN: "1234", Name: "Foo"}).String() != "Foo" {
		t.Error("Name should take precedence over OCN")
	}
	if (Carrier{OCN: "1234", Name: "Foo", Brand: "Bar"}).String() != "Bar" {
		t.Error("Brand should take precedence over Name")
	}
}

func TestCarrierLabel(t *testing.T) {
	if (Carrier{Name: "Foo"}).Label() != "Foo" {
		t.Error("Label should return Name when Brand is empty")
	}
	if (Carrier{Name: "Foo", Brand: "Bar"}).Label() != "Bar" {
		t.Error("Label should prefer Brand")
	}
}

// ---------------------------------------------------------------------------
// Region
// ---------------------------------------------------------------------------

func TestLookupRegion(t *testing.T) {
	// Use NXX 200, not 555. NXX 555 is often unassigned in NANPA data.
	r := LookupRegion("+12122001234")
	if !r.Known() {
		t.Skip("Region unknown for 212-200")
	}
	if r.Code != "NY" {
		t.Errorf("Region.Code = %q, want NY", r.Code)
	}
	if r.Name != "New York" {
		t.Errorf("Region.Name = %q, want New York", r.Name)
	}

	// Invalid input
	r2 := LookupRegion("+44123")
	if r2.Known() {
		t.Error("invalid number should return unknown region")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestHelpers(t *testing.T) {
	if YesNo(true) != "Yes" || YesNo(false) != "No" {
		t.Error("YesNo broken")
	}
	if Or("", "fb") != "fb" || Or("  ", "fb") != "fb" || Or("v", "fb") != "v" {
		t.Error("Or broken")
	}
}

// ---------------------------------------------------------------------------
// Golden-file test: ltclass benchmark against list.txt
// ---------------------------------------------------------------------------

func TestGoldenListTxt(t *testing.T) {
	f, err := os.Open("list.txt")
	if err != nil {
		t.Skip("list.txt not found")
	}
	defer func() { _ = f.Close() }()

	counts := map[Class]int{}
	total := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		n := Describe(line)
		if n.Valid {
			counts[n.Class]++
			total++
		}
	}

	// These are the expected counts from the phonevalidator-tomba benchmark.
	// If the data tables are rebuilt, update these values.
	expect := map[Class]int{
		Wireless: 1326,
		VoIP:     191,
		Wireline: 840,
		Unknown:  20,
	}

	if total != 2377 {
		t.Errorf("total classified = %d, want 2377", total)
	}
	for cls, want := range expect {
		if got := counts[cls]; got != want {
			t.Errorf("%v count = %d, want %d", cls, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Concurrency
// ---------------------------------------------------------------------------

func TestConcurrency(t *testing.T) {
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			Lookup("+12025551234")
			LookupCarrier("+12025551234")
			LookupRegion("+12025551234")
			Describe("+525510001234")
		}()
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Fuzz
// ---------------------------------------------------------------------------

func FuzzLookup(f *testing.F) {
	f.Add("+12025551234")
	f.Add("+14155551234")
	f.Add("+525510001234")
	f.Add("+442071234567")
	f.Add("")
	f.Add("+1")
	f.Add("not a number")
	f.Add("+10000000000")
	f.Add("+19999999999")

	f.Fuzz(func(t *testing.T, e164 string) {
		// Must not panic on any input
		c := Lookup(e164)
		_ = c.String()
		_ = c.SMSReachable()

		n := Describe(e164)
		_ = n.CarrierLabel()
		if n.Valid {
			if n.Class > Invalid {
				t.Errorf("Describe(%q) returned out-of-range Class %d", e164, n.Class)
			}
		}

		_ = LookupCarrier(e164)
		_ = LookupRegion(e164)
	})
}

// ---------------------------------------------------------------------------
// Example tests (verified by go test, shown in go doc)
// ---------------------------------------------------------------------------

func ExampleDescribe() {
	n := Describe("+12025551234")
	fmt.Println(n.Valid)
	fmt.Println(n.NPA)
	fmt.Println(n.NXX)
	fmt.Println(n.International())
	fmt.Println(n.National())
	// Output:
	// true
	// 202
	// 555
	// +1 202-555-1234
	// (202) 555-1234
}

func ExampleLookup() {
	c := Lookup("+12025551234")
	fmt.Println(c)
	fmt.Println(c.SMSReachable())
	// Output:
	// unknown
	// false
}

func ExampleLookup_invalid() {
	c := Lookup("+442071234567")
	fmt.Println(c)
	// Output:
	// invalid
}

func ExampleClass_SMSReachable() {
	fmt.Println(Wireless.SMSReachable())
	fmt.Println(VoIP.SMSReachable())
	fmt.Println(Wireline.SMSReachable())
	// Output:
	// true
	// true
	// false
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkLookup(b *testing.B) {
	for range b.N {
		Lookup("+12025551234")
	}
}

func BenchmarkDescribe(b *testing.B) {
	for range b.N {
		Describe("+12025551234")
	}
}

func BenchmarkDescribeMexico(b *testing.B) {
	for range b.N {
		Describe("+525510001234")
	}
}

func BenchmarkLookupCarrier(b *testing.B) {
	for range b.N {
		LookupCarrier("+12025551234")
	}
}

func BenchmarkLookupRegion(b *testing.B) {
	for range b.N {
		LookupRegion("+12025551234")
	}
}

// BenchmarkBulkThroughput measures realistic bulk classification throughput
// by processing all numbers from list.txt.
func BenchmarkBulkThroughput(b *testing.B) {
	f, err := os.Open("list.txt")
	if err != nil {
		b.Skip("list.txt not found")
	}
	var numbers []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			numbers = append(numbers, line)
		}
	}
	_ = f.Close()

	b.ResetTimer()
	for range b.N {
		for _, num := range numbers {
			Describe(num)
		}
	}
	b.ReportMetric(float64(len(numbers)), "numbers/op")
}

// BenchmarkParallel verifies no contention on sync.Once paths.
func BenchmarkParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		numbers := []string{"+12025551234", "+14155551234", "+18168037763", "+525510001234"}
		for pb.Next() {
			Describe(numbers[i%len(numbers)])
			i++
		}
	})
}
