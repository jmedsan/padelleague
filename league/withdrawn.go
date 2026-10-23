package league

import (
	"github.com/pocketbase/pocketbase/core"
)

// WithdrawnSet is the set of pair IDs withdrawn from comp.
func WithdrawnSet(comp *core.Record) map[string]bool {
	set := map[string]bool{}
	for _, id := range comp.GetStringSlice("withdrawn_pairs") {
		set[id] = true
	}
	return set
}
