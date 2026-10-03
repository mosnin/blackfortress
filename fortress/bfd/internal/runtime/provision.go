package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os/user"
	"strings"
	"time"

	"blackfortress.dev/fortress/bfd/internal/secrets"
)

// AgentScopes are the OAuth scopes granted to the token that agents use
// through bfd's MCP endpoint. They mirror what Probo's own CLI requests.
var AgentScopes = []string{
	"v1:access-review", "v1:agent", "v1:ai-system", "v1:asset", "v1:audit",
	"v1:business-function", "v1:compliance-page", "v1:connector", "v1:control",
	"v1:datum", "v1:document", "v1:iam", "v1:itam", "v1:org", "v1:privacy",
	"v1:resource-alias", "v1:risk", "v1:task", "v1:third-party", "v1:webhook",
}

const agentTokenLifetime = 90 * 24 * time.Hour

// Session is a cookie-authenticated GraphQL client for probod.
type Session struct {
	baseURL string
	client  *http.Client
}

func NewSession(baseURL string) *Session {
	jar, _ := cookiejar.New(nil)

	return &Session{
		baseURL: baseURL,
		client:  &http.Client{Jar: jar, Timeout: 30 * time.Second},
	}
}

// Cookies returns the cookies the session holds for probod.
func (s *Session) Cookies() []*http.Cookie {
	req, _ := http.NewRequest(http.MethodGet, s.baseURL, nil)
	return s.client.Jar.Cookies(req.URL)
}

type gqlError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

type GraphQLError struct{ Errors []gqlError }

func (e *GraphQLError) Error() string {
	msgs := make([]string, 0, len(e.Errors))
	for _, x := range e.Errors {
		msgs = append(msgs, x.Message)
	}

	return "graphql: " + strings.Join(msgs, "; ")
}

func (e *GraphQLError) HasCode(code string) bool {
	for _, x := range e.Errors {
		if c, _ := x.Extensions["code"].(string); c == code {
			return true
		}
	}

	return false
}

// Do runs a GraphQL operation against api ("connect" or "console").
func (s *Session) Do(ctx context.Context, api, query string, vars map[string]any, out any) error {
	return doGraphQL(ctx, s.client, s.baseURL+"/api/"+api+"/v1/graphql", "", query, vars, out)
}

func doGraphQL(ctx context.Context, client *http.Client, endpoint, bearer, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}

	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("unexpected response (%d): %s", resp.StatusCode, truncate(raw, 300))
	}

	if len(envelope.Errors) > 0 {
		return &GraphQLError{Errors: envelope.Errors}
	}

	if out != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, out)
	}

	return nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}

	return string(b)
}

func (s *Session) SignIn(ctx context.Context, email, password string) error {
	const q = `mutation($input: SignInInput!) { signIn(input: $input) { identity { id } } }`

	return s.Do(ctx, "connect", q, map[string]any{"input": map[string]any{"email": email, "password": password}}, nil)
}

// AssumeOrganization upgrades the session to act inside orgID, which the
// console API requires.
func (s *Session) AssumeOrganization(ctx context.Context, orgID string) error {
	const q = `mutation($input: AssumeOrganizationSessionInput!) {
  assumeOrganizationSession(input: $input) { result { __typename } }
}`

	var out struct {
		AssumeOrganizationSession struct {
			Result struct {
				Typename string `json:"__typename"`
			} `json:"result"`
		} `json:"assumeOrganizationSession"`
	}

	err := s.Do(ctx, "connect", q, map[string]any{
		"input": map[string]any{"organizationId": orgID, "continue": s.baseURL + "/"},
	}, &out)
	if err != nil {
		return err
	}

	if t := out.AssumeOrganizationSession.Result.Typename; t != "OrganizationSessionCreated" {
		return fmt.Errorf("cannot assume organization session: %s", t)
	}

	return nil
}

func (s *Session) signUp(ctx context.Context, email, password, fullName string) error {
	const q = `mutation($input: SignUpInput!) { signUp(input: $input) { identity { id } } }`

	return s.Do(ctx, "connect", q, map[string]any{
		"input": map[string]any{"email": email, "password": password, "fullName": fullName},
	}, nil)
}

