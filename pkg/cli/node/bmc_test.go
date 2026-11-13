package node

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

const managerResetTarget = "/redfish/v1/Managers/bmc/Actions/Manager.Reset"

// managerHandlers returns a stub exposing one manager allowing the given
// reset types inline.
func managerHandlers(allowable string) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/redfish/v1/Managers/": jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Managers/bmc"}]}`),
		"/redfish/v1/Managers/bmc": jsonBody(`{"Actions":{"#Manager.Reset":{
			"target":"` + managerResetTarget + `",
			"ResetType@Redfish.AllowableValues":[` + allowable + `]}}}`),
		managerResetTarget: status(http.StatusNoContent),
	}
}

// The BMC verbs post the matching ResetType to the Manager.Reset target.
func TestBMCVerbs(t *testing.T) {
	verbs := []struct {
		name string
		fn   func(*cli.Context) error
	}{
		{name: "GracefulRestart", fn: BMCGracefulRestart},
		{name: "ForceRestart", fn: BMCForceRestart},
	}

	for _, v := range verbs {
		t.Run(v.name, func(t *testing.T) {
			rf := newRedfish(t, managerHandlers(`"GracefulRestart","ForceRestart"`))
			netrc := writeNetrc(t, rf.host())
			ctx, buf := newTestContext(t, netrc, withNodes(rf.host()))

			if err := v.fn(ctx); err != nil {
				t.Fatalf("%s: %v", v.name, err)
			}

			var posted, path string
			for _, r := range rf.recordedRequests() {
				if r.method == http.MethodPost {
					posted, path = r.body, r.path
				}
			}
			want := `{ "ResetType": "` + v.name + `" }`
			if posted != want {
				t.Errorf("posted %q, want %q", posted, want)
			}
			if path != managerResetTarget {
				t.Errorf("posted to %q, want %q", path, managerResetTarget)
			}
			if !strings.Contains(buf.String(), ": OK") {
				t.Errorf("output = %q, want a success report", buf.String())
			}
		})
	}
}

// This is the wire form of the curl command this feature replaces.
func TestBMCGracefulRestartMatchesCurlEquivalent(t *testing.T) {
	rf := newRedfish(t, managerHandlers(`"GracefulRestart"`))
	netrc := writeNetrc(t, rf.host())
	ctx, _ := newTestContext(t, netrc, withNodes(rf.host()))

	if err := BMCGracefulRestart(ctx); err != nil {
		t.Fatalf("BMCGracefulRestart: %v", err)
	}

	var post recorded
	var found bool
	for _, r := range rf.recordedRequests() {
		if r.method == http.MethodPost {
			post, found = r, true
		}
	}
	if !found {
		t.Fatal("no POST was sent")
	}
	if post.path != managerResetTarget {
		t.Errorf("path = %q, want %q", post.path, managerResetTarget)
	}
	if post.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", post.contentType)
	}
	if post.body != `{ "ResetType": "GracefulRestart" }` {
		t.Errorf("body = %q, want the GracefulRestart payload", post.body)
	}
	if post.user != "root" || post.pass != "calvin" {
		t.Errorf("auth = %s/%s, want the netrc credentials", post.user, post.pass)
	}
}

func TestBMCResetRequiresCommandFlag(t *testing.T) {
	ctx, _ := newTestContext(t, "/unused")
	err := BMCReset(ctx)
	if err == nil {
		t.Fatal("expected an error when --bmc-command is unset")
	}
	if !strings.Contains(err.Error(), "--bmc-command") {
		t.Errorf("error = %q, want it to mention --bmc-command", err)
	}
}

func TestBMCResetPropagatesNetrcError(t *testing.T) {
	ctx, _ := newTestContext(t, "/nonexistent/netrc",
		withNodes("bmc01"),
		withString("bmc-command", "GracefulRestart"),
	)
	if err := BMCReset(ctx); err == nil {
		t.Fatal("expected an error for a missing netrc")
	}
}

func TestPostManagerResetRejectsDisallowedResetType(t *testing.T) {
	rf := newRedfish(t, managerHandlers(`"GracefulRestart"`))

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "Nope")

	if !strings.Contains(buf.String(), "not an allowable reset value") {
		t.Errorf("output = %q, want a rejection message", buf.String())
	}
	for _, r := range rf.recordedRequests() {
		if r.method == http.MethodPost {
			t.Error("a POST was sent despite the reset type being disallowed")
		}
	}
}

func TestPostManagerResetNoAllowableListStillPosts(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/":    jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Managers/bmc"}]}`),
		"/redfish/v1/Managers/bmc": jsonBody(`{"Actions":{"#Manager.Reset":{"target":"` + managerResetTarget + `"}}}`),
		managerResetTarget:         status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	var posted bool
	for _, r := range rf.recordedRequests() {
		if r.method == http.MethodPost {
			posted = true
		}
	}
	if !posted {
		t.Error("no POST sent when the BMC advertises no allowable values")
	}
}

