package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// changedFiles returns the set of files changed between from and to,
// grouped by the directory relative to modRoot. It also reports whether
// go.mod was among the changed files.
func changedFiles(modRoot, from, to string) (files map[string][]string, goModChanged bool, err error) {
	cmd := exec.Command("git", "diff", "--name-only", from+"..."+to)
	cmd.Dir = modRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, false, fmt.Errorf("git diff --name-only %s...%s: %w", from, to, err)
	}

	files = make(map[string][]string)
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		rel := strings.TrimSpace(sc.Text())
		if rel == "" {
			continue
		}
		if rel == "go.mod" {
			goModChanged = true
			continue
		}
		if rel == "go.sum" {
			continue
		}
		dir := filepath.Dir(rel)
		files[dir] = append(files[dir], rel)
	}
	return files, goModChanged, sc.Err()
}

// fileSeeds maps changed file directories to the Go packages that own
// them. For directories that are not themselves a package (e.g. testdata
// subdirectories), it walks up until it finds an enclosing package.
//
// It also checks for .affected manifest files in each package directory
// and adds packages whose declared patterns match any changed file.
func fileSeeds(changed map[string][]string, pkgs map[string]*packageInfo, modPath string) map[string]bool {
	pkgByImport := make(map[string]bool, len(pkgs))
	for imp := range pkgs {
		pkgByImport[imp] = true
	}

	seeds := make(map[string]bool)

	// Direct directory mapping.
	for dir := range changed {
		importPath := resolvePackage(dir, pkgByImport, modPath)
		if importPath != "" {
			seeds[importPath] = true
		}
	}

	// .affected manifest files.
	allChangedFiles := make(map[string]bool)
	for _, files := range changed {
		for _, f := range files {
			allChangedFiles[f] = true
		}
	}
	for _, p := range pkgs {
		patterns := readAffectedManifest(p.Dir)
		if len(patterns) == 0 {
			continue
		}
		for _, pat := range patterns {
			for f := range allChangedFiles {
				if matched, _ := filepath.Match(pat, f); matched {
					seeds[p.ImportPath] = true
					break
				}
			}
		}
	}

	return seeds
}

// resolvePackage walks dir upward through the module to find the
// nearest enclosing Go package. It only walks up through directories
// that are children of a known package directory (e.g. testdata/
// subdirectories), not through arbitrary unrelated paths.
func resolvePackage(dir string, knownPkgs map[string]bool, modPath string) string {
	if dir == "." {
		if knownPkgs[modPath] {
			return modPath
		}
		return ""
	}

	d := dir
	for d != "." && d != "" {
		candidate := modPath + "/" + d
		if knownPkgs[candidate] {
			return candidate
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

// readAffectedManifest reads a .affected file from dir if it exists.
// Each non-empty, non-comment line is a filepath.Match pattern relative
// to the module root.
func readAffectedManifest(dir string) []string {
	f, err := os.Open(filepath.Join(dir, ".affected"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var patterns []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if sc.Err() != nil {
		return nil
	}
	return patterns
}
