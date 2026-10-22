package league

import (
	"github.com/pocketbase/pocketbase/core"
)

// WithdrawnIndex maps competition ID to the set of withdrawn pair IDs.
type WithdrawnIndex map[string]map[string]bool

// WithdrawnSet is the set of pair IDs withdrawn from comp.
func WithdrawnSet(comp *core.Record) map[string]bool {
	set := map[string]bool{}
	for _, id := range comp.GetStringSlice("withdrawn_pairs") {
		set[id] = true
	}
	return set
}

// LoadWithdrawnIndex loads the withdrawn pairs of every competition with one query.
func LoadWithdrawnIndex(app core.App) (WithdrawnIndex, error) {
	comps, err := app.FindRecordsByFilter("competitions",
		"withdrawn_pairs != '' && withdrawn_pairs != '[]'", "", 0, 0)
	if err != nil {
		return nil, err
	}
	idx := WithdrawnIndex{}
	for _, c := range comps {
		idx[c.Id] = WithdrawnSet(c)
	}
	return idx, nil
}

// Has reports whether pairID is withdrawn from compID.
func (i WithdrawnIndex) Has(compID, pairID string) bool {
	return i[compID][pairID]
}
