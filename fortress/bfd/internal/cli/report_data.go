package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"blackfortress.dev/fortress/bfd/internal/guard"
)

// entriesSince returns the ledger entries at or after since, oldest first.
func entriesSince(l guard.Ledger, since time.Time, session string) ([]guard.Entry, error) {
	var out []guard.Entry

	for day := since.UTC().Truncate(24 * time.Hour); !day.After(time.Now().UTC()); day = day.Add(24 * time.Hour) {
		entries, err := guard.ReadDay(l.Dir, day.Format("2006-01-02"))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}

		for _, e := range entries {
			if e.Time.Before(since) || (session != "" && e.SessionID != session) {
				continue
			}

			out = append(out, e)
		}
	}

	return out, nil
}

func summarize(entries []guard.Entry) (Activity, []guard.Entry) {
	a := Activity{Sessions: []string{}, Repos: []string{}, Controls: map[string]int{}}
	flagged := []guard.Entry{}
	sessions, repos := map[string]bool{}, map[string]bool{}

	for _, e := range entries {
		if e.SessionID != "" {
			sessions[e.SessionID] = true
		}

		if e.Repo != "" {
			repos[e.Repo] = true
		}

		if e.Event != "PreToolUse" || e.Tool == "" {
			continue
		}

		a.Actions++

		switch e.Decision {
		case guard.ActionBlock:
			a.Blocked++
			flagged = append(flagged, e)
		case guard.ActionAsk:
			a.Asked++
			flagged = append(flagged, e)
		case guard.ActionRecord:
			a.Recorded++
		}

		for _, c := range e.Controls {
			a.Controls[c]++
		}
	}

	a.Sessions = sortedKeys(sessions)
	a.Repos = sortedKeys(repos)

	return a, flagged
}

// judge sets the verdict: fail when evidence cannot be trusted or a check
// found a problem; attention when something needs a human look.
func judge(r *Report) (string, []string) {
	var fail, attention []string

	if !r.Ledger.Valid {
		fail = append(fail, "evidence ledger failed verification: "+r.Ledger.Error)
	}

	if r.Checks != nil {
		for _, name := range sortedKeys(r.Checks.Providers) {
			run := r.Checks.Providers[name]
			if run.Error != "" {
				attention = append(attention, fmt.Sprintf("%s checks could not run: %s", run.ProviderName, run.Error))
				continue
			}

			for _, c := range run.Checks {
				switch c.Verdict() {
				case "fail":
					fail = append(fail, fmt.Sprintf("%s: %s (%d findings)", run.ProviderName, c.Name, len(c.Findings)))
				case "error":
					attention = append(attention, fmt.Sprintf("%s: %s errored", run.ProviderName, c.Name))
				}
			}
		}
	}

	if r.Runtime == nil {
		attention = append(attention, "the runtime is not running: posture and automated checks are missing")
	} else if r.Runtime.State != "running" {
		attention = append(attention, "the runtime is "+r.Runtime.State)
	}

	if r.Activity.Blocked > 0 {
		attention = append(attention, fmt.Sprintf("guardrails blocked %d agent actions", r.Activity.Blocked))
	}

	if r.Activity.Asked > 0 {
		attention = append(attention, fmt.Sprintf("%d agent actions needed human approval", r.Activity.Asked))
	}

	switch {
	case len(fail) > 0:
		return VerdictFail, append(fail, attention...)
	case len(attention) > 0:
		return VerdictAttention, attention
	default:
		return VerdictPass, []string{}
	}
}

type apiClient struct{ token string }

// getJSON fetches one control API resource, or nil when bfd is not
// answering or the request fails.
func getJSON[T any](c *apiClient, path string) *T {
	req, err := http.NewRequest(http.MethodGet, controlURL()+path, nil)
	if err != nil {
		return nil
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil
	}

	return &v
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

func oneLine(s string, n int) string {
	return guard.Truncate(strings.Join(strings.Fields(s), " "), n)
}
