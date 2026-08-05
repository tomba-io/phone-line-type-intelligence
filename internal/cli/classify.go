package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	linetype "github.com/tomba-io/phone-line-type-intelligence"
)

var classifyCmd = &cobra.Command{
	Use:   "classify [file]",
	Short: "Classify phone numbers from a file or stdin",
	Long: `Read phone numbers (one per line, E.164 format) and print a classification
summary. Reads from stdin if no file is given.

Examples:
  lti classify list.txt
  cat numbers.txt | lti classify
  echo "+18168037763" | lti classify`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := linetype.Validate(); err != nil {
			return err
		}

		var input *os.File
		if len(args) > 0 {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			input = f
		} else {
			input = os.Stdin
		}

		start := time.Now()
		counts := map[linetype.Class]int{}
		total, skipped := 0, 0

		scanner := bufio.NewScanner(input)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			n := linetype.Describe(line)
			if n.Valid {
				counts[n.Class]++
				total++
			} else {
				skipped++
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}

		elapsed := time.Since(start)
		fmt.Println("lti classify summary")
		fmt.Printf("   input lines      %7d   in %s\n", total, elapsed.Truncate(time.Millisecond))
		fmt.Println("   ---")

		for _, r := range []struct {
			c    linetype.Class
			name string
		}{
			{linetype.Wireless, "wireless"},
			{linetype.VoIP, "voip"},
			{linetype.Wireline, "wireline"},
			{linetype.TollFree, "tollfree"},
			{linetype.Unknown, "unknown"},
		} {
			c := counts[r.c]
			if c == 0 {
				continue
			}
			pct := float64(c) / float64(total) * 100
			bar := strings.Repeat("#", int(pct/10))
			pad := strings.Repeat(".", 10-len(bar))
			fmt.Printf("   %-10s   %7d   %5.1f%%  %s%s\n", r.name, c, pct, bar, pad)
		}
		fmt.Println("   ---")
		fmt.Printf("   emitted          %7d\n", total)
		if skipped > 0 {
			fmt.Printf("   skipped (non-NANP/MX) %4d\n", skipped)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(classifyCmd)
}
