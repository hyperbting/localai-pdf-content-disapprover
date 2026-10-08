package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hyperbting/localai-pdf-content-disapprover/pkg/ai"
)

func newProvidersCmd(a *app) *cobra.Command {
	var detect bool
	c := &cobra.Command{
		Use:   "providers",
		Short: "List registered AI providers, or detect a running local AI server",
		Example: `  disapprover providers
  disapprover providers --detect
  disapprover providers --detect -e http://127.0.0.1:8080`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !detect {
				for _, p := range ai.Providers() {
					fmt.Fprintln(c.OutOrStdout(), p)
				}
				return nil
			}
			d, err := ai.Detect(c.Context(), a.config())
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "provider: %s\nendpoint: %s\nmodel:    %s\n", d.Provider, d.Endpoint, d.Model)
			return nil
		},
	}
	c.Flags().BoolVar(&detect, "detect", false, "probe --endpoint (or the default local ports) and report what was found")
	return c
}
