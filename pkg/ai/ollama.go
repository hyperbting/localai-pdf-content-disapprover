package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

func init() { Register("ollama", NewOllama) }

// Ollama talks to Ollama's native /api/chat endpoint.
type Ollama struct {
	endpoint string
	model    string
	http     *http.Client
}

func NewOllama(cfg Config) (Client, error) {
	if cfg.Model == "" {
		return nil, errors.New("ollama: --model is required (e.g. llama3.1)")
	}
	ep := cfg.Endpoint
	if ep == "" {
		ep = "http://localhost:11434"
	}
	return &Ollama{endpoint: strings.TrimRight(ep, "/"), model: cfg.Model, http: httpClient(cfg.Timeout)}, nil
}

func (o *Ollama) Chat(ctx context.Context, req Request) (string, error) {
	in := map[string]any{
		"model":    o.model,
		"messages": req.Messages,
		"stream":   false,
		"options":  map[string]any{"temperature": req.Temperature},
	}
	if req.JSON {
		in["format"] = "json"
	}
	var out struct {
		Message Message `json:"message"`
	}
	if err := postJSON(ctx, o.http, o.endpoint+"/api/chat", "", in, &out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
}
