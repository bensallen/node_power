package node

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

// systemsHandlers returns a Redfish stub exposing one system with the given
// power state.
func systemsHandlers(state string) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/redfish/v1/Systems/":  jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"PowerState":"` + state + `"}`),
	}
}

func TestStatusReportsPowerState(t *testing.T) {
	rf := newRedfish(t, systemsHandlers("On"))
	netrc := writeNetrc(t, rf.host())
	ctx, buf := newTestContext(t, netrc, withNodes(rf.host()))

	if err := Status(ctx); err != nil {
		t.Fatalf("Status: %v", err)
	}
	want := rf.host() + ": On\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestStatusRejectsUnknownStateFilter(t *testing.T) {
	ctx, _ := newTestContext(t, "/unused", withString("state", "Bogus"))

	err := Status(ctx)
	if err == nil {
		t.Fatal("expected an error for an invalid --state")
	}
	if !strings.Contains(err.Error(), "[On|Off|Failed]") {
		t.Errorf("error = %q, want it to list the valid states", err)
	}
}

func TestStatusAcceptsValidStateFilters(t *testing.T) {
	for _, state := range []string{"", "On", "Off", "Failed"} {
		t.Run("state="+state, func(t *testing.T) {
			rf := newRedfish(t, systemsHandlers("On"))
			netrc := writeNetrc(t, rf.host())
			ctx, _ := newTestContext(t, netrc,
				withNodes(rf.host()),
				withString("state", state),
			)
			if err := Status(ctx); err != nil {
				t.Fatalf("Status with --state %q: %v", state, err)
			}
		})
	}
}

// A filter suppresses hosts whose state does not match.
func TestStatusFilterSuppressesNonMatching(t *testing.T) {
	rf := newRedfish(t, systemsHandlers("On"))
	netrc := writeNetrc(t, rf.host())
	ctx, buf := newTestContext(t, netrc,
		withNodes(rf.host()),
		withString("state", "Off"),
	)

	if err := Status(ctx); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("output = %q, want nothing for a non-matching filter", buf.String())
	}
}

func TestStatusPropagatesNetrcError(t *testing.T) {
	ctx, _ := newTestContext(t, "/nonexistent/netrc", withNodes("bmc01"))
	if err := Status(ctx); err == nil {
		t.Fatal("expected an error for a missing netrc")
	}
}

func TestGetPowerStateSystemsError(t *testing.T) {
	tests := []struct {
		name       string
		filter     string
		wantOutput bool
	}{
		{name: "unfiltered reports failure", filter: "", wantOutput: true},
		{name: "Failed filter reports failure", filter: "Failed", wantOutput: true},
		{name: "On filter suppresses failure", filter: "On", wantOutput: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := uriFor(deadHost(t))
			buf := &bytes.Buffer{}

			getPowerState(newTestClient(), u, buf, tt.filter)

			if got := buf.Len() > 0; got != tt.wantOutput {
				t.Errorf("produced output = %v, want %v (output %q)", got, tt.wantOutput, buf.String())
			}
		})
	}
}

// A failure fetching an individual system is reported subject to the filter.
func TestGetPowerStateSystemFetchError(t *testing.T) {
	for _, tt := range []struct {
		name       string
		filter     string
		wantOutput bool
	}{
		{name: "unfiltered", filter: "", wantOutput: true},
		{name: "Failed filter", filter: "Failed", wantOutput: true},
		{name: "On filter", filter: "On", wantOutput: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rf := newRedfish(t, map[string]http.HandlerFunc{
				"/redfish/v1/Systems/":  jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
				"/redfish/v1/Systems/1": raw(http.StatusOK, "not json"),
			})

			u := uriFor(rf.host())
			buf := &bytes.Buffer{}

			getPowerState(newTestClient(), u, buf, tt.filter)

			if got := buf.Len() > 0; got != tt.wantOutput {
				t.Errorf("produced output = %v, want %v (output %q)", got, tt.wantOutput, buf.String())
			}
		})
	}
}

// Every system advertised by a BMC is reported.
func TestGetPowerStateMultipleSystems(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/": jsonBody(`{"Members":[
			{"@odata.id":"/redfish/v1/Systems/1"},
			{"@odata.id":"/redfish/v1/Systems/2"}
		]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"PowerState":"On"}`),
		"/redfish/v1/Systems/2": jsonBody(`{"PowerState":"Off"}`),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}

	getPowerState(newTestClient(), u, buf, "")

	out := buf.String()
	if !strings.Contains(out, ": On\n") || !strings.Contains(out, ": Off\n") {
		t.Errorf("output = %q, want both systems reported", out)
	}
}
