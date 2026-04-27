// Package banner provides the PAPER TRADING safety banner.
//
// The literal "PAPER TRADING" substring is gated by
// .github/workflows/paper-dashboard-banner-lint.yml. The full markdown
// content is the user-facing safety guarantee from the paper-trading
// architecture brief — preserve byte-for-byte unless the brief changes.
package banner

import (
	"strings"

	"github.com/grafana/grafana-foundation-sdk/go/dashboard"
	"github.com/grafana/grafana-foundation-sdk/go/text"
)

const Literal = "PAPER TRADING"

const Content = "## ⚠️ PAPER TRADING — SIMULATED DATA\n" +
	"Fills are simulated: BUYs lift the ask, SELLs hit the bid, with an " +
	"optional ExtraSlippageBps cushion (0 bps in production today) and a " +
	"100ms constant submit-to-fill latency. Real-world fills will differ " +
	"materially — Schwab's actual fills depend on their routing, venue " +
	"selection, and queue position. **P&L on this dashboard is not real " +
	"money.**"

func init() {
	if !strings.Contains(Content, Literal) {
		// Fail loud at process start if anyone edits Content and breaks
		// the banner-lint contract.
		panic("banner.Content lost the literal 'PAPER TRADING' substring; " +
			"paper-dashboard-banner-lint.yml will fail any PR with this state")
	}
}

// Panel returns the banner text panel at the top of every paper-* dashboard.
func Panel(panelID, y int) *text.PanelBuilder {
	return text.NewPanelBuilder().
		Id(uint32(panelID)).
		Title("").
		GridPos(dashboard.GridPos{H: 3, W: 24, X: 0, Y: uint32(y)}).
		Mode(text.TextModeMarkdown).
		Content(Content)
}
