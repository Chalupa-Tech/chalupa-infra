// Command schwab-rollouts emits the canonical JSON for the Schwab
// Argo Rollouts Grafana dashboard. CI invokes it via
// `go run ./cmd/schwab-rollouts > k8s/apps/schwab/go-schwab-feed/files/schwab-rollouts.json`.
//
// Output path lives under go-schwab-feed/files/ for historical
// reasons (the chart that owns the dashboard ConfigMap mount); the
// dashboard's identity is the binary + internal package name.
package main

import (
	"fmt"
	"os"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/schwabrollouts"
)

func main() {
	out, err := schwabrollouts.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "schwab-rollouts: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "schwab-rollouts: write stdout: %v\n", err)
		os.Exit(1)
	}
}
