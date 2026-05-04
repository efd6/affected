package main

import (
	"testing"

	"golang.org/x/mod/modfile"
)

func mustParse(t *testing.T, content string) *modfile.File {
	t.Helper()
	f, err := modfile.Parse("go.mod", []byte(content), nil)
	if err != nil {
		t.Fatalf("parsing go.mod: %v", err)
	}
	return f
}

func TestDiffModules(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
		want map[string]bool
	}{
		{
			name: "version bump",
			old: `module example.com/mod
go 1.21
require github.com/foo/bar v1.0.0
`,
			new: `module example.com/mod
go 1.21
require github.com/foo/bar v1.1.0
`,
			want: map[string]bool{"github.com/foo/bar": true},
		},
		{
			name: "added dependency",
			old: `module example.com/mod
go 1.21
`,
			new: `module example.com/mod
go 1.21
require github.com/new/dep v0.1.0
`,
			want: map[string]bool{"github.com/new/dep": true},
		},
		{
			name: "removed dependency",
			old: `module example.com/mod
go 1.21
require github.com/old/dep v0.1.0
`,
			new: `module example.com/mod
go 1.21
`,
			want: map[string]bool{"github.com/old/dep": true},
		},
		{
			name: "replace directive changed",
			old: `module example.com/mod
go 1.21
require github.com/foo/bar v1.0.0
replace github.com/foo/bar => github.com/fork/bar v1.0.0
`,
			new: `module example.com/mod
go 1.21
require github.com/foo/bar v1.0.0
replace github.com/foo/bar => github.com/fork/bar v1.1.0
`,
			want: map[string]bool{"github.com/foo/bar": true},
		},
		{
			name: "no changes",
			old: `module example.com/mod
go 1.21
require github.com/foo/bar v1.0.0
`,
			new: `module example.com/mod
go 1.21
require github.com/foo/bar v1.0.0
`,
			want: map[string]bool{},
		},
		{
			name: "multiple changes",
			old: `module example.com/mod
go 1.21
require (
	github.com/foo/bar v1.0.0
	github.com/baz/qux v0.2.0
)
`,
			new: `module example.com/mod
go 1.21
require (
	github.com/foo/bar v1.1.0
	github.com/baz/qux v0.3.0
)
`,
			want: map[string]bool{
				"github.com/foo/bar": true,
				"github.com/baz/qux": true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			oldMod := mustParse(t, test.old)
			newMod := mustParse(t, test.new)
			got := diffModules(oldMod, newMod)
			if len(got) != len(test.want) {
				t.Errorf("got %d changed modules, want %d: got=%v", len(got), len(test.want), got)
			}
			for mod := range test.want {
				if !got[mod] {
					t.Errorf("missing changed module %s", mod)
				}
			}
		})
	}
}

func TestPackagesImporting(t *testing.T) {
	pkgs := map[string]*packageInfo{
		"example.com/mod/a": {
			ImportPath: "example.com/mod/a",
			Deps:       []string{"github.com/foo/bar", "github.com/foo/bar/sub"},
			Imports:    []string{"github.com/foo/bar"},
		},
		"example.com/mod/b": {
			ImportPath:  "example.com/mod/b",
			Deps:        []string{"github.com/baz/qux"},
			TestImports: []string{"github.com/baz/qux"},
		},
		"example.com/mod/c": {
			ImportPath: "example.com/mod/c",
			Deps:       []string{},
		},
	}

	tests := []struct {
		name    string
		changed map[string]bool
		want    map[string]bool
	}{
		{
			name:    "direct dep changed",
			changed: map[string]bool{"github.com/foo/bar": true},
			want:    map[string]bool{"example.com/mod/a": true},
		},
		{
			name:    "test dep changed",
			changed: map[string]bool{"github.com/baz/qux": true},
			want:    map[string]bool{"example.com/mod/b": true},
		},
		{
			name:    "unrelated module",
			changed: map[string]bool{"github.com/unrelated/pkg": true},
			want:    map[string]bool{},
		},
		{
			name:    "multiple modules changed",
			changed: map[string]bool{"github.com/foo/bar": true, "github.com/baz/qux": true},
			want:    map[string]bool{"example.com/mod/a": true, "example.com/mod/b": true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := packagesImporting(test.changed, pkgs)
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
