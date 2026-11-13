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

		Name:                      "nodepower",
		Usage:                     "CLI Interface to control power",
		Version:                   version,
		HideHelpCommand:           true,
		EnableBashCompletion:      true,
		DisableSliceFlagSeparator: true, // Nodesets as passed to --nodes can have commas in them, so disable naive splitting based on commas.
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:     "node",
				Aliases:  []string{"n"},
				Required: true,
				Usage:    "list of nodes with support for node ranges (eg. node[1-2]), and can be specified multiple times",
			},
			&cli.StringFlag{
				Name:    "netrc",
				Aliases: []string{"a"},
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
				Usage:   "timeout to wait for connections (ms)",
				Value:   500,
			},
			&cli.IntFlag{
				Name:    "retry",
				Aliases: []string{"r"},
				Usage:   "retry attempts for each HTTP request",
				Value:   3,
			},
			&cli.StringFlag{
				Name:    "trim-suffix",
				Aliases: []string{"T"},
				Usage:   "trim `SUFFIX` from each node name to derive the BMC hostname (eg. --trim-suffix n0 turns node01n0 into node01)",
			},
			&cli.StringFlag{
				Name:    "suffix",
				Aliases: []string{"S"},
				Usage:   "append `SUFFIX` to each node name to derive the BMC hostname (eg. --suffix -bmc turns node01 into node01-bmc)",
			},
			&cli.IntFlag{
				Name:    "max-concurrent",
				Aliases: []string{"j"},
				Usage:   "maximum in-flight requests, or 0 for unlimited (one per node)",
				Value:   0,
			},
			&cli.StringFlag{
				Name:    "command",
				Aliases: []string{"c"},
				Hidden:  true,
			},
			&cli.StringFlag{
				Name:   "bmc-command",
				Hidden: true,
			},
		},
		Commands: []*cli.Command{
			{
				Name:   "Status",
				Usage:  "Get power status",
				Action: node.Status,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "state", Aliases: []string{"s"}, Usage: "filter output based on state: [On|Off|Failed]", Value: ""},
				},
			},
			{
				Name: "On", Category: "Power", Usage: "Power on", Action: node.On,
			},
			{
				Name: "Off", Category: "Power", Usage: "Power off", Action: node.Off,
			},
			{
				Name: "GracefulRestart", Category: "Power", Usage: "Trigger graceful restart", Action: node.GracefulRestart,
			},
			{
				Name: "PushPowerButton", Category: "Power", Usage: "Trigger ACPI power button press", Action: node.PushPowerButton,
			},
			{
				Name: "ForceOn", Category: "Power", Usage: "Force power on", Action: node.ForceOn,
			},
			{
				Name: "ForceOff", Category: "Power", Usage: "Force power off", Action: node.ForceOff,
			},
			{
				Name: "ForceRestart", Category: "Power", Usage: "Force restart", Action: node.ForceRestart,
			},
			{
				Name: "NMI", Category: "Power", Usage: "Trigger NMI", Action: node.NMI,
			},
			{
				Name:     "Command",
				Category: "Power",
				Hidden:   true,
				Usage:    "Send arbitrary power commands",
				Action:   node.Command,
			},
			{
				Name: "BMCGracefulRestart", Category: "BMC", Usage: "Gracefully restart the BMC itself", Action: node.BMCGracefulRestart,
			},
			{
				Name: "BMCForceRestart", Category: "BMC", Usage: "Force restart the BMC itself", Action: node.BMCForceRestart,
			},
			{
				Name:     "BMCReset",
				Category: "BMC",
				Hidden:   true,
				Usage:    "Send arbitrary Manager.Reset commands to the BMC itself",
				Action:   node.BMCReset,
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