func (s *Session) createOrganization(ctx context.Context, name string) (string, error) {
	const q = `mutation($input: CreateOrganizationInput!) { createOrganization(input: $input) { organization { id } } }`

	var out struct {
		CreateOrganization struct {
			Organization struct {
				ID string `json:"id"`
			} `json:"organization"`
		} `json:"createOrganization"`
	}
	if err := s.Do(ctx, "connect", q, map[string]any{"input": map[string]any{"name": name}}, &out); err != nil {
		return "", err
	}

	return out.CreateOrganization.Organization.ID, nil
}

func (s *Session) createAccessToken(ctx context.Context, name string, expiresAt time.Time) (string, error) {
	const q = `mutation($input: CreateOAuth2AccessTokenInput!) { createOAuth2AccessToken(input: $input) { token } }`

	var out struct {
		CreateOAuth2AccessToken struct {
			Token string `json:"token"`
		} `json:"createOAuth2AccessToken"`
	}

	err := s.Do(ctx, "connect", q, map[string]any{
		"input": map[string]any{
			"name":      name,
			"expiresAt": expiresAt.UTC().Format(time.RFC3339),
			"scopes":    AgentScopes,
		},
	}, &out)
	if err != nil {
		return "", err
	}

	if out.CreateOAuth2AccessToken.Token == "" {
		return "", errors.New("empty access token")
	}

	return out.CreateOAuth2AccessToken.Token, nil
}

// Provision makes sure the local identity, organization and agent token
// exist. It is idempotent: each step is skipped once its result is recorded
// in the secrets file.
func Provision(ctx context.Context, baseURL, mailDir string, s *secrets.Secrets, save func() error) (*Session, error) {
	sess := NewSession(baseURL)

	err := sess.SignIn(ctx, s.UserEmail, s.UserPassword)
	if err != nil && !isUnverified(err) {
		since := time.Now().Add(-time.Second)
		if err := sess.signUp(ctx, s.UserEmail, s.UserPassword, localFullName()); err != nil {
			return nil, fmt.Errorf("cannot create local user: %w", err)
		}

		err = sess.SignIn(ctx, s.UserEmail, s.UserPassword)
		if isUnverified(err) {
			err = sess.verify(ctx, mailDir, since, s)
		}
	} else if isUnverified(err) {
		// A previous run created the user but stopped before verifying.
		since := time.Now().Add(-time.Second)
		_ = sess.Do(ctx, "connect", `mutation($input: ResendVerificationEmailInput!) { resendVerificationEmail(input: $input) { success } }`,
			map[string]any{"input": map[string]any{"email": s.UserEmail}}, nil)
		err = sess.verify(ctx, mailDir, since, s)
	}

	if err != nil {
		return nil, fmt.Errorf("cannot sign in local user: %w", err)
	}

	if s.OrganizationID == "" {
		id, err := sess.createOrganization(ctx, localOrgName())
		if err != nil {
			return nil, fmt.Errorf("cannot create organization: %w", err)
		}

		s.OrganizationID = id
		if err := save(); err != nil {
			return nil, err
		}
	}

	if s.APIKey == "" || time.Until(s.APIKeyExpiresAt) < 7*24*time.Hour {
		expires := time.Now().Add(agentTokenLifetime)

		token, err := sess.createAccessToken(ctx, "Black Fortress agents "+time.Now().Format("2006-01-02"), expires)
		if err != nil {
			return nil, fmt.Errorf("cannot create agent token: %w", err)
		}

		s.APIKey = token
		s.APIKeyExpiresAt = expires
		if err := save(); err != nil {
			return nil, err
		}
	}

	return sess, nil
}

func isUnverified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not verified")
}

func (s *Session) verify(ctx context.Context, mailDir string, since time.Time, sec *secrets.Secrets) error {
	token, err := waitForVerificationToken(ctx, mailDir, since, 3*time.Minute)
	if err != nil {
		return err
	}

	const q = `mutation($input: VerifyEmailInput!) { verifyEmail(input: $input) { success } }`
	if err := s.Do(ctx, "connect", q, map[string]any{"input": map[string]any{"token": token}}, nil); err != nil {
		return fmt.Errorf("cannot verify email: %w", err)
	}

	return s.SignIn(ctx, sec.UserEmail, sec.UserPassword)
}

func localFullName() string {
	if u, err := user.Current(); err == nil {
		if u.Name != "" {
			return u.Name
		}

		if u.Username != "" {
			return u.Username
		}
	}

	return "Black Fortress Owner"
}

func localOrgName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username + "'s workspace"
	}

	return "My workspace"
}