func TestPostManagerResetMissingTarget(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/":    jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Managers/bmc"}]}`),
		"/redfish/v1/Managers/bmc": jsonBody(`{"Actions":{}}`),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	if !strings.Contains(buf.String(), "Manager.Reset action not supported") {
		t.Errorf("output = %q, want an unsupported-action message", buf.String())
	}
}

func TestPostManagerResetManagersError(t *testing.T) {
	u := uriFor(deadHost(t))
	buf := &bytes.Buffer{}

	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	if buf.Len() == 0 {
		t.Error("expected an error to be reported for an unreachable host")
	}
}

func TestPostManagerResetManagerFetchError(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/":    jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Managers/bmc"}]}`),
		"/redfish/v1/Managers/bmc": raw(http.StatusOK, "not json"),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	if buf.Len() == 0 {
		t.Error("expected an error to be reported for an unparsable manager")
	}
}

func TestPostManagerResetPostError(t *testing.T) {
	handlers := managerHandlers(`"GracefulRestart"`)
	handlers[managerResetTarget] = raw(http.StatusInternalServerError, "<html>nope</html>")
	rf := newRedfish(t, handlers)

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	if buf.Len() == 0 {
		t.Error("expected the POST failure to be reported")
	}
}

// Every manager advertised by the BMC receives the action.
func TestPostManagerResetMultipleManagers(t *testing.T) {
	const target2 = "/redfish/v1/Managers/bmc2/Actions/Manager.Reset"
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/": jsonBody(`{"Members":[
			{"@odata.id":"/redfish/v1/Managers/bmc"},
			{"@odata.id":"/redfish/v1/Managers/bmc2"}
		]}`),
		"/redfish/v1/Managers/bmc": jsonBody(`{"Actions":{"#Manager.Reset":{
			"target":"` + managerResetTarget + `","ResetType@Redfish.AllowableValues":["GracefulRestart"]}}}`),
		"/redfish/v1/Managers/bmc2": jsonBody(`{"Actions":{"#Manager.Reset":{
			"target":"` + target2 + `","ResetType@Redfish.AllowableValues":["GracefulRestart"]}}}`),
		managerResetTarget: status(http.StatusNoContent),
		target2:            status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	var posts int
	for _, r := range rf.recordedRequests() {
		if r.method == http.MethodPost {
			posts++
		}
	}
	if posts != 2 {
		t.Errorf("sent %d POSTs, want one per manager (2)", posts)
	}
}

func TestGetManagers(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/": jsonBody(`{"Members":[
			{"@odata.id":"/redfish/v1/Managers/bmc"},
			{"@odata.id":"/redfish/v1/Managers/bmc2"}
		]}`),
	})

	u := uriFor(rf.host())
	links, err := getManagers(newTestClient(), u)
	if err != nil {
		t.Fatalf("getManagers: %v", err)
	}
	want := []string{"/redfish/v1/Managers/bmc", "/redfish/v1/Managers/bmc2"}
	if len(links) != len(want) {
		t.Fatalf("got %d links, want %d", len(links), len(want))
	}
	for i := range want {
		if string(links[i]) != want[i] {
			t.Errorf("link[%d] = %q, want %q", i, links[i], want[i])
		}
	}
}

func TestGetManagersRequestError(t *testing.T) {
	u := uriFor(deadHost(t))
	if _, err := getManagers(newTestClient(), u); err == nil {
		t.Fatal("expected an error from an unreachable host")
	}
}

func TestGetManagersNoMembers(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "members absent", body: `{}`},
		{name: "members empty", body: `{"Members":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rf := newRedfish(t, map[string]http.HandlerFunc{
				"/redfish/v1/Managers/": jsonBody(tc.body),
			})

			u := uriFor(rf.host())
			_, err := getManagers(newTestClient(), u)
			if err == nil {
				t.Fatal("expected an error when no managers are returned")
			}
			if !strings.Contains(err.Error(), "managers members not found") {
				t.Errorf("error = %q, want it to mention missing members", err)
			}
		})
	}
}

// Allowable values may be published indirectly via @Redfish.ActionInfo.
func TestAllowableManagerResetValuesViaActionInfo(t *testing.T) {
	const actionInfo = "/redfish/v1/Managers/bmc/ResetActionInfo"
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/": jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Managers/bmc"}]}`),
		"/redfish/v1/Managers/bmc": jsonBody(`{"Actions":{"#Manager.Reset":{
			"target":"` + managerResetTarget + `",
			"@Redfish.ActionInfo":"` + actionInfo + `"}}}`),
		actionInfo: jsonBody(`{"Parameters":[
			{"Name":"Ignored","AllowableValues":["Nope"]},
			{"Name":"ResetType","AllowableValues":["GracefulRestart"]}
		]}`),
		managerResetTarget: status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	if !strings.Contains(buf.String(), ": OK") {
		t.Errorf("output = %q, want the ActionInfo allow list to permit the reset", buf.String())
	}
}

// An unreachable ActionInfo URL degrades to an empty allow list.
func TestAllowableManagerResetValuesActionInfoFetchFails(t *testing.T) {
	const actionInfo = "/redfish/v1/Managers/bmc/ResetActionInfo"
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Managers/": jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Managers/bmc"}]}`),
		"/redfish/v1/Managers/bmc": jsonBody(`{"Actions":{"#Manager.Reset":{
			"target":"` + managerResetTarget + `",
			"@Redfish.ActionInfo":"` + actionInfo + `"}}}`),
		actionInfo:         raw(http.StatusOK, "not json"),
		managerResetTarget: status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postManagerReset(newTestClient(), u, buf, "GracefulRestart")

	if !strings.Contains(buf.String(), ": OK") {
		t.Errorf("output = %q, want the command to proceed", buf.String())
	}
}
