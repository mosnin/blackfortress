package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
	"blackfortress.dev/fortress/bfd/internal/paths"
)

// Status prints bfd's /v1/status, or reports that it is not running.
func Status(stdout io.Writer) int {
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get(controlURL() + "/v1/status")
	if err != nil {
		fmt.Fprintln(stdout, "Black Fortress is not running.")
		return 1
	}
	defer resp.Body.Close()

	var v any
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)

	return 0
}

// Ledger prints recent ledger entries, or with "verify" checks the chain.
func Ledger(args []string, stdout io.Writer) int {
	home, err := paths.Home()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	l := guard.Ledger{Dir: paths.NewLayout(home).Ledger}

	if len(args) > 0 && args[0] == "verify" {
		n, err := l.Verify()
		if err != nil {
			fmt.Fprintf(stdout, "✗ ledger tampered or corrupt after %d valid entries: %v\n", n, err)
			return 1
		}

		fmt.Fprintf(stdout, "✓ ledger intact: %d entries verified\n", n)

		return 0
	}

	entries, err := l.Recent(20)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		fmt.Fprintf(stdout, "%s  %-12s %-18s %-8s %s %v\n",
			e.Time.Local().Format("2006-01-02 15:04:05"), e.Agent, e.Event, e.Decision, e.Target, e.Controls)
	}

	return 0
}
