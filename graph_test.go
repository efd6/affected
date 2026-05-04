package main

import (
	"slices"
	"testing"
)

func TestParsePackages(t *testing.T) {
	data := []byte(`{"ImportPath":"example.com/mod/a","Dir":"/src/a","Imports":["example.com/mod/b"],"TestImports":["example.com/mod/c"],"Deps":["example.com/mod/b"],"TestGoFiles":["a_test.go"]}
{"ImportPath":"example.com/mod/b","Dir":"/src/b","Imports":[],"Deps":[],"GoFiles":["b.go"]}
{"ImportPath":"example.com/mod/c","Dir":"/src/c","Imports":["example.com/mod/b"],"Deps":["example.com/mod/b"],"GoFiles":["c.go"],"TestGoFiles":["c_test.go"]}
`)
	pkgs, err := parsePackages(data)
	if err != nil {
		t.Fatalf("parsePackages failed: %v", err)
	}
	if got := len(pkgs); got != 3 {
		t.Fatalf("parsePackages returned %d packages, want 3", got)
	}
	if pkgs["example.com/mod/a"] == nil {
		t.Fatal("missing package example.com/mod/a")
	}
	if got := pkgs["example.com/mod/a"].Dir; got != "/src/a" {
		t.Errorf("package a Dir = %q, want %q", got, "/src/a")
	}
}

func TestBuildReverseGraph(t *testing.T) {
	pkgs := map[string]*packageInfo{
		"example.com/mod/a": {
			ImportPath:  "example.com/mod/a",
			Imports:     []string{"example.com/mod/b"},
			TestImports: []string{"example.com/mod/c"},
		},
		"example.com/mod/b": {
			ImportPath: "example.com/mod/b",
			Imports:    []string{"example.com/mod/d"},
		},
		"example.com/mod/c": {
			ImportPath: "example.com/mod/c",
			Imports:    []string{"example.com/mod/b"},
		},
		"example.com/mod/d": {
			ImportPath: "example.com/mod/d",
		},
	}

	reverse := buildReverseGraph(pkgs)

	tests := []struct {
		dep       string
		importers []string
	}{
		{"example.com/mod/b", []string{"example.com/mod/a", "example.com/mod/c"}},
		{"example.com/mod/c", []string{"example.com/mod/a"}},
		{"example.com/mod/d", []string{"example.com/mod/b"}},
	}

	for _, test := range tests {
		importers := reverse[test.dep]
		if importers == nil {
			t.Errorf("no reverse edges for %s", test.dep)
			continue
		}
		for _, want := range test.importers {
			if !importers[want] {
				t.Errorf("reverse[%s] missing %s", test.dep, want)
			}
		}
		if got := len(importers); got != len(test.importers) {
			t.Errorf("reverse[%s] has %d importers, want %d", test.dep, got, len(test.importers))
		}
	}
}

func TestMergePackages(t *testing.T) {
	t.Run("new package added", func(t *testing.T) {
		dst := map[string]*packageInfo{
			"example.com/mod/a": {
				ImportPath: "example.com/mod/a",
				Imports:    []string{"example.com/mod/b"},
				Configs:    []string{"-"},
			},
		}
		incoming := map[string]*packageInfo{
			"example.com/mod/c": {
				ImportPath:  "example.com/mod/c",
				Imports:     []string{"example.com/mod/a"},
				TestGoFiles: []string{"c_test.go"},
			},
		}
		mergePackages(dst, incoming, "integration")

		c := dst["example.com/mod/c"]
		if c == nil {
			t.Fatal("package c not added to dst")
		}
		if !slices.Equal(c.Configs, []string{"integration"}) {
			t.Errorf("c.Configs = %v, want [integration]", c.Configs)
		}
	})

	t.Run("existing package gains config", func(t *testing.T) {
		dst := map[string]*packageInfo{
			"example.com/mod/a": {
				ImportPath: "example.com/mod/a",
				Imports:    []string{"example.com/mod/b"},
				Configs:    []string{"-"},
			},
		}
		incoming := map[string]*packageInfo{
			"example.com/mod/a": {
				ImportPath:  "example.com/mod/a",
				Imports:     []string{"example.com/mod/b", "example.com/mod/c"},
				TestImports: []string{"example.com/mod/d"},
			},
		}
		mergePackages(dst, incoming, "integration")

		a := dst["example.com/mod/a"]
		if !slices.Equal(a.Configs, []string{"-", "integration"}) {
			t.Errorf("a.Configs = %v, want [- integration]", a.Configs)
		}
		if !slices.Contains(a.Imports, "example.com/mod/c") {
			t.Error("a.Imports missing example.com/mod/c after merge")
		}
		if !slices.Contains(a.TestImports, "example.com/mod/d") {
			t.Error("a.TestImports missing example.com/mod/d after merge")
		}
	})

	t.Run("union deduplicates imports", func(t *testing.T) {
		dst := map[string]*packageInfo{
			"example.com/mod/a": {
				ImportPath: "example.com/mod/a",
				Imports:    []string{"example.com/mod/b", "example.com/mod/c"},
				Configs:    []string{"-"},
			},
		}
		incoming := map[string]*packageInfo{
			"example.com/mod/a": {
				ImportPath: "example.com/mod/a",
				Imports:    []string{"example.com/mod/b", "example.com/mod/d"},
			},
		}
		mergePackages(dst, incoming, "fips")

		a := dst["example.com/mod/a"]
		want := []string{"example.com/mod/b", "example.com/mod/c", "example.com/mod/d"}
		if !slices.Equal(a.Imports, want) {
			t.Errorf("a.Imports = %v, want %v", a.Imports, want)
		}
	})
}

func TestUnionStrings(t *testing.T) {
	tests := []struct {
		name string
		a, b []string
		want []string
	}{
		{
			name: "no overlap",
			a:    []string{"x", "y"},
			b:    []string{"z"},
			want: []string{"x", "y", "z"},
		},
		{
			name: "full overlap",
			a:    []string{"x", "y"},
			b:    []string{"x", "y"},
			want: []string{"x", "y"},
		},
		{
			name: "partial overlap preserves order",
			a:    []string{"a", "b", "c"},
			b:    []string{"b", "d"},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "both empty",
			a:    nil,
			b:    nil,
			want: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := unionStrings(test.a, test.b)
			if got == nil {
				got = []string{}
			}
			if !slices.Equal(got, test.want) {
				t.Errorf("unionStrings(%v, %v) = %v, want %v", test.a, test.b, got, test.want)
			}
		})
	}
}

func TestStdlibSkipped(t *testing.T) {
	pkgs := map[string]*packageInfo{
		"example.com/mod/a": {
			ImportPath: "example.com/mod/a",
			Imports:    []string{"fmt", "os", "example.com/mod/b"},
		},
		"example.com/mod/b": {
			ImportPath: "example.com/mod/b",
		},
	}

	reverse := buildReverseGraph(pkgs)

	if _, ok := reverse["fmt"]; ok {
		t.Error("stdlib package 'fmt' should not appear in reverse graph")
	}
	if _, ok := reverse["os"]; ok {
		t.Error("stdlib package 'os' should not appear in reverse graph")
	}
	if !reverse["example.com/mod/b"]["example.com/mod/a"] {
		t.Error("expected example.com/mod/a to be a reverse dep of example.com/mod/b")
	}
}
