package avatar

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
)

// The provider receives only the normalized pixels that would be activated.
// Endpoint and redirects are fixed so uploaded data cannot select a destination.
func newImageScreen(c config.ContentScreeningConfig) func(context.Context, []byte) error {
	if !config.AvatarScreeningEnabled(c) {
		return nil
	}
	timeout := c.TimeoutS
	if timeout == 0 {
		timeout = 10
	}
	client := &http.Client{Timeout: time.Duration(timeout) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return imageScreen(client, c.Model, c.APIKey)
}
func imageScreen(client *http.Client, model, key string) func(context.Context, []byte) error {
	return func(ctx context.Context, image []byte) error {
		if len(image) == 0 || len(image) > maxAvatarBytes {
			return ErrInvalid
		}
		request := struct {
			Model string `json:"model"`
			Input []any  `json:"input"`
		}{Model: model, Input: []any{map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/webp;base64," + base64.StdEncoding.EncodeToString(image)}}}}
		body, err := json.Marshal(request)
		if err != nil {
			return ErrUnavailable
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/moderations", bytes.NewReader(body))
		if err != nil {
			return ErrUnavailable
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return ErrUnavailable
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return ErrUnavailable
		}
		const maxResponse = 1 << 20
		data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
		if err != nil || len(data) > maxResponse {
			return ErrUnavailable
		}
		var reply struct {
			Results []struct {
				Flagged *bool `json:"flagged"`
			} `json:"results"`
		}
		if json.Unmarshal(data, &reply) != nil || len(reply.Results) != 1 || reply.Results[0].Flagged == nil {
			return ErrUnavailable
		}
		if *reply.Results[0].Flagged {
			return ErrFlagged
		}
		return nil
	}
}
