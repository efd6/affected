package main

// walk performs a breadth-first traversal from the seed set along
// reverse import edges, returning all transitively affected packages.
// The seeds themselves are included in the result.
func walk(seeds map[string]bool, reverse map[string]map[string]bool) map[string]bool {
	affected := make(map[string]bool, len(seeds))
	queue := make([]string, 0, len(seeds))

	for p := range seeds {
		affected[p] = true
		queue = append(queue, p)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for dep := range reverse[current] {
			if !affected[dep] {
				affected[dep] = true
				queue = append(queue, dep)
			}
		}
	}

	return affected
}
