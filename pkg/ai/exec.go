package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func init() { Register("exec", NewExec) }

// Exec pipes the prompt to an external command's stdin and reads the reply
// from stdout. This lets you inject any local runner (llama-cli, a Python
// script, a shell wrapper) without writing Go.
type Exec struct {
	argv []string
}

func NewExec(cfg Config) (Client, error) {
	if len(cfg.Command) == 0 {
		return nil, errors.New(`exec: --exec is required (e.g. --exec "python my_model.py")`)
	}
	return &Exec{argv: cfg.Command}, nil
}

func (e *Exec) Chat(ctx context.Context, req Request) (string, error) {
	var prompt strings.Builder
	for _, m := range req.Messages {
		fmt.Fprintf(&prompt, "### %s\n%s\n\n", strings.ToUpper(m.Role), m.Content)
	}
	prompt.WriteString("### ASSISTANT\n")

	cmd := exec.CommandContext(ctx, e.argv[0], e.argv[1:]...)
	cmd.Stdin = strings.NewReader(prompt.String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("exec %s: %w: %s", e.argv[0], err, bytes.TrimSpace(stderr.Bytes()))
	}
	return stdout.String(), nil
}
