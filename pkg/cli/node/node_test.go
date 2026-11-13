package node

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

func TestBMCHostname(t *testing.T) {
	tests := []struct {
		name       string
		node       string
		trimSuffix string
		suffix     string
		want       string
	}{
		{name: "no change", node: "node01", want: "node01"},
		{name: "trim", node: "node01n0", trimSuffix: "n0", want: "node01"},
		{name: "trim not present", node: "node01", trimSuffix: "n0", want: "node01"},
		{name: "trim only matches suffix", node: "n0node01", trimSuffix: "n0", want: "n0node01"},
		{name: "append", node: "node01", suffix: "-bmc", want: "node01-bmc"},
		{name: "trim then append", node: "node01n0", trimSuffix: "n0", suffix: "-bmc", want: "node01-bmc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bmcHostname(tt.node, tt.trimSuffix, tt.suffix); got != tt.want {
				t.Errorf("bmcHostname(%q, %q, %q) = %q, want %q", tt.node, tt.trimSuffix, tt.suffix, got, tt.want)
			}
		})
	}
}

func TestNewOutputDefaultsToStdout(t *testing.T) {
	// A context with no app writer falls back to os.Stdout.
	set := newBareFlagSet()
	ctx := cli.NewContext(&cli.App{Name: "test"}, set, nil)

	sw, ok := newOutput(ctx).(*syncWriter)
	if !ok {
		t.Fatal("newOutput did not return a syncWriter")
	}
	if sw.w != io.Writer(os.Stdout) {
		t.Errorf("writer = %v, want os.Stdout", sw.w)
	}
}

func TestNewOutputNilContext(t *testing.T) {
	sw, ok := newOutput(nil).(*syncWriter)
	if !ok {
		t.Fatal("newOutput did not return a syncWriter")
	}
	if sw.w != io.Writer(os.Stdout) {
		t.Errorf("writer = %v, want os.Stdout for a nil context", sw.w)
	}
}

func TestNewOutputUsesAppWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	set := newBareFlagSet()
	ctx := cli.NewContext(&cli.App{Name: "test", Writer: buf}, set, nil)

	out := newOutput(ctx)
	if _, err := out.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if buf.String() != "hello" {
		t.Errorf("buffer = %q, want hello", buf.String())
	}
}

// syncWriter must serialise concurrent writes so lines are not interleaved.
func TestSyncWriterSerialisesWrites(t *testing.T) {
	buf := &bytes.Buffer{}
	sw := &syncWriter{w: buf}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sw.Write([]byte("0123456789\n"))
		}()
	}
	wg.Wait()

	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if line != "0123456789" {
			t.Fatalf("interleaved write produced %q", line)
		}
	}
}

func TestPrintActionResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "no body, success status",
			status: http.StatusNoContent,
			want:   "bmc01: OK\n",
		},
		{
			name:   "no body, error status",
			status: http.StatusInternalServerError,
			want:   "bmc01: HTTP 500\n",
		},
		{
			name:   "body without extended info",
			status: http.StatusOK,
			body:   `{"Unrelated":"value"}`,
			want:   "bmc01: OK\n",
		},
		{
			name:   "top level extended info",
			status: http.StatusOK,
			body:   `{"@Message.ExtendedInfo":[{"MessageSeverity":"OK","Message":"Done."}]}`,
			want:   "bmc01: OK - Done.\n",
		},
		{
			name:   "nested under error",
			status: http.StatusBadRequest,
			body:   `{"error":{"@Message.ExtendedInfo":[{"MessageSeverity":"Warning","Message":"Bad."}]}}`,
			want:   "bmc01: Warning - Bad.\n",
		},
		{
			name:   "legacy Severity key",
			status: http.StatusOK,
			body:   `{"@Message.ExtendedInfo":[{"Severity":"Critical","Message":"Boom."}]}`,
			want:   "bmc01: Critical - Boom.\n",
		},
		{
			name:   "severity without message",
			status: http.StatusOK,
			body:   `{"@Message.ExtendedInfo":[{"MessageSeverity":"OK"}]}`,
			want:   "bmc01: OK\n",
		},
		{
			name:   "multiple messages each reported",
			status: http.StatusOK,
			body:   `{"@Message.ExtendedInfo":[{"MessageSeverity":"OK","Message":"One."},{"MessageSeverity":"Warning","Message":"Two."}]}`,
			want:   "bmc01: OK - One.\nbmc01: Warning - Two.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var vals *fastjson.Value
			if tt.body != "" {
				var p fastjson.Parser
				v, err := p.Parse(tt.body)
				if err != nil {
					t.Fatalf("parsing fixture: %v", err)
				}
				vals = v
			}

			buf := &bytes.Buffer{}
			printActionResponse(buf, []byte("bmc01"), tt.status, vals)
			if buf.String() != tt.want {
				t.Errorf("output = %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestEachBMCExpandsAndDerivesHostnames(t *testing.T) {
	// Two nodes via a range, each mapped to a BMC name by trim and suffix.
	netrc := writeNetrc(t, "node1-bmc", "node2-bmc")
	ctx, _ := newTestContext(t, netrc,
		withNodes("node[1-2]n0"),
		withString("trim-suffix", "n0"),
		withString("suffix", "-bmc"),
	)

	var mu sync.Mutex
	var hosts []string
	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		mu.Lock()
		hosts = append(hosts, string(u.Host()))
		mu.Unlock()
		fasthttp.ReleaseURI(u)
	})
	if err != nil {
		t.Fatalf("eachBMC: %v", err)
	}

	sort.Strings(hosts)
	want := []string{"node1-bmc", "node2-bmc"}
	if len(hosts) != len(want) {
		t.Fatalf("visited %v, want %v", hosts, want)
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("host[%d] = %q, want %q", i, hosts[i], want[i])
		}
	}
}

