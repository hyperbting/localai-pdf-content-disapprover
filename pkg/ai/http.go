package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func httpClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 5 * time.Minute // local models can be slow
	}
	return &http.Client{Timeout: timeout}
}

func postJSON(ctx context.Context, c *http.Client, url, apiKey string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("POST %s: %s: %s", url, resp.Status, bytes.TrimSpace(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response from %s: %w", url, err)
	}
	return nil
}
