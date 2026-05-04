package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// packageInfo holds the subset of go list -json fields we need.
type packageInfo struct {
	ImportPath      string   `json:"ImportPath"`
	Dir             string   `json:"Dir"`
	GoFiles         []string `json:"GoFiles"`
	TestGoFiles     []string `json:"TestGoFiles"`
	XTestGoFiles    []string `json:"XTestGoFiles"`
	Imports         []string `json:"Imports"`
	TestImports     []string `json:"TestImports"`
	XTestImports    []string `json:"XTestImports"`
	Deps            []string `json:"Deps"`
	EmbedFiles      []string `json:"EmbedFiles"`
	TestEmbedFiles  []string `json:"TestEmbedFiles"`
	XTestEmbedFiles []string `json:"XTestEmbedFiles"`

	// Configs records which tag configurations include this package.
	// "-" means the package is visible with no build tags.
	Configs []string `json:"-"`
}

// listAllPackages runs go list -json ./... once per tag configuration
// and merges the results. The configs slice contains the individual tag
// values to test; a no-tag run (labelled "-") is always included.
func listAllPackages(modRoot string, configs []string) (map[string]*packageInfo, error) {
	labels := append([]string{"-"}, configs...)

	merged := make(map[string]*packageInfo)
	for _, cfg := range labels {
		var tags string
		if cfg != "-" {
			tags = cfg
		}
		data, err := runGoList(modRoot, tags)
		if err != nil {
			return nil, err
		}
		pkgs, err := parsePackages(data)
		if err != nil {
			return nil, err
		}
		mergePackages(merged, pkgs, cfg)
	}
	return merged, nil
}

// runGoList executes go list -json ./... with optional build tags.
func runGoList(modRoot, tags string) ([]byte, error) {
	args := []string{"list", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "./...")
	cmd := exec.Command("go", args...)
	cmd.Dir = modRoot
	out, err := cmd.Output()
	if err != nil {
		if tags != "" {
			return nil, fmt.Errorf("go list -json -tags %s ./...: %w", tags, err)
		}
		return nil, fmt.Errorf("go list -json ./...: %w", err)
	}
	return out, nil
}

// mergePackages merges pkgs into dst, annotating each with the config
// label. For packages already in dst, it unions the import and dep
// slices and appends the config label.
func mergePackages(dst map[string]*packageInfo, pkgs map[string]*packageInfo, config string) {
	for path, p := range pkgs {
		existing, ok := dst[path]
		if !ok {
			p.Configs = []string{config}
			dst[path] = p
			continue
		}
		existing.Configs = append(existing.Configs, config)
		existing.Imports = unionStrings(existing.Imports, p.Imports)
		existing.TestImports = unionStrings(existing.TestImports, p.TestImports)
		existing.XTestImports = unionStrings(existing.XTestImports, p.XTestImports)
		existing.Deps = unionStrings(existing.Deps, p.Deps)
		existing.GoFiles = unionStrings(existing.GoFiles, p.GoFiles)
		existing.TestGoFiles = unionStrings(existing.TestGoFiles, p.TestGoFiles)
		existing.XTestGoFiles = unionStrings(existing.XTestGoFiles, p.XTestGoFiles)
	}
}

// unionStrings returns the union of two string slices, preserving order
// of first appearance.
func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, s := range a {
		seen[s] = true
	}
	result := append([]string(nil), a...)
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// parsePackages decodes a stream of JSON objects (as produced by go list -json)
// into a map keyed by import path.
func parsePackages(data []byte) (map[string]*packageInfo, error) {
	pkgs := make(map[string]*packageInfo)
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var p packageInfo
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("decoding go list output: %w", err)
		}
		pkgs[p.ImportPath] = &p
	}
	return pkgs, nil
}

// buildReverseGraph builds a map from package P to the set of packages
// that import P (directly in code or in tests). Only internal packages
// (those present in pkgs) are tracked as dependents.
func buildReverseGraph(pkgs map[string]*packageInfo) map[string]map[string]bool {
	reverse := make(map[string]map[string]bool)
	for _, p := range pkgs {
		addEdges(reverse, p.ImportPath, p.Imports)
		addEdges(reverse, p.ImportPath, p.TestImports)
		addEdges(reverse, p.ImportPath, p.XTestImports)
	}
	return reverse
}

func addEdges(reverse map[string]map[string]bool, importer string, deps []string) {
	for _, dep := range deps {
		if !strings.Contains(dep, ".") {
			// Heuristic: standard library packages have no dot in
			// the first path element. This matches all real module
			// paths (which require a dot) and excludes stdlib.
			continue
		}
		if reverse[dep] == nil {
			reverse[dep] = make(map[string]bool)
		}
		reverse[dep][importer] = true
	}
}
