// Command affected identifies Go packages within a module that are
// transitively affected by a set of changes between two git commits.
// It outputs one package import path per line, suitable for passing
// to go test.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("affected: ")

	tags := flag.String("tags", "", "build tags (comma-separated), passed to go list")
	asJSON := flag.Bool("json", false, "output as JSON array with metadata")
	inclNoTests := flag.Bool("include-no-tests", false, "include packages that have no test files")
	relative := flag.Bool("relative", false, "output ./relative paths instead of full import paths")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] [from-ref] [to-ref]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Identifies Go packages affected by changes between two commits.\n\n")
		fmt.Fprintf(os.Stderr, "  from-ref  Base commit (default: merge-base with upstream default branch)\n")
		fmt.Fprintf(os.Stderr, "  to-ref    Head commit (default: HEAD)\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	from, to, err := resolveRefs(flag.Args())
	if err != nil {
		log.Fatal(err)
	}

	modRoot, modPath, err := moduleInfo()
	if err != nil {
		log.Fatal(err)
	}

	changed, goModChanged, err := changedFiles(modRoot, from, to)
	if err != nil {
		log.Fatal(err)
	}
	if len(changed) == 0 && !goModChanged {
		os.Exit(0)
	}

	var configs []string
	if *tags != "" {
		for _, t := range strings.Split(*tags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				configs = append(configs, t)
			}
		}
	}

	pkgs, err := listAllPackages(modRoot, configs)
	if err != nil {
		log.Fatal(err)
	}

	reverse := buildReverseGraph(pkgs)

	fileSeedSet := fileSeeds(changed, pkgs, modPath)

	allSeeds := make(map[string]bool, len(fileSeedSet))
	for p := range fileSeedSet {
		allSeeds[p] = true
	}
	if goModChanged {
		modSeedSet, err := goModSeeds(modRoot, from, to, pkgs)
		if err != nil {
			log.Fatal(err)
		}
		for p := range modSeedSet {
			allSeeds[p] = true
		}
	}

	affected := walk(allSeeds, reverse)

	if !*inclNoTests {
		for p := range affected {
			pkg, ok := pkgs[p]
			if !ok {
				delete(affected, p)
				continue
			}
			if len(pkg.TestGoFiles) == 0 && len(pkg.XTestGoFiles) == 0 {
				delete(affected, p)
			}
		}
	}

	sorted := make([]string, 0, len(affected))
	for p := range affected {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)

	if *asJSON {
		writeJSON(sorted, fileSeedSet, pkgs, modPath, *relative)
	} else {
		for _, p := range sorted {
			if *relative {
				p = toRelative(p, modPath)
			}
			fmt.Println(p)
		}
	}
}

// resolveRefs determines the from and to git refs from positional args.
// With no args it computes merge-base against the upstream default branch.
func resolveRefs(args []string) (from, to string, err error) {
	switch len(args) {
	case 0:
		to = "HEAD"
		from, err = mergeBase(to)
		if err != nil {
			return "", "", fmt.Errorf("determining merge-base: %w", err)
		}
	case 1:
		from = args[0]
		to = "HEAD"
	case 2:
		from = args[0]
		to = args[1]
	default:
		return "", "", fmt.Errorf("expected at most 2 positional arguments, got %d", len(args))
	}
	return from, to, nil
}

func mergeBase(head string) (string, error) {
	remote, err := defaultRemote()
	if err != nil {
		return "", err
	}
	branch, err := defaultBranch(remote)
	if err != nil {
		return "", err
	}
	out, err := exec.Command("git", "merge-base", remote+"/"+branch, head).Output()
	if err != nil {
		return "", fmt.Errorf("git merge-base %s/%s %s: %w", remote, branch, head, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func defaultRemote() (string, error) {
	out, err := exec.Command("git", "remote").Output()
	if err != nil {
		return "", fmt.Errorf("git remote: %w", err)
	}
	remotes := strings.Fields(strings.TrimSpace(string(out)))
	for _, r := range remotes {
		if r == "upstream" {
			return r, nil
		}
	}
	for _, r := range remotes {
		if r == "origin" {
			return r, nil
		}
	}
	if len(remotes) > 0 {
		return remotes[0], nil
	}
	return "", fmt.Errorf("no git remotes found")
}

func defaultBranch(remote string) (string, error) {
	out, err := exec.Command("git", "remote", "show", remote).Output()
	if err != nil {
		// Fall back to common names.
		for _, name := range []string{"main", "master"} {
			if err := exec.Command("git", "rev-parse", "--verify", remote+"/"+name).Run(); err == nil {
				return name, nil
			}
		}
		return "", fmt.Errorf("cannot determine default branch for remote %s", remote)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if branch, ok := strings.CutPrefix(line, "HEAD branch:"); ok {
			return strings.TrimSpace(branch), nil
		}
	}
	return "main", nil
}

// moduleInfo returns the module root directory and module path.
func moduleInfo() (root, modPath string, err error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", "", fmt.Errorf("go env GOMOD: %w", err)
	}
	gomod := strings.TrimSpace(string(out))
	if gomod == "" || gomod == os.DevNull {
		return "", "", fmt.Errorf("not inside a Go module")
	}
	root = filepath.Dir(gomod)

	out, err = exec.Command("go", "list", "-m").Output()
	if err != nil {
		return "", "", fmt.Errorf("go list -m: %w", err)
	}
	modPath = strings.TrimSpace(string(out))
	return root, modPath, nil
}

func toRelative(importPath, modPath string) string {
	rel, ok := strings.CutPrefix(importPath, modPath)
	if !ok {
		return importPath
	}
	rel, _ = strings.CutPrefix(rel, "/")
	if rel == "" {
		return "."
	}
	return "./" + rel
}

type affectedPkg struct {
	ImportPath string   `json:"import_path"`
	Direct     bool     `json:"direct"`
	Tags       []string `json:"tags"`
}

func writeJSON(sorted []string, seeds map[string]bool, pkgs map[string]*packageInfo, modPath string, rel bool) {
	out := make([]affectedPkg, len(sorted))
	for i, p := range sorted {
		display := p
		if rel {
			display = toRelative(p, modPath)
		}
		var tags []string
		if pkg := pkgs[p]; pkg != nil {
			tags = pkg.Configs
		}
		if tags == nil {
			tags = []string{}
		}
		out[i] = affectedPkg{
			ImportPath: display,
			Direct:     seeds[p],
			Tags:       tags,
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(out)
}
