package runtime

import (
	"context"
	"strings"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
)

func (d *Daemon) Posture() Posture {
	d.mu.Lock()
	p := d.posture
	d.mu.Unlock()

	p.Guardrails = d.guardrailSummary()
	if p.Frameworks == nil {
		p.Frameworks = []FrameworkPosture{}
	}

	return p
}

func (d *Daemon) refreshPosture(ctx context.Context) {
	fw, err := FetchPosture(ctx, d.probod.BaseURL(), d.secrets.APIKey, d.secrets.OrganizationID)
	if err != nil {
		d.logger.Printf("posture refresh failed: %v", err)
		return
	}

	d.mu.Lock()
	d.posture = Posture{UpdatedAt: time.Now().UTC(), Frameworks: fw}
	d.mu.Unlock()
	d.hub.Publish("posture", d.Posture())
}

func (d *Daemon) postureLoop(ctx context.Context) {
	refresh := func() { d.refreshPosture(ctx) }

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	renew := time.NewTicker(12 * time.Hour)
	defer renew.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-renew.C:
			d.renewAgentToken(ctx)
		case <-ticker.C:
			refresh()
		case <-d.postureKick:
			// Coalesce bursts of agent activity into one refresh.
			time.Sleep(2 * time.Second)
			refresh()
		}
	}
}

// evidenceLoop uploads completed days of agent evidence to Probo at startup
// and then hourly.
func (d *Daemon) evidenceLoop(ctx context.Context) {
	run := func() {
		n, err := d.SyncEvidence(ctx, false)
		if err != nil {
			d.logger.Printf("evidence sync failed: %v", err)
			return
		}

		if n > 0 {
			d.logger.Printf("uploaded %d evidence reports", n)
		}
	}

	run()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}

// renewAgentToken re-runs provisioning, which mints a new agent token once
// the current one is within a week of expiring, so a long-running bfd
// never starts failing MCP and posture calls.
func (d *Daemon) renewAgentToken(ctx context.Context) {
	if time.Until(d.secrets.APIKeyExpiresAt) > 7*24*time.Hour {
		return
	}

	save := func() error { return d.secrets.Save(d.layout.Secrets) }
	if _, err := Provision(ctx, d.probodURL(), d.layout.Mail, d.secrets, save); err != nil {
		d.logger.Printf("agent token renewal failed: %v", err)
		return
	}

	d.logger.Printf("agent token renewed")
}

func (d *Daemon) kickPosture() {
	select {
	case d.postureKick <- struct{}{}:
	default:
	}
}

func (d *Daemon) publishEntry(e guard.Entry) {
	typ := "evidence"
	if e.Decision == guard.ActionBlock || e.Decision == guard.ActionAsk {
		typ = "guardrail"
	}

	d.hub.Publish(typ, e)
	d.kickPosture()
}

// recordMCPCalls writes one ledger entry per MCP tool call an agent makes.
func (d *Daemon) recordMCPCalls(userAgent string, body []byte) {
	for _, c := range parseRPCCalls(body) {
		if c.Method != "tools/call" {
			continue
		}

		e := guard.Entry{
			Agent:    agentFromUserAgent(userAgent),
			Event:    "MCPToolCall",
			Tool:     "mcp:" + c.Params.Name,
			Target:   summarizeArgs(c.Params.Arguments),
			Decision: guard.ActionRecord,
			Controls: mcpControls(c.Params.Name),
		}

		if err := d.ledger.Append(&e); err != nil {
			d.logger.Printf("cannot record MCP call: %v", err)
			continue
		}

		d.publishEntry(e)
	}
}

func agentFromUserAgent(ua string) string {
	switch l := strings.ToLower(ua); {
	case strings.Contains(l, "claude"):
		return "claude-code"
	case strings.Contains(l, "cursor"):
		return "cursor"
	case strings.Contains(l, "codex"):
		return "codex"
	case ua == "":
		return "mcp-client"
	default:
		if i := strings.IndexAny(ua, "/ "); i > 0 {
			return ua[:i]
		}

		return ua
	}
}

func summarizeArgs(raw []byte) string {
	return guard.Truncate(strings.Join(strings.Fields(string(raw)), " "), 200)
}

// mcpControls tags compliance-record changes made by agents: writes to the
// GRC record itself fall under documented information and change control.
func mcpControls(tool string) []string {
	l := strings.ToLower(tool)
	if strings.HasPrefix(l, "list") || strings.HasPrefix(l, "get") || strings.HasPrefix(l, "search") {
		return nil
	}

	return []string{"ISO27001:7.5.3", "SOC2:CC2.1"}
}

func (d *Daemon) guardrailSummary() GuardrailSummary {
	entries, err := d.ledger.Recent(5000)
	if err != nil {
		return GuardrailSummary{}
	}

	var g GuardrailSummary
	cutoff := time.Now().Add(-24 * time.Hour)

	for _, e := range entries {
		if e.Time.Before(cutoff) {
			break
		}

		g.Last24h++
		switch e.Decision {
		case guard.ActionBlock:
			g.Blocked++
		case guard.ActionAsk:
			g.Asked++
		case guard.ActionRecord:
			g.Recorded++
		}
	}

	return g
}
