package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/hyperbting/localai-pdf-content-disapprover/internal/pdftext"
	"github.com/hyperbting/localai-pdf-content-disapprover/internal/store"
)

func newExtractCmd(_ *app) *cobra.Command {
	var (
		save   saveFlags
		asJSON bool
	)
	c := &cobra.Command{
		Use:   "extract <file.pdf>",
		Short: "Load a PDF and print or save its text",
		Example: `  disapprover extract report.pdf
  disapprover extract report.pdf --json -o out/report.json --commit`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if err := save.validate(); err != nil {
				return err
			}
			doc, err := pdftext.Load(args[0])
			if err != nil {
				return err
			}

			if save.out == "" {
				if asJSON {
					return printJSON(c.OutOrStdout(), doc)
				}
				_, err := fmt.Fprint(c.OutOrStdout(), doc.Text())
				return err
			}

			if asJSON {
				err = store.WriteJSON(save.out, doc)
			} else {
				err = store.WriteFile(save.out, []byte(doc.Text()))
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(c.ErrOrStderr(), "saved %d pages to %s\n", len(doc.Pages), save.out)
			return save.commitIfRequested(c, fmt.Sprintf("Extract text from %s", filepath.Base(args[0])))
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "output JSON with per-page text and file hash")
	save.register(c, "write extracted text to this file instead of stdout")
	return c
}
