// Command storage-logs emits the canonical JSON for the Storage & Log
// Pipeline Grafana dashboard (platform phase-95). build.sh writes it to
// k8s/platform/observability/files/storage-logs.json.
package main

import (
	"fmt"
	"os"

	"github.com/Chalupa-Tech/chalupa-infra/scripts/dashboards/internal/storagelogs"
)

func main() {
	out, err := storagelogs.Render()
	if err != nil {
		fmt.Fprintf(os.Stderr, "storage-logs: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintf(os.Stderr, "storage-logs: write stdout: %v\n", err)
		os.Exit(1)
	}
}
