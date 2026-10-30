package aruba

import "testing"

// Issue #14 (glepping's 2930M stack): ports in a trunk are listed as
// "1/13-Trk3", and module ports without an optic have no type.
const stack2930M = `
 Status and Counters - Port Status

                          | Intrusion                           MDI   Flow  Bcast
  Port         Type       | Alert     Enabled Status Mode       Mode  Ctrl  Limit
  ------------ ---------- + --------- ------- ------ ---------- ----- ----- ------
  1/12         100/1000T  | No        Yes     Down   1000FDx    Auto  off   0
  1/13-Trk3    100/1000T  | No        Yes     Down   1000FDx    Auto  off   0
  1/21-Trk5    1000SX     | No        Yes     Up     1000FDx    NA    off   0
  1/22         1000SX     | No        Yes     Up     1000FDx    NA    off   0
  1/24         100/1000T  | No        Yes     Down   1000FDx    Auto  off   0
  1/A1-Trk1    SFP+DA1    | No        Yes     Up     10GigFD    NA    off   0
  1/A3                    | No        Yes     Down   .                off   0
  2/13-Trk3    100/1000T  | No        Yes     Up     1000FDx    MDI   off   0
  13-Trk2      100/1000T  | No        Yes     Up     1000FDx    MDI   off   0
`

func TestTrunkMemberPorts(t *testing.T) {
	got := map[string]Port{}
	for _, p := range ParseInterfacesBrief(stack2930M) {
		got[p.Name] = p
	}
	for name, trunk := range map[string]string{"1/12": "", "1/13": "Trk3", "1/21": "Trk5", "1/22": "", "1/24": "", "1/A1": "Trk1", "1/A3": "", "2/13": "Trk3", "13": "Trk2"} {
		p, ok := got[name]
		if !ok {
			t.Errorf("port %s missing (have %d ports)", name, len(got))
			continue
		}
		if p.Trunk != trunk {
			t.Errorf("port %s trunk = %q, want %q", name, p.Trunk, trunk)
		}
	}
	if p := got["1/21"]; p.Type != "1000SX" || !p.Up {
		t.Errorf("1/21: %+v", p)
	}
	if p := got["1/A3"]; p.Up || p.Type != "" {
		t.Errorf("1/A3: %+v", p)
	}
}
