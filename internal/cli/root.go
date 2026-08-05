package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "lti",
	Short: "Phone Line Type Intelligence",
	Long: `Use Line Type Intelligence to identify the carrier and phone line type,
such as mobile, landline, fixed VoIP, non-fixed VoIP, toll free, and more —
with no per-lookup API cost.

Classifies +1 (US, Canada) and +52 (Mexico) numbers from published
numbering-plan allocation data embedded directly in the binary.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("lti v%s, commit %s, built at %s\n", version, commit, date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
