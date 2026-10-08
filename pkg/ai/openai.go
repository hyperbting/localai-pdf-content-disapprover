package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

func init() { Register("openai", NewOpenAI) }

// OpenAI targets any OpenAI-compatible server: LocalAI, LM Studio,
// llama.cpp server, vLLM, Jan, Strata, etc. Endpoint is the base URL including /v1.
type OpenAI struct {
	endpoint        string
	model           string
	apiKey          string
	reasoningEffort string
	http            *http.Client
}

func NewOpenAI(cfg Config) (Client, error) {
	if cfg.Model == "" {
		return nil, errors.New("openai: --model is required")
	}
	ep := cfg.Endpoint
	if ep == "" {
		ep = "http://localhost:8080/v1" // LocalAI default
	}
	return &OpenAI{
		endpoint:        strings.TrimRight(ep, "/"),
		model:           cfg.Model,
		apiKey:          cfg.APIKey,
		reasoningEffort: cfg.ReasoningEffort,
		http:            httpClient(cfg.Timeout),
	}, nil
}

func (o *OpenAI) Chat(ctx context.Context, req Request) (string, error) {
	// response_format is intentionally omitted: many local servers reject it.
	// The reviewer prompt asks for JSON and the parser tolerates surrounding text.
	in := map[string]any{
		"model":       o.model,
		"messages":    req.Messages,
		"temperature": req.Temperature,
		"stream":      false,
	}
	if o.reasoningEffort != "" {
		in["reasoning_effort"] = o.reasoningEffort
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := postJSON(ctx, o.http, o.endpoint+"/chat/completions", o.apiKey, in, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("openai: response contained no choices")
	}
	return out.Choices[0].Message.Content, nil
}
