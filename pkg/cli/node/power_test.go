package node

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

const resetTarget = "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset"

// resetHandlers returns a stub exposing one system that allows the given
// reset types inline.
func resetHandlers(allowable string) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/redfish/v1/Systems/": jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"Actions":{"#ComputerSystem.Reset":{
			"target":"` + resetTarget + `",
			"ResetType@Redfish.AllowableValues":[` + allowable + `]}}}`),
		resetTarget: status(http.StatusNoContent),
	}
}

// Each exported power verb sets its command and posts the matching ResetType.
func TestPowerVerbs(t *testing.T) {
	verbs := []struct {
		name string
		fn   func(*cli.Context) error
	}{
		{name: "On", fn: On},
		{name: "Off", fn: Off},
		{name: "PushPowerButton", fn: PushPowerButton},
		{name: "GracefulRestart", fn: GracefulRestart},
		{name: "ForceOff", fn: ForceOff},
		{name: "ForceOn", fn: ForceOn},
		{name: "ForceRestart", fn: ForceRestart},
		{name: "NMI", fn: NMI},
	}

	for _, v := range verbs {
		t.Run(v.name, func(t *testing.T) {
			allowable := `"On","Off","PushPowerButton","GracefulRestart","ForceOff","ForceOn","ForceRestart","Nmi","NMI"`
			rf := newRedfish(t, resetHandlers(allowable))
			netrc := writeNetrc(t, rf.host())
			ctx, _ := newTestContext(t, netrc, withNodes(rf.host()))

			if err := v.fn(ctx); err != nil {
				t.Fatalf("%s: %v", v.name, err)
			}

			var posted string
			for _, r := range rf.recordedRequests() {
				if r.method == http.MethodPost {
					posted = r.body
				}
			}
			want := `{ "ResetType": "` + v.name + `" }`
			if posted != want {
				t.Errorf("posted %q, want %q", posted, want)
			}
		})
	}
}

func TestCommandRequiresCommandFlag(t *testing.T) {
	ctx, _ := newTestContext(t, "/unused")
	err := Command(ctx)
	if err == nil {
		t.Fatal("expected an error when --command is unset")
	}
	if !strings.Contains(err.Error(), "--command") {
		t.Errorf("error = %q, want it to mention --command", err)
	}
}

func TestCommandPropagatesNetrcError(t *testing.T) {
	ctx, _ := newTestContext(t, "/nonexistent/netrc",
		withNodes("bmc01"),
		withString("command", "On"),
	)
	if err := Command(ctx); err == nil {
		t.Fatal("expected an error for a missing netrc")
	}
}

func TestPostPowerCommandReportsExtendedInfo(t *testing.T) {
	handlers := resetHandlers(`"On"`)
	handlers[resetTarget] = jsonBody(`{"@Message.ExtendedInfo":[{"MessageSeverity":"OK","Message":"Done."}]}`)
	rf := newRedfish(t, handlers)

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "On")

	want := rf.host() + ": OK - Done.\n"
	if buf.String() != want {
		t.Errorf("output = %q, want %q", buf.String(), want)
	}
}

func TestPostPowerCommandRejectsDisallowedResetType(t *testing.T) {
	rf := newRedfish(t, resetHandlers(`"On","Off"`))

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "NMI")

	if !strings.Contains(buf.String(), "not an allowable reset value") {
		t.Errorf("output = %q, want a rejection message", buf.String())
	}
	for _, r := range rf.recordedRequests() {
		if r.method == http.MethodPost {
			t.Error("a POST was sent despite the reset type being disallowed")
		}
	}
}

// When a BMC advertises no allowable values the command is attempted anyway.
func TestPostPowerCommandNoAllowableListStillPosts(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/":  jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"Actions":{"#ComputerSystem.Reset":{"target":"` + resetTarget + `"}}}`),
		resetTarget:             status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "On")

	var posted bool
	for _, r := range rf.recordedRequests() {
		if r.method == http.MethodPost {
			posted = true
		}
	}
	if !posted {
		t.Error("no POST sent when the BMC advertises no allowable values")
	}
	if !strings.Contains(buf.String(), ": OK") {
		t.Errorf("output = %q, want a success report", buf.String())
	}
}

func TestPostPowerCommandMissingTarget(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/":  jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"Actions":{}}`),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "On")

	if !strings.Contains(buf.String(), "ComputerSystem.Reset action not supported") {
		t.Errorf("output = %q, want an unsupported-action message", buf.String())
	}
}

