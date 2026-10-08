package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hyperbting/localai-pdf-content-disapprover/internal/pdftext"
	"github.com/hyperbting/localai-pdf-content-disapprover/internal/review"
	"github.com/hyperbting/localai-pdf-content-disapprover/internal/store"
)

func newReviewCmd(a *app) *cobra.Command {
	var (
		save       saveFlags
		rulesFile  string
		maxChars   int
		failOnDeny bool
		quiet      bool
	)
	c := &cobra.Command{
		Use:   "review <file.pdf>",
		Short: "Ask the local AI to approve or disapprove a PDF's content",
		Example: `  disapprover review contract.pdf -m llama3.1
  disapprover review contract.pdf -p openai -e http://localhost:1234/v1 -m qwen2.5-7b-instruct
  disapprover review contract.pdf -p exec --exec "python my_model.py"
  disapprover review contract.pdf -m llama3.1 --rules rules.txt -o reviews/contract.json --commit --fail-on-disapprove`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := save.validate(); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			rules := ""
			if rulesFile != "" {
				b, err := os.ReadFile(rulesFile)
				if err != nil {
					return err
				}
				rules = string(b)
			}

			doc, err := pdftext.Load(args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(doc.Text()) == "" {
				return fmt.Errorf("%s has no extractable text (scanned PDF? run OCR first)", args[0])
			}

			rv := &review.Reviewer{Client: client, Rules: rules, MaxChars: maxChars}
			if !quiet {
				rv.Progress = func(i, n, first, last int) {
					fmt.Fprintf(c.ErrOrStderr(), "reviewing chunk %d/%d (pages %d-%d)...\n", i, n, first, last)
				}
			}
			rep, err := rv.Review(c.Context(), doc)
			if err != nil {
				return err
			}
			if a.injected == nil {
				rep.Provider, rep.Model = a.provider, a.model
			}

			if save.out != "" {
				if err := store.WriteJSON(save.out, rep); err != nil {
					return err
				}
				fmt.Fprintf(c.ErrOrStderr(), "saved report to %s\n", save.out)
				msg := fmt.Sprintf("Review %s: %s (%d findings)", filepath.Base(args[0]), rep.Verdict, len(rep.Findings))
				if err := save.commitIfRequested(c, msg); err != nil {
					return err
				}
			}
			printSummary(c.OutOrStdout(), rep)

			if failOnDeny && !rep.Approved() {
				return exitCode(2)
			}
			return nil
		},
	}
	f := c.Flags()
	f.StringVarP(&rulesFile, "rules", "r", "", "file with review rules (built-in rules if empty)")
	f.IntVar(&maxChars, "max-chars", 8000, "max characters of PDF text per AI request")
	f.BoolVar(&failOnDeny, "fail-on-disapprove", false, "exit with status 2 when the verdict is disapprove (for CI)")
	f.BoolVarP(&quiet, "quiet", "q", false, "suppress progress output")
	save.register(c, "save the JSON report to this file")
	return c
}

func printSummary(w io.Writer, rep *review.Report) {
	mark := "APPROVED"
	if !rep.Approved() {
		mark = "DISAPPROVED"
	}
	fmt.Fprintf(w, "%s  %s  (%d pages, %d findings)\n", mark, rep.File, rep.Pages, len(rep.Findings))
	for _, f := range rep.Findings {
		fmt.Fprintf(w, "  - p.%d [%s/%s] %s\n", f.Page, f.Severity, f.Category, f.Reason)
		if f.Excerpt != "" {
			fmt.Fprintf(w, "      %q\n", f.Excerpt)
		}
	}
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
