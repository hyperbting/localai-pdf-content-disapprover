package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIChat(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hi","reasoning_content":"thinking"}}]}`))
	}))
	defer srv.Close()

	c, err := New("openai", Config{Endpoint: srv.URL + "/v1/", Model: "m", APIKey: "k", ReasoningEffort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out != "hi" {
		t.Errorf("content = %q", out)
	}
	if got["model"] != "m" || got["stream"] != false || got["reasoning_effort"] != "low" {
		t.Errorf("request body = %v", got)
	}
}

func TestOpenAIOmitsReasoningEffortByDefault(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c, _ := New("openai", Config{Endpoint: srv.URL, Model: "m"})
	if _, err := c.Chat(context.Background(), Request{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["reasoning_effort"]; ok {
		t.Errorf("reasoning_effort should be omitted: %v", got)
	}
}

func TestOllamaChat(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path = %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"message":{"role":"assistant","content":"yo"}}`))
	}))
	defer srv.Close()

	c, _ := New("ollama", Config{Endpoint: srv.URL, Model: "llama3.1"})
	out, err := c.Chat(context.Background(), Request{JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	if out != "yo" || got["format"] != "json" || got["stream"] != false {
		t.Errorf("out=%q body=%v", out, got)
	}
}