func TestPostPowerCommandSystemsError(t *testing.T) {
	u := uriFor(deadHost(t))
	buf := &bytes.Buffer{}

	postPowerCommand(newTestClient(), u, buf, "On")

	if buf.Len() == 0 {
		t.Error("expected an error to be reported for an unreachable host")
	}
}

func TestPostPowerCommandSystemFetchError(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/":  jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": raw(http.StatusOK, "not json"),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "On")

	if buf.Len() == 0 {
		t.Error("expected an error to be reported for an unparsable system")
	}
}

func TestPostPowerCommandPostError(t *testing.T) {
	handlers := resetHandlers(`"On"`)
	handlers[resetTarget] = raw(http.StatusInternalServerError, "<html>nope</html>")
	rf := newRedfish(t, handlers)

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "On")

	if buf.Len() == 0 {
		t.Error("expected the POST failure to be reported")
	}
}

// Allowable values may be published indirectly via @Redfish.ActionInfo.
func TestAllowableResetValuesViaActionInfo(t *testing.T) {
	const actionInfo = "/redfish/v1/Systems/1/ResetActionInfo"
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/": jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"Actions":{"#ComputerSystem.Reset":{
			"target":"` + resetTarget + `",
			"@Redfish.ActionInfo":"` + actionInfo + `"}}}`),
		actionInfo: jsonBody(`{"Parameters":[
			{"Name":"Ignored","AllowableValues":["Nope"]},
			{"Name":"ResetType","AllowableValues":["On","ForceOff"]}
		]}`),
		resetTarget: status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "ForceOff")

	if !strings.Contains(buf.String(), ": OK") {
		t.Errorf("output = %q, want the ActionInfo allow list to permit ForceOff", buf.String())
	}
}

// A disallowed value sourced from ActionInfo is still rejected.
func TestAllowableResetValuesViaActionInfoRejects(t *testing.T) {
	const actionInfo = "/redfish/v1/Systems/1/ResetActionInfo"
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/": jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"Actions":{"#ComputerSystem.Reset":{
			"target":"` + resetTarget + `",
			"@Redfish.ActionInfo":"` + actionInfo + `"}}}`),
		actionInfo: jsonBody(`{"Parameters":[{"Name":"ResetType","AllowableValues":["On"]}]}`),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "NMI")

	if !strings.Contains(buf.String(), "not an allowable reset value") {
		t.Errorf("output = %q, want a rejection", buf.String())
	}
}

// An unreachable ActionInfo URL degrades to an empty allow list rather than
// failing the command.
func TestAllowableResetValuesActionInfoFetchFails(t *testing.T) {
	const actionInfo = "/redfish/v1/Systems/1/ResetActionInfo"
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/": jsonBody(`{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"Actions":{"#ComputerSystem.Reset":{
			"target":"` + resetTarget + `",
			"@Redfish.ActionInfo":"` + actionInfo + `"}}}`),
		actionInfo:  raw(http.StatusOK, "not json"),
		resetTarget: status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "On")

	if !strings.Contains(buf.String(), ": OK") {
		t.Errorf("output = %q, want the command to proceed", buf.String())
	}
}

// Every system advertised by the BMC receives the action.
func TestPostPowerCommandMultipleSystems(t *testing.T) {
	const target2 = "/redfish/v1/Systems/2/Actions/ComputerSystem.Reset"
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/": jsonBody(`{"Members":[
			{"@odata.id":"/redfish/v1/Systems/1"},
			{"@odata.id":"/redfish/v1/Systems/2"}
		]}`),
		"/redfish/v1/Systems/1": jsonBody(`{"Actions":{"#ComputerSystem.Reset":{
			"target":"` + resetTarget + `","ResetType@Redfish.AllowableValues":["On"]}}}`),
		"/redfish/v1/Systems/2": jsonBody(`{"Actions":{"#ComputerSystem.Reset":{
			"target":"` + target2 + `","ResetType@Redfish.AllowableValues":["On"]}}}`),
		resetTarget: status(http.StatusNoContent),
		target2:     status(http.StatusNoContent),
	})

	u := uriFor(rf.host())
	buf := &bytes.Buffer{}
	postPowerCommand(newTestClient(), u, buf, "On")

	var posts int
	for _, r := range rf.recordedRequests() {
		if r.method == http.MethodPost {
			posts++
		}
	}
	if posts != 2 {
		t.Errorf("sent %d POSTs, want one per system (2)", posts)
	}
}
