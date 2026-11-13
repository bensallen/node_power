package node

import (
	"bytes"
	"fmt"
	"io"
	"slices"

	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

func On(c *cli.Context) error {
	if err := c.Set("command", "On"); err != nil {
		return err
	}
	return Command(c)
}

func Off(c *cli.Context) error {
	if err := c.Set("command", "Off"); err != nil {
		return err
	}
	return Command(c)
}

func PushPowerButton(c *cli.Context) error {
	if err := c.Set("command", "PushPowerButton"); err != nil {
		return err
	}
	return Command(c)
}

func GracefulRestart(c *cli.Context) error {
	if err := c.Set("command", "GracefulRestart"); err != nil {
		return err
	}
	return Command(c)
}

func ForceOff(c *cli.Context) error {
	if err := c.Set("command", "ForceOff"); err != nil {
		return err
	}
	return Command(c)
}

func NMI(c *cli.Context) error {
	if err := c.Set("command", "NMI"); err != nil {
		return err
	}
	return Command(c)
}

func ForceOn(c *cli.Context) error {
	if err := c.Set("command", "ForceOn"); err != nil {
		return err
	}
	return Command(c)
}

func ForceRestart(c *cli.Context) error {
	if err := c.Set("command", "ForceRestart"); err != nil {
		return err
	}
	return Command(c)
}

func Command(c *cli.Context) error {
	command := c.String("command")
	if command == "" {
		return fmt.Errorf("--command must be set")
	}

	return eachBMC(c, func(client *fasthttp.Client, u *fasthttp.URI, out io.Writer) {
		postPowerCommand(client, u, out, command)
	})
}

// postPowerCommand issues a ComputerSystem.Reset action against every system
// advertised by the BMC.
func postPowerCommand(c *fasthttp.Client, url *fasthttp.URI, out io.Writer, cmd string) {
	defer fasthttp.ReleaseURI(url)

	sysLinks, err := getSystems(c, url)
	if err != nil {
		fmt.Fprintf(out, "%s: %s\n", url.Host(), err)
		return
	}
	for _, sysLink := range sysLinks {
		url.SetPathBytes(sysLink)
		systemValues, err := doGetRequest(c, url)
		if err != nil {
			fmt.Fprintf(out, "%s: %s\n", url.Host(), err)
			return
		}
		resetValues := allowableResetValues(systemValues, c, url)

		// Only enforce the allow list when the BMC actually advertises one.
		if len(resetValues) > 0 && slices.IndexFunc(resetValues, func(b []byte) bool { return bytes.Equal([]byte(cmd), b) }) == -1 {
			fmt.Fprintf(out, "%s: command %s, not an allowable reset value: %s\n", url.Host(), cmd, resetValues)
			return
		}

		actionTargetUrl := systemValues.GetStringBytes("Actions", "#ComputerSystem.Reset", "target")
		if len(actionTargetUrl) == 0 {
			fmt.Fprintf(out, "%s: ComputerSystem.Reset action not supported\n", url.Host())
			return
		}

		url.SetPathBytes(actionTargetUrl)
		status, postVals, err := doPostRequest(c, url, []byte(fmt.Sprintf(`{ "ResetType": "%s" }`, cmd)))
		if err != nil {
			fmt.Fprintf(out, "%s: %s\n", url.Host(), err)
			return
		}

		printActionResponse(out, url.Host(), status, postVals)
	}
}

// allowableResetValues reports the reset types the system advertises, either
// inline or via @Redfish.ActionInfo. An empty result means the system
// published no allow list, in which case callers should not enforce one.
func allowableResetValues(systemValues *fastjson.Value, c *fasthttp.Client, url *fasthttp.URI) [][]byte {
	allowableInfoList := systemValues.GetArray("Actions", "#ComputerSystem.Reset", "ResetType@Redfish.AllowableValues")

	if len(allowableInfoList) == 0 {
		actionInfoUrl := systemValues.GetStringBytes("Actions", "#ComputerSystem.Reset", "@Redfish.ActionInfo")
		if len(actionInfoUrl) == 0 {
			return nil
		}

		url.SetPathBytes(actionInfoUrl)
		actionInfoVals, err := doGetRequest(c, url)
		if err != nil {
			// Treated as "no allow list published" so the command still runs.
			return nil
		}

		for _, params := range actionInfoVals.GetArray("Parameters") {
			if string(params.GetStringBytes("Name")) == "ResetType" {
				allowableInfoList = params.GetArray("AllowableValues")
			}
		}
	}

	actionAllowableValues := make([][]byte, len(allowableInfoList))
	for idx, allowableValue := range allowableInfoList {
		actionAllowableValues[idx] = allowableValue.GetStringBytes()
	}

	return actionAllowableValues
}
