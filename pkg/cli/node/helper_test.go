package node

import (
	"bytes"
	"crypto/tls"
	"flag"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
)

// redfish is a stub Redfish endpoint. Handlers are keyed by request path;
// any path without a handler returns 404.
type redfish struct {
	t        *testing.T
	server   *httptest.Server
	mu       sync.Mutex
	requests []recorded
}

type recorded struct {
	method      string
	path        string
	body        string
	contentType string
	user, pass  string
}

// newRedfish starts a TLS stub serving the supplied handlers.
func newRedfish(t *testing.T, handlers map[string]http.HandlerFunc) *redfish {
	t.Helper()

	rf := &redfish{t: t}
	mux := http.NewServeMux()
	for path, h := range handlers {
		handler := h
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			body, _ := readAll(r)
			user, pass, _ := r.BasicAuth()
			rf.mu.Lock()
			rf.requests = append(rf.requests, recorded{
				method:      r.Method,
				path:        r.URL.Path,
				body:        body,
				contentType: r.Header.Get("Content-Type"),
				user:        user,
				pass:        pass,
			})
			rf.mu.Unlock()
			handler(w, r)
		})
	}

	rf.server = httptest.NewTLSServer(mux)
	t.Cleanup(rf.server.Close)
	return rf
}

func readAll(r *http.Request) (string, error) {
	if r.Body == nil {
		return "", nil
	}
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r.Body)
	return buf.String(), err
}

// host returns the host:port the stub is listening on.
func (rf *redfish) host() string {
	return rf.server.Listener.Addr().String()
}

// recordedRequests returns a copy of the requests seen so far.
func (rf *redfish) recordedRequests() []recorded {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	out := make([]recorded, len(rf.requests))
	copy(out, rf.requests)
	return out
}

// json responds with a 200 and the given JSON body.
func jsonBody(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

// status responds with the given status code and an empty body.
func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
	}
}

// raw responds with a non-JSON body, to exercise unmarshal failures.
func raw(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

// writeNetrc writes a netrc granting the given credentials for each machine
// and returns its path.
func writeNetrc(t *testing.T, machines ...string) string {
	t.Helper()
	var b strings.Builder
	for _, m := range machines {
		b.WriteString("machine " + m + " login root password calvin\n")
	}
	path := filepath.Join(t.TempDir(), "netrc")
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatalf("writing netrc: %v", err)
	}
	return path
}

// ctxOpt customises the cli.Context built by newTestContext.
type ctxOpt func(*flag.FlagSet, *cli.StringSlice)

func withNodes(nodes ...string) ctxOpt {
	return func(_ *flag.FlagSet, ss *cli.StringSlice) {
		for _, n := range nodes {
			_ = ss.Set(n)
		}
	}
}

func withString(name, value string) ctxOpt {
	return func(set *flag.FlagSet, _ *cli.StringSlice) {
		if set.Lookup(name) == nil {
			set.String(name, "", "")
		}
		_ = set.Set(name, value)
	}
}

// newTestContext builds a cli.Context carrying the global flags the node
// package reads, with output directed at the returned buffer.
func newTestContext(t *testing.T, netrcPath string, opts ...ctxOpt) (*cli.Context, *bytes.Buffer) {
	t.Helper()

	set := flag.NewFlagSet("test", flag.ContinueOnError)
	nodes := cli.NewStringSlice()
	set.Var(nodes, "node", "")
	set.String("netrc", netrcPath, "")
	set.Bool("insecure", true, "")
	set.Int64("timeout", 5000, "")
	set.Int("retry", 1, "")
	set.Int("max-concurrent", 0, "")
	set.String("trim-suffix", "", "")
	set.String("suffix", "", "")
	set.String("command", "", "")
	set.String("bmc-command", "", "")
	set.String("state", "", "")

	for _, opt := range opts {
		opt(set, nodes)
	}

	buf := &bytes.Buffer{}
	app := &cli.App{Name: "test", Writer: buf}
	return cli.NewContext(app, set, nil), buf
}

// newTestClient returns a fasthttp client that trusts the stub's certificate.
func newTestClient() *fasthttp.Client {
	return &fasthttp.Client{
		ReadTimeout:              5 * time.Second,
		WriteTimeout:             5 * time.Second,
		NoDefaultUserAgentHeader: true,
		TLSConfig:                &tls.Config{InsecureSkipVerify: true},
	}
}

// uriFor builds an authenticated URI pointing at the given host.
func uriFor(host string) *fasthttp.URI {
	u := fasthttp.AcquireURI()
	u.SetUsername("root")
	u.SetPassword("calvin")
	u.SetScheme("https")
	u.SetHost(host)
	return u
}

// deadHost returns a host:port that refuses connections, for exercising
// transport error paths.
func deadHost(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("closing listener: %v", err)
	}
	return addr
}

// newBareFlagSet returns an empty flag set, for contexts where no flags are
// read.
func newBareFlagSet() *flag.FlagSet {
	return flag.NewFlagSet("test", flag.ContinueOnError)
}
