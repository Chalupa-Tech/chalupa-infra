// Command paper-trader emits the canonical JSON for the paper-trading
// Grafana dashboard. CI invokes it via `go run ./cmd/paper-trader >
// k8s/apps/schwab/go-paper-trader/files/paper-trading.json`.
package main

import (
	"fmt"
	"os"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/papertrading"
)

func main() {
	out, err := papertrading.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "paper-trader: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "paper-trader: write stdout: %v\n", err)
		os.Exit(1)
	}
}
