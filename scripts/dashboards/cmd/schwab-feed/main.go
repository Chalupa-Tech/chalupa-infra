// Command schwab-feed emits the canonical JSON for the schwab market
// feed Grafana dashboard. CI invokes it via
// `go run ./cmd/schwab-feed > k8s/apps/schwab/go-schwab-feed/files/schwab-feed.json`.
package main

import (
	"fmt"
	"os"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/schwabfeed"
)

func main() {
	out, err := schwabfeed.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "schwab-feed: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "schwab-feed: write stdout: %v\n", err)
		os.Exit(1)
	}
}
