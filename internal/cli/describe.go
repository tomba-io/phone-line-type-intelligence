package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	linetype "github.com/tomba-io/phone-line-type-intelligence"
)

var describeJSON bool

var describeCmd = &cobra.Command{
	Use:   "describe <e164>",
	Short: "Describe a single phone number",
	Long: `Look up line type, carrier, and region for an E.164 phone number.

Examples:
  lti describe +18168037763
  lti describe +525510001234
  lti describe --json +14155551234`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := linetype.Validate(); err != nil {
			return err
		}

		n := linetype.Describe(args[0])
		if !n.Valid {
			fmt.Fprintf(os.Stderr, "%s is not a valid US/CA/MX number\n", args[0])
			os.Exit(1)
		}

		if describeJSON {
			out := map[string]any{
				"e164":          n.E164,
				"valid":         n.Valid,
				"line_type":     n.Class.String(),
				"sms_reachable": n.SMSReachable,
				"international": n.International(),
				"national":      n.National(),
				"npa":           n.NPA,
				"nxx":           n.NXX,
				"block":         n.Block,
				"country":       n.Country,
				"country_code":  n.CountryCode,
				"carrier_ocn":   n.Carrier.OCN,
				"carrier_name":  n.Carrier.Name,
				"carrier_brand": n.Carrier.Brand,
				"carrier_label": n.Carrier.Label(),
				"region_code":   n.Region.Code,
				"region_name":   n.Region.Name,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
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
		fmt.Printf("Country:       %s (%s)\n",
			linetype.Or(n.Country, "unknown"),
			linetype.Or(n.CountryCode, "unknown"))
		return nil
	},
}

func init() {
	describeCmd.Flags().BoolVar(&describeJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(describeCmd)
}
