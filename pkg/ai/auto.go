package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func init() { Register("auto", NewAuto) }

// Detected is the client chosen by the auto provider, with what was found.
type Detected struct {
	Client
	Provider string
	Endpoint string
	Model    string
}

type candidate struct{ provider, endpoint string }

// DefaultCandidates are probed in order when no endpoint is given.
var DefaultCandidates = []candidate{
	{"ollama", "http://localhost:11434"},
	{"openai", "http://127.0.0.1:8080/v1"}, // LocalAI, Strata, llama.cpp server
	{"openai", "http://localhost:1234/v1"}, // LM Studio
	{"openai", "http://localhost:8000/v1"}, // vLLM
}

// probeTimeout bounds each detection request; local servers answer fast.
const probeTimeout = 2 * time.Second

// NewAuto detects which kind of server is reachable and returns that provider.
// An --exec command always wins. Otherwise the configured endpoint (or each
// default local port) is probed: Ollama via /api/tags, OpenAI-compatible via
// /models. If no model is given, the first model the server lists is used.
func NewAuto(cfg Config) (Client, error) {
	if len(cfg.Command) > 0 {
		c, err := NewExec(cfg)
		if err != nil {
			return nil, err
		}
		return &Detected{Client: c, Provider: "exec", Endpoint: strings.Join(cfg.Command, " ")}, nil
	}
	return Detect(context.Background(), cfg)
}

// Detect probes for a server and builds the matching client.
func Detect(ctx context.Context, cfg Config) (*Detected, error) {
	hc := &http.Client{Timeout: probeTimeout}
	cands := DefaultCandidates
	if ep := strings.TrimRight(cfg.Endpoint, "/"); ep != "" {
		if strings.HasSuffix(ep, "/v1") {
			cands = []candidate{{"openai", ep}, {"ollama", strings.TrimSuffix(ep, "/v1")}}
		} else {
			cands = []candidate{{"ollama", ep}, {"openai", ep + "/v1"}, {"openai", ep}}
		}
	}

	var tried []string
	for _, c := range cands {
		var models []string
		var ok bool
		switch c.provider {
		case "ollama":
			models, ok = probeOllama(ctx, hc, c.endpoint)
		case "openai":
			models, ok = probeOpenAI(ctx, hc, c.endpoint, cfg.APIKey)
		}
		tried = append(tried, c.provider+" "+c.endpoint)
		if !ok {
			continue
		}

		sub := cfg
		sub.Endpoint = c.endpoint
		if sub.Model == "" {
			if len(models) == 0 {
				return nil, fmt.Errorf("auto: found %s at %s but it lists no models; pass --model", c.provider, c.endpoint)
			}
			sub.Model = models[0]
		}
		client, err := New(c.provider, sub)
		if err != nil {
			return nil, err
		}
		return &Detected{Client: client, Provider: c.provider, Endpoint: c.endpoint, Model: sub.Model}, nil
	}
	return nil, errors.New("auto: no local AI server found (tried " + strings.Join(tried, ", ") + "); start one or set --provider and --endpoint")
}

// probeOllama requires a JSON body with a "models" array, so a web app that
// answers every path with HTML is not mistaken for Ollama.
func probeOllama(ctx context.Context, hc *http.Client, base string) ([]string, bool) {
	var out struct {
		Models *[]struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if !getJSON(ctx, hc, base+"/api/tags", "", &out) || out.Models == nil {
		return nil, false
	}
	names := make([]string, 0, len(*out.Models))
	for _, m := range *out.Models {
		names = append(names, m.Name)
	}
	return names, true
}

func probeOpenAI(ctx context.Context, hc *http.Client, base, apiKey string) ([]string, bool) {
	var out struct {
		Data *[]struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if !getJSON(ctx, hc, base+"/models", apiKey, &out) || out.Data == nil {
		return nil, false
	}
	ids := make([]string, 0, len(*out.Data))
	for _, m := range *out.Data {
		ids = append(ids, m.ID)
	}
	return ids, true
}

func getJSON(ctx context.Context, hc *http.Client, url, apiKey string, out any) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(resp.Body).Decode(out) == nil
}
