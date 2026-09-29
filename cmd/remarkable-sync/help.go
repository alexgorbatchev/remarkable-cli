package main

import (
	cobrahelptree "github.com/alexgorbatchev/cobra-help-tree"
	"github.com/spf13/cobra"
)

var techCatalog = cobrahelptree.TechCatalog{
	"remarkable-sync": {
		Summary:     "reMarkable Cloud daily planner sync and document export CLI",
		Description: "reMarkable Cloud daily planner sync and document export CLI - command-line interface.",
	},
	"remarkable-sync status": {
		Summary:     "Display system and environment status",
		Description: "Outputs current runtime readiness and version information.",
	},
}

func setupHelp(cmd *cobra.Command) {
	cobrahelptree.Setup(cmd, cobrahelptree.TreeOptions{
		TechCatalog: techCatalog,
	})
}
