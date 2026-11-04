package aruba

import "testing"

func TestHardwareModels(t *testing.T) {
	h, ok := LookupHW("JL256A 2930F-48G-PoE+-4SFP+ Switch")
	if !ok || len(h.SFP) != 4 || h.SFP[0] != "49" {
		t.Fatalf("JL256A: %+v %v", h, ok)
	}
	if _, ok := LookupHW("Aruba JL658A 6300M"); !ok {
		t.Fatal("JL658A not found")
	}
	if _, ok := LookupHW("Some other switch"); ok {
		t.Fatal("found a model that isn't there")
	}
	// empty cages count as SFP: no optic means no type from the switch
	snap := &Snapshot{Facts: Facts{Model: "JL256A 2930F-48G-PoE+-4SFP+"}, Ports: []Port{{Name: "48", Type: "100/1000T"}, {Name: "49"}, {Name: "1/52"}}}
	c := SFPCages(snap)
	if c["48"] || !c["49"] || !c["1/52"] {
		t.Fatalf("cages: %v", c)
	}
}
