package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgPath string

func main() {
	root := &cobra.Command{
		Use:   "probex",
		Short: "ProbeX — Network Quality Monitoring",
		Long: `ProbeX is a distributed network quality monitoring platform.

Modes:
  standalone  Single-node mode (default) — runs both hub and local agent
  hub         Hub mode — accepts agent connections, no local probing
  agent       Agent mode — connects to hub, executes probes locally`,
	}

	root.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "config file path")

	root.AddCommand(standaloneCmd())
	root.AddCommand(hubCmd())
	root.AddCommand(agentCmd())

	// Default to standalone if no subcommand given
	if len(os.Args) == 1 || (len(os.Args) > 1 && os.Args[1][0] == '-') {
		os.Args = append([]string{os.Args[0], "standalone"}, os.Args[1:]...)
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
