package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hyperbting/localai-pdf-content-disapprover/pkg/ai"
)

func newProvidersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "providers",
		Short: "List registered AI providers",
		Args:  cobra.NoArgs,
		Run: func(c *cobra.Command, _ []string) {
			for _, p := range ai.Providers() {
				fmt.Fprintln(c.OutOrStdout(), p)
			}
		},
	}
}
