package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/bensallen/node_power/pkg/cli/docs"
	"github.com/bensallen/node_power/pkg/cli/node"
	"github.com/urfave/cli/v2"
)

var version = "unknown"

func main() {

	homeDir, _ := os.UserHomeDir()

	app := &cli.App{

		Name:                 "nodepower",
		Usage:                "CLI Interface to control power",
		Version:              version,
		HideHelpCommand:      true,
		EnableBashCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "netrc",
				Aliases: []string{"n"},
				Usage:   "load authentication information from netrc `FILE`",
				Value:   filepath.Join(homeDir, ".netrc"),
			},
			&cli.BoolFlag{
				Name:    "insecure",
				Aliases: []string{"k"},
				Usage:   "skip TLS verification of the connection to the node",
				Value:   false,
			},
			&cli.Int64Flag{
				Name:    "timeout",
				Aliases: []string{"t"},
				Usage:   "Timeout to wait for connections (ms)",
				Value:   500,
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "status",
				Usage:  "power status a node",
				Action: node.Status,
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "node", Aliases: []string{"n"}, Required: true, Usage: "list of node with support for node ranges, and can be specified multiple times"},
					&cli.StringFlag{Name: "status", Aliases: []string{"s"}, Usage: "filter output based on status of node: on, off", Value: ""},
				},
			},
			{
				Name:   "manual",
				Hidden: true,
				Action: docs.Manual,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "output", Required: true, Usage: "path to write man page"},
				},
			},
			{
				Name:   "readme",
				Hidden: true,
				Action: docs.Readme,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "output", Required: true, Usage: "path to write readme"},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatalf("error, %v", err)
	}
}
