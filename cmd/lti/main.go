// Command lti is the Phone Line Type Intelligence CLI.
//
// Usage:
//
//	lti classify list.txt
//	lti describe +18168037763
//	lti build -co cocodes.txt -blocks blocks.txt -ocn ocn.csv -out data/linetype.bin
//	lti audit --verbose
//	lti version
package main

import (
	"github.com/tomba-io/phone-line-type-intelligence/internal/cli"
)

func main() {
	cli.Execute()
}
