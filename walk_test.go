package main

import (
	"testing"
)

func TestWalk(t *testing.T) {
	// Graph:
	//   d -> b -> a
	//        c -> a (via test import)
	//   e (isolated)
	reverse := map[string]map[string]bool{
		"d": {"b": true},
		"b": {"a": true, "c": true},
		"c": {"a": true},
	}

	tests := []struct {
		name  string
		seeds map[string]bool
		want  map[string]bool
	}{
		{
			name:  "leaf change propagates to root",
			seeds: map[string]bool{"d": true},
			want:  map[string]bool{"d": true, "b": true, "a": true, "c": true},
		},
		{
			name:  "middle change",
			seeds: map[string]bool{"b": true},
			want:  map[string]bool{"b": true, "a": true, "c": true},
		},
		{
			name:  "root change stays local",
			seeds: map[string]bool{"a": true},
			want:  map[string]bool{"a": true},
		},
		{
			name:  "isolated package",
			seeds: map[string]bool{"e": true},
			want:  map[string]bool{"e": true},
		},
		{
			name:  "multiple seeds",
			seeds: map[string]bool{"d": true, "c": true},
			want:  map[string]bool{"d": true, "b": true, "a": true, "c": true},
		},
		{
			name:  "empty seeds",
			seeds: map[string]bool{},
			want:  map[string]bool{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := walk(test.seeds, reverse)
			if len(got) != len(test.want) {
				t.Errorf("got %d affected, want %d: got=%v", len(got), len(test.want), got)
			}
			for p := range test.want {
				if !got[p] {
					t.Errorf("missing affected package %s", p)
				}
			}
			for p := range got {
				if !test.want[p] {
					t.Errorf("unexpected affected package %s", p)
				}
			}
		})
	}
}

func TestWalkCycle(t *testing.T) {
	// Cycles shouldn't cause infinite loops.
	reverse := map[string]map[string]bool{
		"a": {"b": true},
		"b": {"c": true},
		"c": {"a": true},
	}

	got := walk(map[string]bool{"a": true}, reverse)
	want := map[string]bool{"a": true, "b": true, "c": true}

	if len(got) != len(want) {
		t.Fatalf("got %d affected, want %d: got=%v", len(got), len(want), got)
	}
	for p := range want {
		if !got[p] {
			t.Errorf("missing affected package %s", p)
		}
	}
}
