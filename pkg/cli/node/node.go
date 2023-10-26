package node

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/bensallen/nodeset"
	"github.com/bgentry/go-netrc/netrc"
	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

func Status(c *cli.Context) error {
	nodes := c.StringSlice("node")
	netrcPath := c.String("netrc")
	insecure := c.Bool("insecure")
	timeout := c.Int64("timeout")
	retry := c.Int("retry")
	state := c.String("state")

	if state != "" {
		switch state {
		case "On":
		case "Off":
		case "Failed":
		default:
			return fmt.Errorf("--state must be [On|Off|Failed]")
		}
	}

	nr, err := netrc.ParseFile(netrcPath)
	if err != nil {
		return err
	}

	readTimeout := time.Duration(timeout * 1000000)
	writeTimeout := time.Duration(timeout * 1000000)
	maxIdleConnDuration, _ := time.ParseDuration("1h")

	client := &fasthttp.Client{
		ReadTimeout:               readTimeout,
		WriteTimeout:              writeTimeout,
		MaxIdleConnDuration:       maxIdleConnDuration,
		MaxIdemponentCallAttempts: retry,
		NoDefaultUserAgentHeader:  true, // Don't send: User-Agent: fasthttp
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
		go func(c *fasthttp.Client, u *fasthttp.URI, f string) {
			defer wg.Done()
			getPowerState(c, u, f)
		}(client, url, state)

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

func getPowerState(c *fasthttp.Client, url *fasthttp.URI, filter string) {
	sysLinks, err := getSystems(c, url)
	if err != nil {
		if filter == "" || filter == "Failed" {
			fmt.Printf("%s: %s\n", url.Host(), err)
		}
		return
	}
	for _, sysLink := range sysLinks {
		url.SetPathBytes(sysLink)
		powerState, err := getSystem(c, url)
		if err != nil {
			if filter == "" || filter == "Failed" {
				fmt.Printf("%s: %s\n", url.Host(), err)
			}
			return
		}
		if filter == "" || powerState == filter {
			fmt.Printf("%s: %s\n", url.Host(), powerState)
		}
	}
	fasthttp.ReleaseURI(url)
}

func getSystems(c *fasthttp.Client, u *fasthttp.URI) ([][]byte, error) {
	u.SetPath("/redfish/v1/Systems/")
	req := fasthttp.AcquireRequest()
	req.SetURI(u)

	req.Header.SetMethod(fasthttp.MethodGet)
	resp := fasthttp.AcquireResponse()
	err := c.Do(req, resp)
	fasthttp.ReleaseRequest(req)

	if err != nil {
		return [][]byte{}, fmt.Errorf("connection error: %v", err)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(resp.Body())
	if err != nil {
		return [][]byte{}, fmt.Errorf("error unmarshaling JSON body: %v", err)
	}
	members := v.GetArray("Members")
	if members == nil || len(members) == 0 {
		return [][]byte{}, fmt.Errorf("error unmarshaling JSON, members not found: %v", err)
	}

	links := [][]byte{}
	for _, member := range members {
		links = append(links, member.GetStringBytes("@odata.id"))
	}

	return links, nil
}

func getSystem(c *fasthttp.Client, u *fasthttp.URI) (string, error) {
	req := fasthttp.AcquireRequest()
	req.SetURI(u)

	req.Header.SetMethod(fasthttp.MethodGet)
	resp := fasthttp.AcquireResponse()
	err := c.Do(req, resp)
	fasthttp.ReleaseRequest(req)

	if err != nil {
		return "", fmt.Errorf("connection error: %v", err)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(resp.Body())
	if err != nil {
		return "", fmt.Errorf("error unmarshaling JSON body: %v", err)
	}
	powerState := v.GetStringBytes("PowerState")

	return string(powerState), nil
}
