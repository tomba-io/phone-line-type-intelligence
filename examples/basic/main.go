// Example: basic line type lookup for a single number.
package main

import (
	"fmt"
	"os"

	linetype "github.com/tomba-io/phone-line-type-intelligence"
)

func main() {
	if err := linetype.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	number := "+18168037763"
	if len(os.Args) > 1 {
		number = os.Args[1]
	}

	n := linetype.Describe(number)
	if !n.Valid {
		fmt.Printf("%s is not a valid US/CA/MX number\n", number)
		os.Exit(1)
	}

	fmt.Printf("Number:        %s\n", n.E164)
	fmt.Printf("International: %s\n", n.International())
	fmt.Printf("National:      %s\n", n.National())
	fmt.Printf("Line Type:     %s\n", n.Class)
	fmt.Printf("SMS Reachable: %s\n", linetype.YesNo(n.SMSReachable))
	fmt.Printf("Carrier:       %s\n", linetype.Or(n.Carrier.Label(), "unknown"))
	fmt.Printf("OCN:           %s\n", linetype.Or(n.Carrier.OCN, "unknown"))
	fmt.Printf("Region:        %s\n", linetype.Or(n.Region.Name, "unknown"))
	fmt.Printf("State:         %s\n", linetype.Or(n.Region.Code, "unknown"))
	fmt.Printf("Country:       %s (%s)\n", linetype.Or(n.Country, "unknown"), linetype.Or(n.CountryCode, "unknown"))
}
