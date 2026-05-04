package main

import (
	"testing"
)

func TestFileSeeds(t *testing.T) {
	pkgs := map[string]*packageInfo{
		"example.com/mod": {
			ImportPath: "example.com/mod",
			Dir:        "/src/mod",
		},
		"example.com/mod/pkg/foo": {
			ImportPath: "example.com/mod/pkg/foo",
			Dir:        "/src/mod/pkg/foo",
		},
		"example.com/mod/pkg/bar": {
			ImportPath: "example.com/mod/pkg/bar",
			Dir:        "/src/mod/pkg/bar",
		},
	}

	tests := []struct {
		name    string
		changed map[string][]string
		want    map[string]bool
	}{
		{
			name: "direct package directory",
			changed: map[string][]string{
				"pkg/foo": {"pkg/foo/handler.go"},
			},
			want: map[string]bool{
				"example.com/mod/pkg/foo": true,
			},
		},
		{
			name: "testdata subdirectory resolves to parent package",
			changed: map[string][]string{
				"pkg/foo/testdata": {"pkg/foo/testdata/input.json"},
			},
			want: map[string]bool{
				"example.com/mod/pkg/foo": true,
			},
		},
		{
			name: "nested testdata resolves to nearest package",
			changed: map[string][]string{
				"pkg/bar/testdata/deep/nested": {"pkg/bar/testdata/deep/nested/fixture.yaml"},
			},
			want: map[string]bool{
				"example.com/mod/pkg/bar": true,
			},
		},
		{
			name: "module root file",
			changed: map[string][]string{
				".": {"README.md"},
			},
			want: map[string]bool{
				"example.com/mod": true,
			},
		},
		{
			name: "multiple directories",
			changed: map[string][]string{
				"pkg/foo": {"pkg/foo/a.go"},
				"pkg/bar": {"pkg/bar/b.go"},
			},
			want: map[string]bool{
				"example.com/mod/pkg/foo": true,
				"example.com/mod/pkg/bar": true,
			},
		},
		{
			name:    "no changed files",
			changed: map[string][]string{},
			want:    map[string]bool{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := fileSeeds(test.changed, pkgs, "example.com/mod")
			if len(got) != len(test.want) {
				t.Errorf("got %d seeds, want %d: got=%v", len(got), len(test.want), got)
			}
			for p := range test.want {
				if !got[p] {
					t.Errorf("missing seed %s", p)
				}
			}
		})
	}
}

func TestResolvePackage(t *testing.T) {
	knownPkgs := map[string]bool{
		"example.com/mod":         true,
		"example.com/mod/pkg/foo": true,
	}

	tests := []struct {
		dir  string
		want string
	}{
		{"pkg/foo", "example.com/mod/pkg/foo"},
		{"pkg/foo/testdata", "example.com/mod/pkg/foo"},
		{"pkg/foo/testdata/deep", "example.com/mod/pkg/foo"},
		{".", "example.com/mod"},
		{"nonexistent/deep/path", ""},
	}

	for _, test := range tests {
		t.Run(test.dir, func(t *testing.T) {
			got := resolvePackage(test.dir, knownPkgs, "example.com/mod")
			if got != test.want {
				t.Errorf("resolvePackage(%q) = %q, want %q", test.dir, got, test.want)
			}
		})
	}
}

func TestToRelative(t *testing.T) {
	tests := []struct {
		importPath string
		modPath    string
		want       string
	}{
		{"example.com/mod/pkg/foo", "example.com/mod", "./pkg/foo"},
		{"example.com/mod", "example.com/mod", "."},
		{"other.com/pkg", "example.com/mod", "other.com/pkg"},
	}

	for _, test := range tests {
		t.Run(test.importPath, func(t *testing.T) {
			got := toRelative(test.importPath, test.modPath)
			if got != test.want {
				t.Errorf("toRelative(%q, %q) = %q, want %q", test.importPath, test.modPath, got, test.want)
			}
		})
	}
}
