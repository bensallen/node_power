package node

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

// benchNetrc writes a netrc containing n machines named node<i>-bmc and
// returns its path.
func benchNetrc(b *testing.B, n int) string {
	b.Helper()
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("machine node")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("-bmc login root password calvin\n")
	}
	path := filepath.Join(b.TempDir(), "netrc")
	if err := os.WriteFile(path, []byte(sb.String()), 0600); err != nil {
		b.Fatalf("writing netrc: %v", err)
	}
	return path
}

// benchContext builds a context over a netrc of n machines, requesting the
// full node[0-n-1] range with the BMC suffix applied.
func benchContext(b *testing.B, n int) *cli.Context {
	b.Helper()

	set := flag.NewFlagSet("bench", flag.ContinueOnError)
	nodes := cli.NewStringSlice()
	set.Var(nodes, "node", "")
	set.String("netrc", benchNetrc(b, n), "")
	set.Bool("insecure", true, "")
	set.Int64("timeout", 5000, "")
	set.Int("retry", 1, "")
	set.String("trim-suffix", "", "")
	set.String("suffix", "-bmc", "")
	_ = nodes.Set("node[0-" + strconv.Itoa(n-1) + "]")

	return cli.NewContext(&cli.App{Name: "bench", Writer: io.Discard}, set, nil)
}

// BenchmarkEachBMC measures node expansion, BMC name derivation, and netrc
// lookup across a cluster-sized node list. The callback is a no-op so the
// benchmark isolates the dispatch path rather than any HTTP work.
func BenchmarkEachBMC(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			ctx := benchContext(b, n)
			noop := func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
				fasthttp.ReleaseURI(u)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := eachBMC(ctx, noop); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBMCHostname(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bmcHostname("x1000c0s0b0n0", "n0", "-bmc")
	}
}

func BenchmarkPrintActionResponse(b *testing.B) {
	var p fastjson.Parser
	vals, err := p.Parse(`{"@Message.ExtendedInfo":[{"MessageSeverity":"OK","Message":"Request completed successfully."}]}`)
	if err != nil {
		b.Fatal(err)
	}
	host := []byte("x1000c0s0b0-bmc")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		printActionResponse(io.Discard, host, 200, vals)
	}
}

// benchRedfish starts a stub Redfish endpoint for the HTTP-path benchmarks.
func benchRedfish(b *testing.B, handlers map[string]http.HandlerFunc) *httptest.Server {
	b.Helper()
	mux := http.NewServeMux()
	for path, h := range handlers {
		mux.HandleFunc(path, h)
	}
	srv := httptest.NewTLSServer(mux)
	b.Cleanup(srv.Close)
	return srv
}

func benchJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// BenchmarkGetPowerState measures a full status query: Systems collection
// fetch, per-system fetch, and reporting.
func BenchmarkGetPowerState(b *testing.B) {
	srv := benchRedfish(b, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/":  benchJSON(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": benchJSON(`{"PowerState":"On"}`),
	})
	host := srv.Listener.Addr().String()
	client := newBenchClient()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u := fasthttp.AcquireURI()
		u.SetUsername("root")
		u.SetPassword("calvin")
		u.SetScheme("https")
		u.SetHost(host)
		getPowerState(client, u, io.Discard, "")
	}
}

// BenchmarkPostManagerReset measures a full BMC reset: Managers collection
// fetch, manager fetch, allow-list check, and the action POST.
func BenchmarkPostManagerReset(b *testing.B) {
	const target = "/redfish/v1/Managers/bmc/Actions/Manager.Reset"
	srv := benchRedfish(b, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/": benchJSON(`{"Members":[{"@odata.id":"/redfish/v1/Managers/bmc"}]}`),
		"/redfish/v1/Managers/bmc": benchJSON(`{"Actions":{"#Manager.Reset":{
			"target":"` + target + `","ResetType@Redfish.AllowableValues":["GracefulRestart"]}}}`),
		target: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) },
	})
	host := srv.Listener.Addr().String()
	client := newBenchClient()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u := fasthttp.AcquireURI()
		u.SetUsername("root")
		u.SetPassword("calvin")
		u.SetScheme("https")
		u.SetHost(host)
		postManagerReset(client, u, io.Discard, "GracefulRestart")
	}
}

// BenchmarkDoGetRequest isolates a single GET plus JSON parse.
func BenchmarkDoGetRequest(b *testing.B) {
	srv := benchRedfish(b, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/1": benchJSON(`{"PowerState":"On"}`),
	})
	host := srv.Listener.Addr().String()
	client := newBenchClient()

	u := fasthttp.AcquireURI()
	defer fasthttp.ReleaseURI(u)
	u.SetUsername("root")
	u.SetPassword("calvin")
	u.SetScheme("https")
	u.SetHost(host)
	u.SetPath("/redfish/v1/Systems/1")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := doGetRequest(client, u); err != nil {
			b.Fatal(err)
		}
	}
}

func newBenchClient() *fasthttp.Client {
	return &fasthttp.Client{
		ReadTimeout:              5 * time.Second,
		WriteTimeout:             5 * time.Second,
		NoDefaultUserAgentHeader: true,
		TLSConfig:                &tls.Config{InsecureSkipVerify: true},
	}
}
