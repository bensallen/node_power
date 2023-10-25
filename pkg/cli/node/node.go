package node

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/bensallen/nodeset"
	"github.com/bgentry/go-netrc/netrc"
	"github.com/stmcginnis/gofish/redfish"
	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
)

func Status(c *cli.Context) error {
	nodes := c.StringSlice("node")
	netrcPath := c.String("netrc")
	insecure := c.Bool("insecure")
	timeout := c.Int64("timeout")

	nr, err := netrc.ParseFile(netrcPath)
	if err != nil {
		return err
	}

	readTimeout := time.Duration(timeout * 1000000)
	writeTimeout := time.Duration(timeout * 1000000)
	maxIdleConnDuration, _ := time.ParseDuration("1h")
	client := &fasthttp.Client{
		ReadTimeout:              readTimeout,
		WriteTimeout:             writeTimeout,
		MaxIdleConnDuration:      maxIdleConnDuration,
		NoDefaultUserAgentHeader: true, // Don't send: User-Agent: fasthttp
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
		TLSConfig: &tls.Config{InsecureSkipVerify: insecure},
	}

	var wg sync.WaitGroup

	f := func(s string) error {
		machine := nr.FindMachine(s)
		if machine == nil {
			return fmt.Errorf("node %s not found in netrc", s)
		}

		url := fasthttp.AcquireURI()
		url.SetUsername(machine.Login)
		url.SetPassword(machine.Password)
		url.SetScheme("https")
		url.SetHost(s)

		wg.Add(1)
		go func(c *fasthttp.Client, u *fasthttp.URI) {
			defer wg.Done()
			getPowerState(client, url)
		}(client, url)

		return nil
	}

	for _, node := range nodes {
		err := nodeset.Expand(node, f)
		if err != nil {
			return err
		}
	}

	wg.Wait()

	return nil
}

type SystemsCollection struct {
	OdataID   string `json:"@odata.id"`
	OdataType string `json:"@odata.type"`
	Members   []struct {
		OdataID string `json:"@odata.id"`
	} `json:"Members"`
	MembersOdataCount int    `json:"Members@odata.count"`
	Name              string `json:"Name"`
}

func getPowerState(c *fasthttp.Client, url *fasthttp.URI) {
	sc, err := getSystems(c, url)
	if err != nil {
		fmt.Printf("%s:%s\n", url.Host(), err)
	}
	for _, systemPath := range sc.Members {
		url.SetPath(systemPath.OdataID)
		cs, err := getSystem(c, url)
		if err != nil {
			fmt.Printf("%s:%s\n", url.Host(), err)
		}
		fmt.Printf("%s:%s\n", url.Host(), cs.PowerState)
	}
	fasthttp.ReleaseURI(url)
}

func getSystems(c *fasthttp.Client, u *fasthttp.URI) (SystemsCollection, error) {

	u.SetPath("/redfish/v1/Systems/")
	req := fasthttp.AcquireRequest()
	req.SetURI(u)

	req.Header.SetMethod(fasthttp.MethodGet)
	resp := fasthttp.AcquireResponse()
	err := c.Do(req, resp)
	fasthttp.ReleaseRequest(req)

	if err != nil {
		return SystemsCollection{}, fmt.Errorf("connection error: %v\n", err)
	}
	sc := SystemsCollection{}
	json.Unmarshal(resp.Body(), &sc)

	return sc, nil
}

func getSystem(c *fasthttp.Client, u *fasthttp.URI) (redfish.ComputerSystem, error) {
	cs := redfish.ComputerSystem{}

	req := fasthttp.AcquireRequest()
	req.SetURI(u)

	req.Header.SetMethod(fasthttp.MethodGet)
	resp := fasthttp.AcquireResponse()
	err := c.Do(req, resp)
	fasthttp.ReleaseRequest(req)

	if err != nil {
		return redfish.ComputerSystem{}, fmt.Errorf("connection error: %v\n", err)
	}

	if err := json.Unmarshal(resp.Body(), &cs); err != nil {
		return redfish.ComputerSystem{}, fmt.Errorf("error unmarshaling JSON body: %v\n", err)
	}

	return cs, nil
}
