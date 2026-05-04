package main

import (
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/mod/modfile"
)

// goModSeeds finds internal packages affected by go.mod changes
// between from and to. It parses go.mod at both refs, diffs the
// require and replace directives, and identifies internal packages
// whose transitive deps include any package from a changed module.
func goModSeeds(modRoot, from, to string, pkgs map[string]*packageInfo) (map[string]bool, error) {
	oldMod, err := goModAt(modRoot, from)
	if err != nil {
		return nil, fmt.Errorf("reading go.mod at %s: %w", from, err)
	}
	newMod, err := goModAt(modRoot, to)
	if err != nil {
		return nil, fmt.Errorf("reading go.mod at %s: %w", to, err)
	}

	changed := diffModules(oldMod, newMod)
	if len(changed) == 0 {
		return nil, nil
	}

	return packagesImporting(changed, pkgs), nil
}

// goModAt retrieves the go.mod file content at a specific git ref.
func goModAt(modRoot, ref string) (*modfile.File, error) {
	cmd := exec.Command("git", "show", ref+":go.mod")
	cmd.Dir = modRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:go.mod: %w", ref, err)
	}
	return modfile.Parse("go.mod", out, nil)
}

// diffModules compares two parsed go.mod files and returns the set of
// module paths whose version or replace target changed.
func diffModules(oldMod, newMod *modfile.File) map[string]bool {
	changed := make(map[string]bool)

	oldReqs := requireMap(oldMod)
	newReqs := requireMap(newMod)

	for mod, ver := range oldReqs {
		if newReqs[mod] != ver {
			changed[mod] = true
		}
	}
	for mod, ver := range newReqs {
		if oldReqs[mod] != ver {
			changed[mod] = true
		}
	}

	oldRepls := replaceMap(oldMod)
	newRepls := replaceMap(newMod)

	for mod, repl := range oldRepls {
		if newRepls[mod] != repl {
			changed[mod] = true
		}
	}
	for mod, repl := range newRepls {
		if oldRepls[mod] != repl {
			changed[mod] = true
		}
	}

	return changed
}

func requireMap(f *modfile.File) map[string]string {
	m := make(map[string]string, len(f.Require))
	for _, r := range f.Require {
		m[r.Mod.Path] = r.Mod.Version
	}
	return m
}

type replaceTarget struct {
	path    string
	version string
}

func (r replaceTarget) String() string {
	return r.path + "@" + r.version
}

func replaceMap(f *modfile.File) map[string]string {
	m := make(map[string]string, len(f.Replace))
	for _, r := range f.Replace {
		t := replaceTarget{path: r.New.Path, version: r.New.Version}
		m[r.Old.Path] = t.String()
	}
	return m
}

// packagesImporting finds internal packages whose transitive
// dependency closure includes any package from a changed module.
func packagesImporting(changedModules map[string]bool, pkgs map[string]*packageInfo) map[string]bool {
	seeds := make(map[string]bool)
	for _, p := range pkgs {
		if importsChangedModule(p, changedModules) {
			seeds[p.ImportPath] = true
		}
	}
	return seeds
}

// importsChangedModule checks whether any of a package's transitive
// deps belong to a changed module. It checks both the Deps field
// (transitive closure) and direct imports/test imports.
func importsChangedModule(p *packageInfo, changedModules map[string]bool) bool {
	all := make([]string, 0, len(p.Deps)+len(p.Imports)+len(p.TestImports)+len(p.XTestImports))
	all = append(all, p.Deps...)
	all = append(all, p.Imports...)
	all = append(all, p.TestImports...)
	all = append(all, p.XTestImports...)

	for _, dep := range all {
		for mod := range changedModules {
			if dep == mod || strings.HasPrefix(dep, mod+"/") {
				return true
			}
		}
	}
	return false
}
