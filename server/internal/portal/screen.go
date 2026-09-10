package portal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/knowoff/knowoff/server/internal/config"
	"io"
	"net/http"
	"strings"
	"time"
)

// TextScreener is a replaceable automated check used before human approval.
type TextScreener interface {
	ScreenText(context.Context, string) error
}

var ErrScreeningUnavailable = errors.New("automated content screening is unavailable; approval is paused")
var ErrContentFlagged = errors.New("automated content screening flagged this text; approval is paused")

func (m *Manager) screenText(ctx context.Context, text string) error {
	if m.screener == nil {
		return ErrScreeningUnavailable
	}
	return m.screener.ScreenText(ctx, text)
}

const maxScreenTextBytes = 64 << 10

// NewTextScreener configures server-only moderation. No request is made here.
// Missing configuration deliberately returns nil so approvals fail closed.
func NewTextScreener(cfg config.ContentScreeningConfig) TextScreener {
	if cfg.Provider != "openai" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil
	}
	timeout := cfg.TimeoutS
	if timeout == 0 {
		timeout = 10
	}
	if timeout < 1 || timeout > 30 {
		return nil
	}
	return &openAITextScreener{
		endpoint: "https://api.openai.com/v1/moderations", apiKey: cfg.APIKey, model: cfg.Model,
		client: &http.Client{Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

type openAITextScreener struct {
	endpoint, apiKey, model string
	client                  *http.Client
}

func (s *openAITextScreener) ScreenText(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" || len(text) > maxScreenTextBytes {
		return ErrScreeningUnavailable
	}
	body, err := json.Marshal(struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{s.model, text})
	if err != nil {
		return ErrScreeningUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrScreeningUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return ErrScreeningUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ErrScreeningUnavailable
	}
	const maxResponseBytes = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return ErrScreeningUnavailable
	}
	var result struct {
		Results []struct {
			Flagged *bool `json:"flagged"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Results) == 0 {
		return ErrScreeningUnavailable
	}
	for _, decision := range result.Results {
		if decision.Flagged == nil {
			return ErrScreeningUnavailable
		}
		if *decision.Flagged {
			return ErrContentFlagged
		}
	}
	return nil
}
