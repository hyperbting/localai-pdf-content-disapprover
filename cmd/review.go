package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hyperbting/localai-pdf-content-disapprover/internal/pdftext"
	"github.com/hyperbting/localai-pdf-content-disapprover/internal/review"
	"github.com/hyperbting/localai-pdf-content-disapprover/internal/store"
	"github.com/hyperbting/localai-pdf-content-disapprover/pkg/ai"
)

// policyWarnRunes is the policy size above which review warns about context length.
const policyWarnRunes = 24000

func newReviewCmd(a *app) *cobra.Command {
	var (
		save       saveFlags
		pf         review.PolicyFiles
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
  disapprover review contract.pdf -m llama3.1 --rules rules.txt -o reviews/contract.json --commit --fail-on-disapprove
  disapprover review ad.pdf -m llama3.1 --laws pdpa.txt --laws fair-trade.txt --pass-examples ok-ads.txt --reason-format reason.txt`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := save.validate(); err != nil {
				return err
			}
			client, err := a.client()
			if err != nil {
				return err
			}
			provider, model := a.provider, a.model
			if d, ok := client.(*ai.Detected); ok {
				provider, model = d.Provider, d.Model
				if !quiet {
					fmt.Fprintf(c.ErrOrStderr(), "auto: using %s at %s (model %s)\n", d.Provider, d.Endpoint, d.Model)
				}
			}
			policy, err := review.LoadPolicy(pf)
			if err != nil {
				return err
			}
			// The policy is resent with every chunk, so it must fit the model's context.
			if n := len([]rune(policy.Text())); n > policyWarnRunes && !quiet {
				fmt.Fprintf(c.ErrOrStderr(), "warning: policy is %d characters and is sent with every chunk; make sure it fits your model's context window\n", n)
			}

			doc, err := pdftext.Load(args[0])
			if err != nil {
				return err
			}
			if strings.TrimSpace(doc.Text()) == "" {
				return fmt.Errorf("%s has no extractable text (scanned PDF? run OCR first)", args[0])
			}

			rv := &review.Reviewer{Client: client, Policy: policy, MaxChars: maxChars}
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
				rep.Provider, rep.Model = provider, model
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
	f.StringArrayVarP(&pf.Rules, "rules", "r", nil, "house rules txt file (repeatable; built-in rules if no --rules or --laws)")
	f.StringArrayVar(&pf.Laws, "laws", nil, "law/regulation txt file; content against it is disapproved (repeatable)")
	f.StringArrayVar(&pf.PassExamples, "pass-examples", nil, "txt file of content that should be approved, to calibrate the model (repeatable)")
	f.StringVar(&pf.ReasonFormat, "reason-format", "", "txt template for each finding's message, placeholders: {page} {rule_id} {law} {article} {citation} {category} {severity} {excerpt} {reason}")
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
		fmt.Fprintf(w, "  - %s\n", f.Message)
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
