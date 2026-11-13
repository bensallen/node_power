package node

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
)

func TestNewClient(t *testing.T) {
	ctx, _ := newTestContext(t, "/does/not/matter")
	if err := ctx.Set("timeout", "250"); err != nil {
		t.Fatalf("setting timeout: %v", err)
	}
	if err := ctx.Set("retry", "7"); err != nil {
		t.Fatalf("setting retry: %v", err)
	}

	c := newClient(ctx)

	if c.ReadTimeout != 250*time.Millisecond {
		t.Errorf("ReadTimeout = %v, want 250ms", c.ReadTimeout)
	}
	if c.WriteTimeout != 250*time.Millisecond {
		t.Errorf("WriteTimeout = %v, want 250ms", c.WriteTimeout)
	}
	if c.MaxIdemponentCallAttempts != 7 {
		t.Errorf("MaxIdemponentCallAttempts = %d, want 7", c.MaxIdemponentCallAttempts)
	}
	if !c.NoDefaultUserAgentHeader {
		t.Error("NoDefaultUserAgentHeader = false, want true")
	}
	if c.TLSConfig == nil || !c.TLSConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify not propagated from --insecure")
	}
	if c.Dial == nil {
		t.Error("Dial not configured")
	}
}

func TestNewClientSecureByDefault(t *testing.T) {
	ctx, _ := newTestContext(t, "/does/not/matter")
	if err := ctx.Set("insecure", "false"); err != nil {
		t.Fatalf("setting insecure: %v", err)
	}

	c := newClient(ctx)
	if c.TLSConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = true, want false when --insecure is not set")
	}
}

func TestDoGetRequest(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/ok": jsonBody(`{"PowerState":"On"}`),
	})

	u := uriFor(rf.host())
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/ok")

	v, err := doGetRequest(newTestClient(), u)
	if err != nil {
		t.Fatalf("doGetRequest: %v", err)
	}
	if got := string(v.GetStringBytes("PowerState")); got != "On" {
		t.Errorf("PowerState = %q, want On", got)
	}

	reqs := rf.recordedRequests()
	if len(reqs) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(reqs))
	}
	if reqs[0].method != http.MethodGet {
		t.Errorf("method = %s, want GET", reqs[0].method)
	}
	if reqs[0].user != "root" || reqs[0].pass != "calvin" {
		t.Errorf("basic auth = %s/%s, want root/calvin", reqs[0].user, reqs[0].pass)
	}
}

func TestDoGetRequestConnectionError(t *testing.T) {
	u := uriFor(deadHost(t))
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/anything")

	_, err := doGetRequest(newTestClient(), u)
	if err == nil {
		t.Fatal("expected a connection error, got nil")
	}
	if !strings.Contains(err.Error(), "connection error") {
		t.Errorf("error = %q, want it to mention a connection error", err)
	}
}

func TestDoGetRequestInvalidJSON(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/bad": raw(http.StatusOK, "this is not json"),
	})

	u := uriFor(rf.host())
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/bad")

	_, err := doGetRequest(newTestClient(), u)
	if err == nil {
		t.Fatal("expected an unmarshal error, got nil")
	}
	if !strings.Contains(err.Error(), "error unmarshaling JSON body") {
		t.Errorf("error = %q, want it to mention unmarshaling", err)
	}
}

func TestDoPostRequest(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/act": jsonBody(`{"ok":true}`),
	})

	u := uriFor(rf.host())
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/act")

	code, v, err := doPostRequest(newTestClient(), u, []byte(`{"ResetType":"On"}`))
	if err != nil {
		t.Fatalf("doPostRequest: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if v == nil || !v.GetBool("ok") {
		t.Error("response body not parsed")
	}

	reqs := rf.recordedRequests()
	if len(reqs) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(reqs))
	}
	if reqs[0].method != http.MethodPost {
		t.Errorf("method = %s, want POST", reqs[0].method)
	}
	if reqs[0].contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", reqs[0].contentType)
	}
	if reqs[0].body != `{"ResetType":"On"}` {
		t.Errorf("body = %q, want the reset payload", reqs[0].body)
	}
}

// A 204 with no body is the common reply to a Redfish action and must not be
// reported as a JSON unmarshal failure.
func TestDoPostRequestNoContent(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/act": status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/act")

	code, v, err := doPostRequest(newTestClient(), u, []byte(`{}`))
	if err != nil {
		t.Fatalf("doPostRequest returned an error for an empty body: %v", err)
	}
	if code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", code)
	}
	if v != nil {
		t.Errorf("value = %v, want nil for an empty body", v)
	}
}

// A body of only whitespace is treated the same as an empty body.
func TestDoPostRequestWhitespaceBody(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/act": raw(http.StatusOK, "   \n\t "),
	})

	u := uriFor(rf.host())
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/act")

	code, v, err := doPostRequest(newTestClient(), u, []byte(`{}`))
	if err != nil {
		t.Fatalf("doPostRequest: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if v != nil {
		t.Errorf("value = %v, want nil for a whitespace-only body", v)
	}
}

func TestDoPostRequestConnectionError(t *testing.T) {
	u := uriFor(deadHost(t))
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/act")

	code, _, err := doPostRequest(newTestClient(), u, []byte(`{}`))
	if err == nil {
		t.Fatal("expected a connection error, got nil")
	}
	if code != 0 {
		t.Errorf("status = %d, want 0 on a transport failure", code)
	}
}

// A malformed body still yields the status code, so callers can report it.
func TestDoPostRequestInvalidJSON(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/act": raw(http.StatusInternalServerError, "<html>error</html>"),
	})

	u := uriFor(rf.host())
	defer fasthttp.ReleaseURI(u)
	u.SetPath("/act")

	code, v, err := doPostRequest(newTestClient(), u, []byte(`{}`))
	if err == nil {
		t.Fatal("expected an unmarshal error, got nil")
	}
	if code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 to still be reported", code)
	}
	if v != nil {
		t.Errorf("value = %v, want nil", v)
	}
}