// Credentials from netrc must land on the URI handed to the callback.
func TestEachBMCAppliesCredentials(t *testing.T) {
	netrc := writeNetrc(t, "bmc01")
	ctx, _ := newTestContext(t, netrc, withNodes("bmc01"))

	var user, pass, scheme string
	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		user, pass, scheme = string(u.Username()), string(u.Password()), string(u.Scheme())
		fasthttp.ReleaseURI(u)
	})
	if err != nil {
		t.Fatalf("eachBMC: %v", err)
	}
	if user != "root" || pass != "calvin" {
		t.Errorf("credentials = %s/%s, want root/calvin", user, pass)
	}
	if scheme != "https" {
		t.Errorf("scheme = %q, want https", scheme)
	}
}

// Multiple --node values and comma-separated patterns both expand.
func TestEachBMCMultipleNodeFlagsAndCommas(t *testing.T) {
	netrc := writeNetrc(t, "a1", "a2", "b1", "c1")
	ctx, _ := newTestContext(t, netrc, withNodes("a[1-2],b1", "c1"))

	var mu sync.Mutex
	var hosts []string
	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		mu.Lock()
		hosts = append(hosts, string(u.Host()))
		mu.Unlock()
		fasthttp.ReleaseURI(u)
	})
	if err != nil {
		t.Fatalf("eachBMC: %v", err)
	}

	sort.Strings(hosts)
	want := []string{"a1", "a2", "b1", "c1"}
	if strings.Join(hosts, ",") != strings.Join(want, ",") {
		t.Errorf("visited %v, want %v", hosts, want)
	}
}

func TestEachBMCMissingNetrc(t *testing.T) {
	ctx, _ := newTestContext(t, "/nonexistent/netrc", withNodes("bmc01"))

	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		t.Error("callback ran despite an unreadable netrc")
		fasthttp.ReleaseURI(u)
	})
	if err == nil {
		t.Fatal("expected an error for a missing netrc")
	}
}

func TestEachBMCNodeNotInNetrc(t *testing.T) {
	netrc := writeNetrc(t, "someone-else")
	ctx, _ := newTestContext(t, netrc, withNodes("bmc01"))

	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		t.Error("callback ran for a node absent from netrc")
		fasthttp.ReleaseURI(u)
	})
	if err == nil {
		t.Fatal("expected an error for a node missing from netrc")
	}
	if !strings.Contains(err.Error(), "not found in netrc") {
		t.Errorf("error = %q, want it to mention netrc", err)
	}
}

// The derived BMC hostname, not the raw node name, is looked up in netrc.
func TestEachBMCNetrcLookupUsesDerivedName(t *testing.T) {
	netrc := writeNetrc(t, "node01")
	ctx, _ := newTestContext(t, netrc,
		withNodes("node01"),
		withString("suffix", "-bmc"),
	)

	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		t.Error("callback ran even though the derived host is absent from netrc")
		fasthttp.ReleaseURI(u)
	})
	if err == nil {
		t.Fatal("expected a lookup failure for the derived hostname")
	}
	if !strings.Contains(err.Error(), "node01-bmc") {
		t.Errorf("error = %q, want it to name the derived host", err)
	}
}

