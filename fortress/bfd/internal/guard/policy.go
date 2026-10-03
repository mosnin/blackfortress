// Package guard evaluates coding-agent actions against compliance rules and
// records every evaluation in an append-only evidence ledger.
package guard

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
)

// Action is what a rule asks the agent runtime to do.
type Action string

const (
	ActionBlock  Action = "block"
	ActionAsk    Action = "ask"
	ActionRecord Action = "record"
)

func (a Action) rank() int {
	switch a {
	case ActionBlock:
		return 3
	case ActionAsk:
		return 2
	case ActionRecord:
		return 1
	default:
		return 0
	}
}

// Rule maps a class of agent action to the controls it affects.
type Rule struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Action      Action   `json:"action"`
	Severity    string   `json:"severity,omitempty"`
	Controls    []string `json:"controls"`
	Disabled    bool     `json:"disabled,omitempty"`

	// Matchers. Empty matchers match anything; all non-empty matchers must
	// match for the rule to fire.
	Tools   string `json:"tools,omitempty"`
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
	Content string `json:"content,omitempty"`

	tools, path, command, content *regexp.Regexp
}

type Policy struct {
	Version int    `json:"version"`
	Rules   []Rule `json:"rules"`
}

//go:embed default_policy.json
var defaultPolicyJSON []byte

// DefaultPolicy returns the built-in rule set.
func DefaultPolicy() (*Policy, error) {
	return parsePolicy(defaultPolicyJSON)
}

// LoadPolicy merges the user's policy file over the defaults: a user rule
// with the same id replaces the default, new ids are appended, and
// "disabled": true turns a default rule off.
func LoadPolicy(path string) (*Policy, error) {
	p, err := DefaultPolicy()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}

	if err != nil {
		return nil, err
	}

	user, err := parsePolicy(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	index := map[string]int{}
	for i, r := range p.Rules {
		index[r.ID] = i
	}

	for _, r := range user.Rules {
		if i, ok := index[r.ID]; ok {
			p.Rules[i] = r
		} else {
			p.Rules = append(p.Rules, r)
		}
	}

	return p, nil
}

func parsePolicy(data []byte) (*Policy, error) {
	var p Policy
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid policy: %w", err)
	}

	for i := range p.Rules {
		if err := p.Rules[i].compile(); err != nil {
			return nil, err
		}
	}

	return &p, nil
}

func (r *Rule) compile() error {
	if r.ID == "" {
		return errors.New("rule without id")
	}

	switch r.Action {
	case ActionBlock, ActionAsk, ActionRecord:
	default:
		return fmt.Errorf("rule %s: unknown action %q", r.ID, r.Action)
	}

	var err error
	for _, m := range []struct {
		src string
		dst **regexp.Regexp
	}{
		{r.Tools, &r.tools},
		{r.Path, &r.path},
		{r.Command, &r.command},
		{r.Content, &r.content},
	} {
		if m.src == "" {
			continue
		}

		if *m.dst, err = regexp.Compile(m.src); err != nil {
			return fmt.Errorf("rule %s: %w", r.ID, err)
		}
	}

	return nil
}
