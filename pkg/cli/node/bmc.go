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

const managersPath = "/redfish/v1/Managers/"

func BMCGracefulRestart(c *cli.Context) error {
	if err := c.Set("bmc-command", "GracefulRestart"); err != nil {
		return err
	}
	return BMCReset(c)
}

func BMCForceRestart(c *cli.Context) error {
	if err := c.Set("bmc-command", "ForceRestart"); err != nil {
		return err
	}
	return BMCReset(c)
}

// BMCReset issues a Manager.Reset action against every manager advertised by
// each BMC.
func BMCReset(c *cli.Context) error {
	cmd := c.String("bmc-command")
	if cmd == "" {
		return fmt.Errorf("--bmc-command must be set")
	}

	return eachBMC(c, func(client *fasthttp.Client, u *fasthttp.URI, out io.Writer) {
		postManagerReset(client, u, out, cmd)
	})
}

func postManagerReset(c *fasthttp.Client, url *fasthttp.URI, out io.Writer, cmd string) {
	defer fasthttp.ReleaseURI(url)

	mgrLinks, err := getManagers(c, url)
	if err != nil {
		fmt.Fprintf(out, "%s: %s\n", url.Host(), err)
		return
	}

	for _, mgrLink := range mgrLinks {
		url.SetPathBytes(mgrLink)
		mgrVals, err := doGetRequest(c, url)
		if err != nil {
			fmt.Fprintf(out, "%s: %s\n", url.Host(), err)
			return
		}

		resetValues := allowableManagerResetValues(mgrVals, c, url)

		// Only enforce the allow list when the BMC actually advertises one.
		if len(resetValues) > 0 && slices.IndexFunc(resetValues, func(b []byte) bool { return bytes.Equal([]byte(cmd), b) }) == -1 {
			fmt.Fprintf(out, "%s: command %s, not an allowable reset value: %s\n", url.Host(), cmd, resetValues)
			return
		}

		actionTargetUrl := mgrVals.GetStringBytes("Actions", "#Manager.Reset", "target")
		if len(actionTargetUrl) == 0 {
			fmt.Fprintf(out, "%s: Manager.Reset action not supported\n", url.Host())
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

func getManagers(c *fasthttp.Client, u *fasthttp.URI) ([][]byte, error) {
	u.SetPath(managersPath)

	managersVals, err := doGetRequest(c, u)
	if err != nil {
		return nil, err
	}

	members := managersVals.GetArray("Members")
	if len(members) == 0 {
		return nil, fmt.Errorf("error unmarshaling JSON, managers members not found")
	}

	links := make([][]byte, 0, len(members))
	for _, member := range members {
		links = append(links, member.GetStringBytes("@odata.id"))
	}

	return links, nil
}

// allowableManagerResetValues reports the reset types the manager advertises,
// either inline or via @Redfish.ActionInfo. An empty result means the manager
// published no allow list, in which case callers should not enforce one.
func allowableManagerResetValues(mgrVals *fastjson.Value, c *fasthttp.Client, url *fasthttp.URI) [][]byte {
	allowableInfoList := mgrVals.GetArray("Actions", "#Manager.Reset", "ResetType@Redfish.AllowableValues")

	if len(allowableInfoList) == 0 {
		actionInfoUrl := mgrVals.GetStringBytes("Actions", "#Manager.Reset", "@Redfish.ActionInfo")
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

	allowableValues := make([][]byte, len(allowableInfoList))
	for idx, allowableValue := range allowableInfoList {
		allowableValues[idx] = allowableValue.GetStringBytes()
	}

	return allowableValues
}
