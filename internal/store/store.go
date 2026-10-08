// Package store writes output files and optionally commits them to git.
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WriteJSON writes v as indented JSON, creating parent dirs.
func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFile(path, append(data, '\n'))
}

// WriteFile writes data, creating parent dirs.
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// CommitOptions controls Commit.
type CommitOptions struct {
	Message string
	// Init creates a repository in the file's directory if it is not already in one.
	Init bool
}

// Commit stages and commits only the given paths, returning the new commit hash.
func Commit(ctx context.Context, paths []string, opt CommitOptions) (string, error) {
	if len(paths) == 0 {
		return "", errors.New("commit: no paths")
	}
	abs := make([]string, len(paths))
	for i, p := range paths {
		a, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		abs[i] = a
	}
	dir := filepath.Dir(abs[0])

	if _, err := git(ctx, dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		if !opt.Init {
			return "", fmt.Errorf("%s is not inside a git repository (use --git-init to create one)", dir)
		}
		if _, err := git(ctx, dir, "init"); err != nil {
			return "", err
		}
	}

	addArgs := append([]string{"add", "--"}, abs...)
	if _, err := git(ctx, dir, addArgs...); err != nil {
		return "", err
	}
	msg := opt.Message
	if msg == "" {
		msg = "Update " + filepath.Base(abs[0])
	}
	commitArgs := append([]string{"commit", "-m", msg, "--"}, abs...)
	if _, err := git(ctx, dir, commitArgs...); err != nil {
		return "", err
	}
	return git(ctx, dir, "rev-parse", "--short", "HEAD")
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stderr.String() + " " + stdout.String())
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(stdout.String()), nil
}
