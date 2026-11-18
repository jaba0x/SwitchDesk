package store

import (
	"reflect"
	"testing"
)

// Issue #43: site and tags for several switches at once.
func TestEditSwitches(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.SaveSwitch(Switch{Host: "10.0.0.1", Site: "HQ", Tags: []string{"Core", "old"}})
	b, _ := s.SaveSwitch(Switch{Host: "10.0.0.2", Tags: []string{"access"}})
	c, _ := s.SaveSwitch(Switch{Host: "10.0.0.3", Site: "Branch"})
	n, err := s.EditSwitches([]string{a.ID, b.ID}, BulkEdit{SetSite: true, Site: "  Factory   A ", AddTags: []string{"plant", "core"}, RemoveTags: []string{"OLD"}})
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	get := func(id string) Switch { sw, _ := s.Switch(id); return sw }
	if sw := get(a.ID); sw.Site != "Factory A" || !reflect.DeepEqual(sw.Tags, []string{"Core", "plant"}) {
		t.Fatalf("a: %q %v", sw.Site, sw.Tags)
	}
	if sw := get(b.ID); sw.Site != "Factory A" || !reflect.DeepEqual(sw.Tags, []string{"access", "plant", "core"}) {
		t.Fatalf("b: %q %v", sw.Site, sw.Tags)
	}
	if sw := get(c.ID); sw.Site != "Branch" {
		t.Fatal("c changed")
	}
	// clearing the site leaves tags alone
	if n, _ := s.EditSwitches([]string{a.ID, c.ID}, BulkEdit{SetSite: true}); n != 2 || get(a.ID).Site != "" || get(c.ID).Site != "" || len(get(a.ID).Tags) != 2 {
		t.Fatal("clear site")
	}
}
