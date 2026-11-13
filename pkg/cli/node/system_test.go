package node

import (
	"net/http"
	"strings"
	"testing"

	"github.com/valyala/fasthttp"
)

func TestGetSystems(t *testing.T) {
	rf := newRedfish(t, map[string]http.HandlerFunc{
		"/redfish/v1/Systems/": jsonBody(`{"Members":[
			{"@odata.id":"/redfish/v1/Systems/1"},
			{"@odata.id":"/redfish/v1/Systems/2"}
		]}`),
	})

	u := uriFor(rf.host())
	defer fasthttp.ReleaseURI(u)

	links, err := getSystems(newTestClient(), u)
	if err != nil {
		t.Fatalf("getSystems: %v", err)
	}
	want := []string{"/redfish/v1/Systems/1", "/redfish/v1/Systems/2"}
	if len(links) != len(want) {
		t.Fatalf("got %d links, want %d", len(links), len(want))
	}
	for i, w := range want {
		if string(links[i]) != w {
			t.Errorf("link[%d] = %q, want %q", i, links[i], w)
		}
	}
}

func TestGetSystemsRequestError(t *testing.T) {
	u := uriFor(deadHost(t))
	defer fasthttp.ReleaseURI(u)

	if _, err := getSystems(newTestClient(), u); err == nil {
		t.Fatal("expected an error from an unreachable host")
	}
}

func TestGetSystemsNoMembers(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "members absent", body: `{}`},
		{name: "members empty", body: `{"Members":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rf := newRedfish(t, map[string]http.HandlerFunc{
				"/redfish/v1/Systems/": jsonBody(tc.body),
			})

			u := uriFor(rf.host())
			defer fasthttp.ReleaseURI(u)

			_, err := getSystems(newTestClient(), u)
			if err == nil {
				t.Fatal("expected an error when no members are returned")
			}
			if !strings.Contains(err.Error(), "members not found") {
				t.Errorf("error = %q, want it to mention missing members", err)
			}
		})
	}
}
