package node

import (
	"fmt"

	"github.com/valyala/fasthttp"
)

func getSystems(c *fasthttp.Client, u *fasthttp.URI) ([][]byte, error) {
	u.SetPath("/redfish/v1/Systems/")

	systemsVals, err := doGetRequest(c, u)
	if err != nil {
		return nil, err
	}

	members := systemsVals.GetArray("Members")
	if members == nil || len(members) == 0 {
		return [][]byte{}, fmt.Errorf("error unmarshaling JSON, members not found: %v", err)
	}

	links := [][]byte{}
	for _, member := range members {
		links = append(links, member.GetStringBytes("@odata.id"))
	}

	return links, nil
}
