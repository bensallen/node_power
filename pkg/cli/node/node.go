package node

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/bensallen/nodeset"
	"github.com/bgentry/go-netrc/netrc"
	"github.com/urfave/cli/v2"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fastjson"
)

// syncWriter serializes writes from the per-node goroutines so that lines
// from concurrent hosts are not interleaved.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// newOutput returns the writer command results are reported on, wrapped so it
// is safe for concurrent use. It honours the app's configured writer, which
// lets tests capture output.
func newOutput(c *cli.Context) io.Writer {
	var w io.Writer = os.Stdout
	if c != nil && c.App != nil && c.App.Writer != nil {
		w = c.App.Writer
	}
	return &syncWriter{w: w}
}

// bmcHostname derives a BMC hostname from a node name by first trimming
// trimSuffix from the end of the name and then appending suffix. For example
// node01n0 with trimSuffix "n0" yields node01, and node01 with suffix "-bmc"
// yields node01-bmc.
func bmcHostname(node, trimSuffix, suffix string) string {
	if trimSuffix != "" {
		node = strings.TrimSuffix(node, trimSuffix)
	}
	return node + suffix
}

// eachBMC expands the --node patterns, maps every node to its BMC hostname,
// resolves credentials from netrc, and runs fn concurrently once per BMC. The
// URI handed to fn is owned by fn, which is responsible for releasing it.
func eachBMC(c *cli.Context, fn func(client *fasthttp.Client, u *fasthttp.URI, out io.Writer)) error {
	nr, err := netrc.ParseFile(c.String("netrc"))
	if err != nil {
		return err
	}

	out := newOutput(c)

	trimSuffix := c.String("trim-suffix")
	suffix := c.String("suffix")

	client := newClient(c)

	// A nil channel means unlimited; otherwise it caps in-flight requests.
	var sem chan struct{}
	if n := c.Int("max-concurrent"); n > 0 {
		sem = make(chan struct{}, n)
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	iter := func(s string) error {
		host := bmcHostname(s, trimSuffix, suffix)

		machine := nr.FindMachine(host)
		if machine == nil {
			return fmt.Errorf("node %s not found in netrc", host)
		}

		u := fasthttp.AcquireURI()
		u.SetUsername(machine.Login)
		u.SetPassword(machine.Password)
		u.SetScheme("https")
		u.SetHost(host)

		if sem != nil {
			sem <- struct{}{}
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}
			fn(client, u, out)
		}()

		return nil
	}

	for _, node := range c.StringSlice("node") {
		for _, split := range nodeset.SplitOnComma(node) {
			if err := nodeset.Expand(split, iter); err != nil {
				return err
			}
		}
	}

	return nil
}

// printActionResponse reports the outcome of a Redfish action POST. Responses
// carry an @Message.ExtendedInfo array either at the top level or nested under
// error; when neither is present the HTTP status is reported instead.
func printActionResponse(out io.Writer, host []byte, status int, vals *fastjson.Value) {
	var msgExtendedInfo []*fastjson.Value
	if vals != nil {
		msgExtendedInfo = vals.GetArray("@Message.ExtendedInfo")
		if len(msgExtendedInfo) == 0 {
			msgExtendedInfo = vals.GetArray("error", "@Message.ExtendedInfo")
		}
	}

	if len(msgExtendedInfo) == 0 {
		if status >= 200 && status < 300 {
			fmt.Fprintf(out, "%s: OK\n", host)
		} else {
			fmt.Fprintf(out, "%s: HTTP %d\n", host, status)
		}
		return
	}

	for _, msgVal := range msgExtendedInfo {
		severity := msgVal.GetStringBytes("MessageSeverity")
		if len(severity) == 0 {
			severity = msgVal.GetStringBytes("Severity")
		}
		message := msgVal.GetStringBytes("Message")
		if len(message) != 0 {
			fmt.Fprintf(out, "%s: %s - %s\n", host, severity, message)
		} else {
			fmt.Fprintf(out, "%s: %s\n", host, severity)
		}
	}
}
