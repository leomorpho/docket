package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverFindsDocketReposFromGitmodulesAndNestedScan(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitmodules"), []byte(`
[submodule "goship"]
	path = goship
	url = git@github.com:example/goship.git
[submodule "control-plane"]
	path = control-plane
	url = git@github.com:example/control-plane.git
[submodule "client-a"]
	path = clients/client-a
	url = git@github.com:example/client-a.git
`), 0o644); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}

	for _, rel := range []string{"goship/.docket", "control-plane/.docket", "clients/client-a/.docket", "landing-page/.docket"} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}

	repos, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	var got []string
	for _, repo := range repos {
		rel, _ := filepath.Rel(root, repo.Path)
		got = append(got, rel)
	}

	want := []string{"clients/client-a", "control-plane", "goship", "landing-page"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() repos = %v, want %v", got, want)
	}
}
