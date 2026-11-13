package docs

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// newContext builds a context whose --output points at path, for an app with
// enough structure to render documentation.
func newContext(t *testing.T, path string) *cli.Context {
	t.Helper()

	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.String("output", path, "")

	app := &cli.App{
		Name:  "nodepower",
		Usage: "CLI Interface to control power",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "netrc", Usage: "netrc path"},
		},
		Commands: []*cli.Command{
			{Name: "Status", Usage: "Get power status"},
		},
	}
	return cli.NewContext(app, set, nil)
}

func TestManual(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodepower.1")

	if err := Manual(newContext(t, path)); err != nil {
		t.Fatalf("Manual: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("man page is empty")
	}
	if !strings.Contains(string(got), "nodepower") {
		t.Errorf("man page does not mention the app name:\n%s", got)
	}
}

func TestManualUnwritablePath(t *testing.T) {
	// A path inside a nonexistent directory cannot be created.
	path := filepath.Join(t.TempDir(), "missing", "nodepower.1")

	if err := Manual(newContext(t, path)); err == nil {
		t.Fatal("expected an error for an uncreatable output path")
	}
}

func TestReadme(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")

	if err := Readme(newContext(t, path)); err != nil {
		t.Fatalf("Readme: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("readme is empty")
	}
	if !strings.Contains(string(got), "nodepower") {
		t.Errorf("readme does not mention the app name:\n%s", got)
	}
}

func TestReadmeUnwritablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "README.md")

	if err := Readme(newContext(t, path)); err == nil {
		t.Fatal("expected an error for an uncreatable output path")
	}
}