func TestEachBMCInvalidPattern(t *testing.T) {
	netrc := writeNetrc(t, "bmc01")
	ctx, _ := newTestContext(t, netrc, withNodes("node[1-"))

	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		fasthttp.ReleaseURI(u)
	})
	if err == nil {
		t.Fatal("expected an error for a malformed nodeset pattern")
	}
}

// eachBMC must wait for every goroutine before returning, including on the
// error path, so callers never see partial output.
func TestEachBMCWaitsForGoroutines(t *testing.T) {
	netrc := writeNetrc(t, "a1", "a2", "a3")
	ctx, _ := newTestContext(t, netrc, withNodes("a[1-3]"))

	var count int64
	var mu sync.Mutex
	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		mu.Lock()
		count++
		mu.Unlock()
		fasthttp.ReleaseURI(u)
	})
	if err != nil {
		t.Fatalf("eachBMC: %v", err)
	}
	if count != 3 {
		t.Errorf("callback ran %d times by the time eachBMC returned, want 3", count)
	}
}

// The writer handed to the callback is the app writer, so command output is
// capturable.
func TestEachBMCPassesAppWriter(t *testing.T) {
	netrc := writeNetrc(t, "bmc01")
	ctx, buf := newTestContext(t, netrc, withNodes("bmc01"))

	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, out io.Writer) {
		_, _ = out.Write([]byte("written by callback"))
		fasthttp.ReleaseURI(u)
	})
	if err != nil {
		t.Fatalf("eachBMC: %v", err)
	}
	if buf.String() != "written by callback" {
		t.Errorf("buffer = %q, want the callback output", buf.String())
	}
}

// --max-concurrent bounds the number of in-flight callbacks; 0 leaves it
// unbounded.
func TestEachBMCMaxConcurrent(t *testing.T) {
	const nodes = 200

	for _, tt := range []struct {
		name     string
		limit    string
		wantPeak int64
	}{
		{name: "unlimited by default", limit: "0", wantPeak: nodes},
		{name: "limited to 8", limit: "8", wantPeak: 8},
		{name: "limited to 1", limit: "1", wantPeak: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			machines := make([]string, nodes)
			for i := range machines {
				machines[i] = "node" + strconv.Itoa(i)
			}
			netrc := writeNetrc(t, machines...)
			ctx, _ := newTestContext(t, netrc,
				withNodes("node[0-"+strconv.Itoa(nodes-1)+"]"),
			)
			if err := ctx.Set("max-concurrent", tt.limit); err != nil {
				t.Fatalf("setting max-concurrent: %v", err)
			}

			var peak, live int64
			err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
				c := atomic.AddInt64(&live, 1)
				for {
					p := atomic.LoadInt64(&peak)
					if c <= p || atomic.CompareAndSwapInt64(&peak, p, c) {
						break
					}
				}
				// Hold the slot so concurrency is observable.
				time.Sleep(time.Millisecond)
				atomic.AddInt64(&live, -1)
				fasthttp.ReleaseURI(u)
			})
			if err != nil {
				t.Fatalf("eachBMC: %v", err)
			}

			if tt.wantPeak == nodes {
				// Unbounded: every node should be able to run at once.
				if peak <= 8 {
					t.Errorf("peak concurrency = %d, want it to exceed a bounded run", peak)
				}
				return
			}
			if peak > tt.wantPeak {
				t.Errorf("peak concurrency = %d, want at most %d", peak, tt.wantPeak)
			}
			if peak == 0 {
				t.Error("no callbacks ran")
			}
		})
	}
}

// Every node is still visited when a limit is in force.
func TestEachBMCMaxConcurrentVisitsAll(t *testing.T) {
	const nodes = 100
	machines := make([]string, nodes)
	for i := range machines {
		machines[i] = "node" + strconv.Itoa(i)
	}
	netrc := writeNetrc(t, machines...)
	ctx, _ := newTestContext(t, netrc, withNodes("node[0-"+strconv.Itoa(nodes-1)+"]"))
	if err := ctx.Set("max-concurrent", "4"); err != nil {
		t.Fatalf("setting max-concurrent: %v", err)
	}

	var seen int64
	err := eachBMC(ctx, func(_ *fasthttp.Client, u *fasthttp.URI, _ io.Writer) {
		atomic.AddInt64(&seen, 1)
		fasthttp.ReleaseURI(u)
	})
	if err != nil {
		t.Fatalf("eachBMC: %v", err)
	}
	if seen != nodes {
		t.Errorf("visited %d nodes, want %d", seen, nodes)
	}
}
