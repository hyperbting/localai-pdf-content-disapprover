// Package cmd wires the Cobra command tree.
package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hyperbting/localai-pdf-content-disapprover/pkg/ai"
)

// Option customizes the root command; used for programmatic injection.
type Option func(*app)

// WithClient injects a ready-made AI client, bypassing --provider entirely.
func WithClient(c ai.Client) Option { return func(a *app) { a.injected = c } }

type app struct {
	injected ai.Client

	provider string
	endpoint string
	model    string
	apiKey   string
	execCmd  string
	execArgs []string
	timeout  time.Duration
	effort   string
}

// client returns the injected client or builds one from flags.
func (a *app) client() (ai.Client, error) {
	if a.injected != nil {
		return a.injected, nil
	}
	if len(a.execArgs) > 0 && a.execCmd == "" {
		return nil, fmt.Errorf("--exec-arg needs --exec")
	}
	return ai.New(a.provider, a.config())
}

func (a *app) config() ai.Config {
	return ai.Config{
		Endpoint: a.endpoint,
		Model:    a.model,
		APIKey:   a.apiKey,
		Command:  a.command(),
		Timeout:  a.timeout,

		ReasoningEffort: a.effort,
	}
}

// command turns --exec and --exec-arg into argv. With --exec-arg, or when
// --exec names an existing file, --exec is the program path as given, so
// paths with spaces work. Otherwise --exec is split on whitespace.
func (a *app) command() []string {
	if a.execCmd == "" {
		return nil
	}
	if len(a.execArgs) > 0 || isFile(a.execCmd) {
		return append([]string{a.execCmd}, a.execArgs...)
	}
	return strings.Fields(a.execCmd)
}

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

// NewRootCmd builds the CLI. Embed it in your own main with options to inject
// a custom AI backend:
//
//	cmd.NewRootCmd(cmd.WithClient(myClient)).Execute()
func NewRootCmd(opts ...Option) *cobra.Command {
	a := &app{}
	for _, o := range opts {
		o(a)
	}

	root := &cobra.Command{
		Use:   "disapprover",
		Short: "Review PDF content with a local AI and approve or disapprove it",
		Long: `disapprover loads a PDF, sends its text to a pluggable local AI model,
and reports whether the content is approved or disapproved against your rules.
Results can be saved and committed to git.

AI providers:
  auto    (default) detect a running server: --endpoint if given, else Ollama :11434,
          then OpenAI-compatible :8080, :1234, :8000; uses the first listed model if --model is empty
  ollama  Ollama native API          (default endpoint http://localhost:11434)
  openai  any OpenAI-compatible API  (LocalAI, LM Studio, llama.cpp, vLLM, Strata; default http://localhost:8080/v1)
  exec    any command: prompt on stdin, reply on stdout. For paths with spaces use
          --exec "C:\Program Files\x\run.exe" --exec-arg "--model" --exec-arg "D:\my models\q.gguf"

Flags can also be set with env vars: DISAPPROVER_PROVIDER, DISAPPROVER_ENDPOINT,
DISAPPROVER_MODEL, DISAPPROVER_API_KEY, DISAPPROVER_EXEC, DISAPPROVER_REASONING_EFFORT.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&a.provider, "provider", "p", env("DISAPPROVER_PROVIDER", "auto"), "AI provider: "+strings.Join(ai.Providers(), "|"))
	pf.StringVarP(&a.endpoint, "endpoint", "e", env("DISAPPROVER_ENDPOINT", ""), "AI server base URL (provider default if empty)")
	pf.StringVarP(&a.model, "model", "m", env("DISAPPROVER_MODEL", ""), "model name")
	pf.StringVar(&a.apiKey, "api-key", env("DISAPPROVER_API_KEY", ""), "bearer token for the AI server, if required")
	pf.StringVar(&a.execCmd, "exec", env("DISAPPROVER_EXEC", ""), "command for the exec provider; split on spaces unless it is an existing file or --exec-arg is used")
	pf.StringArrayVar(&a.execArgs, "exec-arg", nil, "argument for the --exec program, passed as is (repeatable; keeps spaces)")
	pf.DurationVar(&a.timeout, "timeout", 5*time.Minute, "per-request AI timeout")
	pf.StringVar(&a.effort, "reasoning-effort", env("DISAPPROVER_REASONING_EFFORT", ""), "reasoning_effort for openai-compatible servers that support it (none|low|medium|high)")

	root.AddCommand(newExtractCmd(a), newReviewCmd(a), newProvidersCmd(a))
	return root
}

// Execute runs the CLI and exits with the returned code.
func Execute(opts ...Option) {
	if err := NewRootCmd(opts...).Execute(); err != nil {
		var ec exitCode
		if asExitCode(err, &ec) {
			os.Exit(int(ec))
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
