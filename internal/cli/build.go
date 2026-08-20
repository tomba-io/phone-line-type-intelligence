package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build binary tables from source data",
	Long: `Build the protobuf data file from downloaded numbering-plan data.
This is a wrapper around the ltbuild command.

Examples:
  lti build -- -co cocodes.txt -blocks blocks.txt -ocn data/ocn.csv -proto-out data/phone_data.pb
  lti build -- -inspect cocodes.txt
  lti build -- -co cocodes.txt -mx pnn.csv -proto-out data/phone_data.pb`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Find ltbuild binary or source
		ltbuild, err := exec.LookPath("ltbuild")
		if err != nil {
			// Try running from source
			goCmd := exec.Command("go", append([]string{"run", "./cmd/ltbuild"}, args...)...)
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr
			goCmd.Stdin = os.Stdin
			return goCmd.Run()
		}
		c := exec.Command(ltbuild, args...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		c.Stdin = os.Stdin
		return c.Run()
	},
}

var fetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Download source data from NANPA, CNAC, and IFT",
	Long: `Download numbering-plan source data needed to rebuild the tables.
Runs the get-data.sh script.

If reports.nanpa.com returns HTTP 403, set up a proxy:
  cp scripts/.proxyrc.example scripts/.proxyrc
  # edit .proxyrc with your credentials`,
	RunE: func(cmd *cobra.Command, args []string) error {
		script := "scripts/get-data.sh"
		if _, err := os.Stat(script); err != nil {
			// Fallback to old location
			script = "cmd/buildlinetype/get-data.sh"
			if _, err := os.Stat(script); err != nil {
				return fmt.Errorf("get-data.sh not found in scripts/ or cmd/buildlinetype/")
			}
		}
		c := exec.Command("bash", script)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(fetchCmd)
}
