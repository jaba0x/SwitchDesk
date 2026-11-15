package core

import (
	"reflect"
	"testing"
)

// Issue #34: switches downstream are updated first.
func TestTopoDepthOrder(t *testing.T) {
	// core -- agg -- (third-party switch) -- acc1, agg -- acc2; lone is unlinked
	topo := Topology{
		Nodes: []Node{{ID: "core", Managed: true, StpRoot: true}, {ID: "agg", Managed: true}, {ID: "x", Kind: "external-switch"},
			{ID: "acc1", Managed: true}, {ID: "acc2", Managed: true}, {ID: "lone", Managed: true}},
		Edges: []Edge{{From: "core", To: "agg"}, {From: "agg", To: "x"}, {From: "x", To: "acc1"}, {From: "agg", To: "acc2"}},
	}
	d := topoDepth(topo)
	want := map[string]int{"core": 0, "agg": 1, "x": 2, "acc2": 2, "acc1": 3}
	if !reflect.DeepEqual(d, want) {
		t.Fatalf("depth %v", d)
	}
	ids := orderByDepth([]string{"core", "lone", "acc2", "agg", "acc1"}, d)
	if got := []string{"acc1", "acc2", "agg", "core", "lone"}; !reflect.DeepEqual(ids, got) {
		t.Fatalf("order %v", ids)
	}

	// no root known: the busiest managed switch is the top
	topo.Nodes[0].StpRoot = false
	if d := topoDepth(topo); d["agg"] != 0 || d["core"] != 1 || d["acc1"] != 2 {
		t.Fatalf("no root: %v", d)
	}
}
