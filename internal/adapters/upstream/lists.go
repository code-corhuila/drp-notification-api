package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/code-corhuila/drp-notification-api/internal/app"
)

type HTTPCatalog struct {
	Name    string
	BaseURL string
	Path    string
	Client  *http.Client
}

func (c HTTPCatalog) Service() string { return c.Name }

func (c HTTPCatalog) FetchAll(ctx context.Context, bearer string) ([]map[string]any, error) {
	if c.BaseURL == "" {
		return nil, app.UpstreamError{Service: c.Name, Status: 503}
	}
	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	base := strings.TrimRight(c.BaseURL, "/")
	out := make([]map[string]any, 0)
	page := 1
	total := 1
	for len(out) < total {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s%s?page=%d&limit=100", base, c.Path, page), nil)
		if err != nil {
			return nil, app.UpstreamError{Service: c.Name, Status: 503}
		}
		req.Header.Set("Authorization", bearer)
		res, err := client.Do(req)
		if err != nil {
			return nil, app.UpstreamError{Service: c.Name, Status: 503}
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			_ = res.Body.Close()
			return nil, app.UpstreamError{Service: c.Name, Status: res.StatusCode}
		}
		var env struct {
			Data []map[string]any `json:"data"`
			Meta struct {
				Total int `json:"total"`
			} `json:"meta"`
		}
		decErr := json.NewDecoder(res.Body).Decode(&env)
		res.Body.Close()
		if decErr != nil {
			return nil, app.UpstreamError{Service: c.Name, Status: 503}
		}
		total = env.Meta.Total
		if total == 0 {
			return []map[string]any{}, nil
		}
		out = append(out, env.Data...)
		if len(env.Data) == 0 {
			break
		}
		page++
	}
	return out, nil
}
