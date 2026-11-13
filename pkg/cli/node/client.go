package node

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

var headerContentTypeJson = []byte("application/json")

func newClient(c *cli.Context) *fasthttp.Client {
	insecure := c.Bool("insecure")
	timeout := c.Int64("timeout")
	retry := c.Int("retry")

	readTimeout := time.Duration(timeout * 1000000)
	writeTimeout := time.Duration(timeout * 1000000)
	maxIdleConnDuration, _ := time.ParseDuration("1h")

	return &fasthttp.Client{
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
}

func doGetRequest(c *fasthttp.Client, u *fasthttp.URI) (*fastjson.Value, error) {
	req := fasthttp.AcquireRequest()
	req.SetURI(u)

	req.Header.SetMethod(fasthttp.MethodGet)
	resp := fasthttp.AcquireResponse()
	err := c.Do(req, resp)
	fasthttp.ReleaseRequest(req)

	if err != nil {
		return nil, fmt.Errorf("connection error: %v", err)
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(resp.Body())
	fasthttp.ReleaseResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling JSON body: %v", err)
	}
	return v, nil
}

// doPostRequest posts body to u and returns the HTTP status code along with
// the parsed response body. A nil value is returned when the response has no
// body, which is common for Redfish actions that reply with 204 No Content.
func doPostRequest(c *fasthttp.Client, u *fasthttp.URI, body []byte) (int, *fastjson.Value, error) {
	req := fasthttp.AcquireRequest()
	req.SetURI(u)

	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentTypeBytes(headerContentTypeJson)
	req.SetBodyRaw(body)

	resp := fasthttp.AcquireResponse()
	err := c.Do(req, resp)

	fasthttp.ReleaseRequest(req)

	if err != nil {
		fasthttp.ReleaseResponse(resp)
		return 0, nil, fmt.Errorf("connection error: %v", err)
	}

	status := resp.StatusCode()

	respBody := bytes.TrimSpace(resp.Body())
	if len(respBody) == 0 {
		fasthttp.ReleaseResponse(resp)
		return status, nil, nil
	}

	var p fastjson.Parser
	v, err := p.ParseBytes(respBody)
	fasthttp.ReleaseResponse(resp)
	if err != nil {
		return status, nil, fmt.Errorf("error unmarshaling JSON body: %v", err)
	}
	return status, v, nil
}
