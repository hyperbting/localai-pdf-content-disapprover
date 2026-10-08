package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hyperbting/localai-pdf-content-disapprover/internal/store"
)

// exitCode is returned as an error to request a specific process exit status
// without printing an error message.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func asExitCode(err error, target *exitCode) bool { return errors.As(err, target) }

// saveFlags is shared by every command that writes output.
type saveFlags struct {
	out     string
	commit  bool
	message string
	gitInit bool
}

func (s *saveFlags) register(c *cobra.Command, outHelp string) {
	f := c.Flags()
	f.StringVarP(&s.out, "out", "o", "", outHelp)
	f.BoolVarP(&s.commit, "commit", "c", false, "git commit the output file after saving (requires --out)")
	f.StringVar(&s.message, "message", "", "commit message (default generated)")
	f.BoolVar(&s.gitInit, "git-init", false, "run git init in the output directory if it is not a repository")
}

func (s *saveFlags) validate() error {
	if s.commit && s.out == "" {
		return errors.New("--commit requires --out")
	}
	return nil
}

// commitIfRequested commits s.out and reports the hash on stderr.
func (s *saveFlags) commitIfRequested(c *cobra.Command, defaultMsg string) error {
	if !s.commit {
		return nil
	}
	msg := s.message
	if msg == "" {
		msg = defaultMsg
	}
	hash, err := store.Commit(c.Context(), []string{s.out}, store.CommitOptions{Message: msg, Init: s.gitInit})
	if err != nil {
		return err
	}
	fmt.Fprintf(c.ErrOrStderr(), "committed %s as %s\n", s.out, hash)
	return nil
}
