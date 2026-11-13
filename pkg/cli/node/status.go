package node

import (
	"fmt"
	"io"

	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
)

func Status(c *cli.Context) error {
	state := c.String("state")

	switch state {
	case "", "On", "Off", "Failed":
	default:
		return fmt.Errorf("--state must be [On|Off|Failed]")
	}

	return eachBMC(c, func(client *fasthttp.Client, u *fasthttp.URI, out io.Writer) {
		getPowerState(client, u, out, state)
	})
}

func getPowerState(c *fasthttp.Client, url *fasthttp.URI, out io.Writer, filter string) {
	defer fasthttp.ReleaseURI(url)

	sysLinks, err := getSystems(c, url)
	if err != nil {
		if filter == "" || filter == "Failed" {
			fmt.Fprintf(out, "%s: %s\n", url.Host(), err)
		}
		return
	}
	for _, sysLink := range sysLinks {
		url.SetPathBytes(sysLink)
		jsonVals, err := doGetRequest(c, url)
		if err != nil {
			if filter == "" || filter == "Failed" {
				fmt.Fprintf(out, "%s: %s\n", url.Host(), err)
			}
			return
		}
		powerState := jsonVals.GetStringBytes("PowerState")

		if filter == "" || string(powerState) == filter {
			fmt.Fprintf(out, "%s: %s\n", url.Host(), powerState)
		}
	}
}
