// Example: read a CSV of phone numbers, classify each, and output enriched CSV.
//
// Usage:
//
//	echo -e "phone\n+18168037763\n+12025551234" | go run ./examples/csv-classify
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"

	linetype "github.com/tomba-io/phone-line-type-intelligence"
)

func main() {
	if err := linetype.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	r := csv.NewReader(os.Stdin)
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Write header
	_ = w.Write([]string{"phone", "line_type", "sms_reachable", "carrier", "region", "country"})

	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if len(rec) == 0 {
			continue
		}

		phone := rec[0]

		// Skip header row
		if first {
			first = false
			if phone == "phone" || phone == "Phone" || phone == "number" {
				continue
			}
		}

		n := linetype.Describe(phone)
		if !n.Valid {
			_ = w.Write([]string{phone, "invalid", "", "", "", ""})
			continue
		}

		_ = w.Write([]string{
			n.E164,
			n.Class.String(),
			linetype.YesNo(n.SMSReachable),
			linetype.Or(n.Carrier.Label(), ""),
			linetype.Or(n.Region.Code, ""),
			linetype.Or(n.CountryCode, ""),
		})
	}
}
