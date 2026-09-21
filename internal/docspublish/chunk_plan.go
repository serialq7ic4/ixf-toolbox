package docspublish

import "fmt"

// writeGroup is one sequential write: a contiguous run of top-level specs, the
// half-open range of block entries that run owns, and its entry cost.
type writeGroup struct {
	StartSpec    int
	EndSpec      int
	StartEntry   int
	EndEntry     int
	BlockEntries int
}

// partitionWriteGroups splits top-level specs into groups that each fit inside a
// single write.
//
// Splitting happens only between top-level specs. buildSpecBlock appends a spec's
// own entry followed by its recursively built children, so each top-level spec
// owns a contiguous run of entries and a boundary between two top-level specs can
// never separate a parent from its children. Splitting Markdown text or line
// ranges instead would re-parse each piece independently and silently promote an
// ordered item's child code block to a top-level sibling.
//
// A single top-level spec that exceeds the budget on its own cannot be split by
// any grouping, so it is reported as an error rather than attempted.
func partitionWriteGroups(specs []Spec) ([]writeGroup, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	costs, err := specEntryCosts(specs)
	if err != nil {
		return nil, err
	}

	groups := []writeGroup{}
	current := writeGroup{}
	for index, cost := range costs {
		if current.BlockEntries > 0 && current.BlockEntries+cost > blockEntryWriteBudget {
			current.EndSpec = index
			current.EndEntry = current.StartEntry + current.BlockEntries
			groups = append(groups, current)
			current = writeGroup{StartSpec: index, StartEntry: current.EndEntry}
		}
		current.BlockEntries += cost
	}
	current.EndSpec = len(specs)
	current.EndEntry = current.StartEntry + current.BlockEntries
	groups = append(groups, current)
	return groups, nil
}

// specEntryCosts measures each top-level spec's entry cost once, and rejects any
// single spec that cannot fit in one write.
func specEntryCosts(specs []Spec) ([]int, error) {
	costs := make([]int, len(specs))
	for index, spec := range specs {
		cost, _ := measureSpecs([]Spec{spec})
		if cost > blockEntryWriteBudget {
			return nil, fmt.Errorf(
				"a single %s block expands to %d block entries, which exceeds the %d entry budget for one write;"+
					" no split can make it fit, so reduce that block (for example, shorten the table) before publishing",
				specKindLabel(spec.Kind), cost, blockEntryWriteBudget)
		}
		costs[index] = cost
	}
	return costs, nil
}

func specKindLabel(kind string) string {
	if kind == "" {
		return "content"
	}
	return kind
}
