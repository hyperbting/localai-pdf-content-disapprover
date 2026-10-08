package ai

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is not a real test: the exec tests run the test binary
// itself as the external command, so they work wherever go test does.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(2)
	}
	switch args[1] {
	case "echo":
		io.Copy(os.Stdout, os.Stdin)
	case "fail":
		fmt.Fprint(os.Stderr, "model not loaded\n")
		os.Exit(3)
	case "sleep":
		time.Sleep(time.Minute)
	}
	os.Exit(0)
}

func helperExec(t *testing.T, mode string) Client {
	t.Helper()
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	c, err := New("exec", Config{Command: []string{os.Args[0], "-test.run=^TestHelperProcess$", "--", mode}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestExecSendsPromptOnStdin(t *testing.T) {
	reply, err := helperExec(t, "echo").Chat(context.Background(), Request{Messages: []Message{
		{Role: "system", Content: "be strict"},
		{Role: "user", Content: "page 1 text"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "### SYSTEM\nbe strict\n\n### USER\npage 1 text\n\n### ASSISTANT\n"
	if reply != want {
		t.Errorf("stdin seen by command = %q, want %q", reply, want)
	}
}

func TestExecReportsStderrOnFailure(t *testing.T) {
	_, err := helperExec(t, "fail").Chat(context.Background(), Request{Messages: []Message{{Role: "user", Content: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "model not loaded") {
		t.Fatalf("err = %v, want it to include the command's stderr", err)
	}
}

func TestExecStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := helperExec(t, "sleep").Chat(ctx, Request{Messages: []Message{{Role: "user", Content: "x"}}}); err == nil {
		t.Fatal("expected an error when the context is done")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("command was not stopped (took %s)", d)
	}
}

func TestExecRequiresCommand(t *testing.T) {
	if _, err := New("exec", Config{}); err == nil || !strings.Contains(err.Error(), "--exec") {
		t.Fatalf("err = %v", err)
	}
}
