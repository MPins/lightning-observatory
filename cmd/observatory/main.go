// Command observatory reads the public Lightning graph and exports metrics
// about it for Prometheus.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/MPins/lightning-observatory/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("observatory", version.Get())
		return
	}

	fmt.Fprintln(os.Stderr, "observatory: not implemented yet")
	os.Exit(1)
}
