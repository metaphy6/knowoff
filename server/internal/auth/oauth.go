package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
)

// OAuthFlowState is a short-lived PKCE + state tuple stored server-side.
type OAuthFlowState struct {
	Provider     string
	State        string
	CodeVerifier string
	AccountID    string
	CreatedAt    time.Time
}

// OAuthFlowStore holds pending OAuth states. In production this is backed by
// Redis; the interface allows an in-memory implementation for tests.
type OAuthFlowStore interface {
	Save(state *OAuthFlowState, ttl time.Duration) error
	Load(state string) (*OAuthFlowState, bool)
	Delete(state string)
}

// memoryOAuthStore is a simple in-memory OAuth state store.
type memoryOAuthStore struct {
	states map[string]*OAuthFlowState
}

func newMemoryOAuthStore() *memoryOAuthStore {
	return &memoryOAuthStore{states: make(map[string]*OAuthFlowState)}
}

func (s *memoryOAuthStore) Save(state *OAuthFlowState, ttl time.Duration) error {
	s.states[state.State] = state
	return nil
}

func (s *memoryOAuthStore) Load(state string) (*OAuthFlowState, bool) {
	st, ok := s.states[state]
	return st, ok
}

func (s *memoryOAuthStore) Delete(state string) {
	delete(s.states, state)
}

// StartOAuth returns the authorization URL and stores the PKCE state.
func (m *Manager) StartOAuth(ctx context.Context, provider, accountID string) (string, error) {
	if accountID != "" {
		purpose, err := m.accountPurpose(ctx, accountID)
		if err != nil || purpose != "player" {
			return "", fmt.Errorf("player account required")
		}
	}
	cfg, ok := m.oauthCfgs[provider]
	if !ok {
		return "", fmt.Errorf("unsupported provider %q", provider)
	}
	state, err := randomState()
	if err != nil {
		return "", err
	}
	verifier, err := randomCodeVerifier()
	if err != nil {
		return "", err
	}
	challenge := codeChallenge(verifier)
	m.oauthStore.Save(&OAuthFlowState{
		Provider:     provider,
		State:        state,
		CodeVerifier: verifier,
		AccountID:    accountID,
		CreatedAt:    time.Now().UTC(),
	}, 10*time.Minute)
	url := cfg.AuthCodeURL(state, oauth2.SetAuthURLParam("code_challenge", challenge), oauth2.SetAuthURLParam("code_challenge_method", "S256"))
	return url, nil
}

// CompleteOAuth exchanges the code, fetches userinfo, and links the provider.
func (m *Manager) CompleteOAuth(ctx context.Context, provider, state, code string) (*TokenPair, error) {
	st, ok := m.oauthStore.Load(state)
	if !ok {
		return nil, fmt.Errorf("invalid or expired state")
	}
	if st.Provider != provider {
		return nil, fmt.Errorf("provider mismatch")
	}
	if time.Since(st.CreatedAt) > 10*time.Minute {
		m.oauthStore.Delete(state)
		return nil, fmt.Errorf("state expired")
	}
	m.oauthStore.Delete(state)

	cfg, ok := m.oauthCfgs[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider")
	}
	token, err := cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", st.CodeVerifier))
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	subject, email, err := fetchUserinfo(ctx, provider, cfg, token)
	if err != nil {
		return nil, fmt.Errorf("userinfo: %w", err)
	}
	if subject == "" {
		return nil, fmt.Errorf("provider did not return subject")
	}

	accountID := st.AccountID
	existing, err := m.FindOAuthAccount(ctx, provider, subject)
	if err == nil && existing != "" && existing != accountID {
		return nil, fmt.Errorf("provider subject already linked to another account")
	}
	if err == nil && existing != "" {
		accountID = existing
	}
	if err := m.LinkOAuth(ctx, accountID, provider, subject, email); err != nil {
		return nil, err
	}
	return m.issueTokens(ctx, accountID, "")
}

func fetchUserinfo(ctx context.Context, provider string, cfg *oauth2.Config, token *oauth2.Token) (string, string, error) {
	client := cfg.Client(ctx, token)
	switch provider {
	case "google":
		return fetchGoogleUserinfo(ctx, client)
	case "facebook":
		return fetchFacebookUserinfo(ctx, client)
	default:
		return "", "", fmt.Errorf("unsupported provider")
	}
}

func fetchGoogleUserinfo(ctx context.Context, client *http.Client) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	var data struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", "", err
	}
	return data.Subject, data.Email, nil
}

func fetchFacebookUserinfo(ctx context.Context, client *http.Client) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.facebook.com/me?fields=id,email", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	var data struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", "", err
	}
	return data.ID, data.Email, nil
}

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
