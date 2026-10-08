// Package ai defines the injectable local-AI abstraction. Any backend that can
// turn a list of chat messages into a text reply satisfies Client; backends are
// registered by name so the CLI can pick one with --provider.
package ai

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Message is a single chat turn.
type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// Request is what callers send to a Client.
type Request struct {
	Messages    []Message
	Temperature float64
	// JSON asks the backend to constrain output to JSON when it supports it.
	JSON bool
}

// Client is the injection point: implement it to plug in any model.
type Client interface {
	Chat(ctx context.Context, req Request) (string, error)
}

// ClientFunc adapts a plain function to Client (handy for tests and embedding).
type ClientFunc func(ctx context.Context, req Request) (string, error)

func (f ClientFunc) Chat(ctx context.Context, req Request) (string, error) { return f(ctx, req) }

// Config is the provider-agnostic settings bag filled from CLI flags / env.
type Config struct {
	Endpoint string
	Model    string
	APIKey   string
	Command  []string // used by the exec provider
	Timeout  time.Duration
	// ReasoningEffort is sent as reasoning_effort by the openai provider when
	// set (e.g. none|low|medium|high); empty leaves the server default.
	ReasoningEffort string
}

// Factory builds a Client from Config.
type Factory func(cfg Config) (Client, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register makes a provider available by name. Call it from init() in your own
// package to add a backend without touching the CLI.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = f
}

// New constructs the named provider.
func New(name string, cfg Config) (Client, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown AI provider %q (available: %v)", name, Providers())
	}
	return f(cfg)
}

// Providers lists registered provider names.
func Providers() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
