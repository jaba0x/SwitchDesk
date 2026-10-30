package aruba

import (
	"reflect"
	"testing"
)

// Issue #14: stacked AOS-S switches (2930M, 3810, VSF) name ports 1/24 and
// 1/A1. The module ports were dropped and port ranges weren't expanded.
func TestStackedAOSSPorts(t *testing.T) {
	brief := "  1/1          100/1000T  | No        Yes     Up     1000FDx    MDIX off  0\n" +
		"  1/24         100/1000T  | No        Yes     Down   1000FDx    Auto off  0\n" +
		"  1/A1         SFP+SR     | No        Yes     Up     10GigFD    NA   off  0\n" +
		"  2/1          100/1000T  | No        Yes     Up     1000FDx    MDIX off  0\n" +
		"  2/A2         SFP+DA3    | No        No      Down   10GigFD    NA   off  0\n" +
		"  Trk1                    | No        Yes     Up     10GigFD         off  0\n"
	var names []string
	for _, p := range ParseInterfacesBrief(brief) {
		names = append(names, p.Name)
	}
	if want := []string{"1/1", "1/24", "1/A1", "2/1", "2/A2", "Trk1"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("ports: %v", names)
	}

	for in, want := range map[string][]string{
		"1/1-1/3,1/A1-1/A2,2/5": {"1/1", "1/2", "1/3", "1/A1", "1/A2", "2/5"},
		"1/1/1-1/1/3":           {"1/1/1", "1/1/2", "1/1/3"},
		"1-3,A1-A2,49":          {"1", "2", "3", "A1", "A2", "49"},
		"1/1-2/3":               {"1/1-2/3"}, // across members: left as it is
	} {
		if got := ExpandPorts(in); !reflect.DeepEqual(got, want) {
			t.Errorf("ExpandPorts(%q) = %v, want %v", in, got, want)
		}
	}
	if got := CompressPorts([]string{"1/1", "1/2", "1/3", "1/A1", "1/A2"}); got != "1/1-1/3,1/A1-1/A2" {
		t.Errorf("CompressPorts: %q", got)
	}
	for _, p := range []string{"1/A1", "2/24", "1/1/1", "A1", "Trk1"} {
		if !safePort.MatchString(p) {
			t.Errorf("%s rejected", p)
		}
	}
	for _, p := range []string{"1/A1;reload", "../1", ""} {
		if safePort.MatchString(p) {
			t.Errorf("%q accepted", p)
		}
	}
}
