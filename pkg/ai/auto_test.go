package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(routes map[string]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
}

func TestDetectOllama(t *testing.T) {
	srv := serve(map[string]string{"/api/tags": `{"models":[{"name":"llama3.1:8b"},{"name":"qwen3"}]}`})
	defer srv.Close()

	d, err := Detect(context.Background(), Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if d.Provider != "ollama" || d.Endpoint != srv.URL || d.Model != "llama3.1:8b" {
		t.Errorf("got %+v", d)
	}
	if _, ok := d.Client.(*Ollama); !ok {
		t.Errorf("client is %T", d.Client)
	}
}

func TestDetectOpenAIWithoutV1InEndpoint(t *testing.T) {
	srv := serve(map[string]string{"/v1/models": `{"object":"list","data":[{"id":"strata-qwen"}]}`})
	defer srv.Close()

	d, err := Detect(context.Background(), Config{Endpoint: srv.URL, Model: "mine"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Provider != "openai" || d.Endpoint != srv.URL+"/v1" || d.Model != "mine" {
		t.Errorf("got %+v (explicit --model must be kept)", d)
	}
}

func TestDetectOpenAIWithV1Endpoint(t *testing.T) {
	srv := serve(map[string]string{"/v1/models": `{"data":[{"id":"m"}]}`})
	defer srv.Close()

	d, err := Detect(context.Background(), Config{Endpoint: srv.URL + "/v1/"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Provider != "openai" || d.Model != "m" {
		t.Errorf("got %+v", d)
	}
}

// A web app that returns HTML with 200 for every path must not be detected.
func TestDetectIgnoresHTMLCatchAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<!doctype html><html></html>"))
	}))
	defer srv.Close()

	_, err := Detect(context.Background(), Config{Endpoint: srv.URL})
	if err == nil || !strings.Contains(err.Error(), "no local AI server found") {
		t.Fatalf("err = %v", err)
	}
}

func TestDetectNoModels(t *testing.T) {
	srv := serve(map[string]string{"/api/tags": `{"models":[]}`})
	defer srv.Close()

	if _, err := Detect(context.Background(), Config{Endpoint: srv.URL}); err == nil || !strings.Contains(err.Error(), "--model") {
		t.Fatalf("err = %v", err)
	}
}

func TestAutoPrefersExec(t *testing.T) {
	c, err := New("auto", Config{Command: []string{"echo"}})
	if err != nil {
		t.Fatal(err)
	}
	if d := c.(*Detected); d.Provider != "exec" {
		t.Errorf("provider = %s", d.Provider)
	}
}
