package slk

import (
	"sort"

	"github.com/renstrom/fuzzysearch/fuzzy"
)

type lenStr []string

func (a lenStr) Len() int           { return len(a) }
func (a lenStr) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a lenStr) Less(i, j int) bool { return len(a[i]) < len(a[j]) }

func fuzzySearch(query string, lookup map[string]Entity) []Entity {
	targets := make([]string, 0, len(lookup))
	for i := range lookup {
		targets = append(targets, i)
	}

	raw := lenStr(fuzzy.Find(query, targets))
	sort.Sort(raw)
	results := make([]Entity, 0, len(raw))
	for i := range raw {
		if user, ok := lookup[raw[i]]; ok {
			results = append(results, user)
		}
	}

	return results
}
