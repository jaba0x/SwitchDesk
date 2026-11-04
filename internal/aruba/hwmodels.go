package aruba

import (
	_ "embed"
	"encoding/json"
	"regexp"
	"strings"
)

// Hardware facts per Aruba part number, generated from the NetBox device type
// library (github.com/netbox-community/devicetype-library, CC0) by
// tools/netbox/gen.py: which front ports are SFP cages, and the library's
// front photo of the model (issue #20).
//
//go:embed hwmodels.json
var hwJSON []byte

type HWModel struct {
	Model string   `json:"model"`
	SFP   []string `json:"sfp"`   // last number of each SFP port: "49" for 49, 1/49 and 1/1/49
	Photo string   `json:"photo"` // file in elevation-images/HPE, or ""
}

var hwModels = func() map[string]HWModel {
	m := map[string]HWModel{}
	json.Unmarshal(hwJSON, &m)
	return m
}()

var partNumberRe = regexp.MustCompile(`\b(J\d{4}[A-Z]|JL\d{3}[A-Z]|R\d[A-Z0-9]{2}\d{2}[A-Z]|S\d[A-Z0-9]{2}\d{2}[A-Z])\b`)

// LookupHW finds the hardware facts for a model string such as
// "JL256A 2930F-48G-PoE+-4SFP+" or "Aruba JL658A 6300M".
func LookupHW(model string) (HWModel, bool) {
	for _, pn := range partNumberRe.FindAllString(strings.ToUpper(model), -1) {
		if h, ok := hwModels[pn]; ok {
			return h, true
		}
	}
	return HWModel{}, false
}

// SFPCages returns the ports of snap that are SFP cages by the model's
// hardware, whether or not an optic is plugged in.
func SFPCages(snap *Snapshot) map[string]bool {
	if snap == nil {
		return nil
	}
	h, ok := LookupHW(snap.Facts.Model)
	if !ok || len(h.SFP) == 0 {
		return nil
	}
	cage := map[string]bool{}
	for _, n := range h.SFP {
		cage[n] = true
	}
	out := map[string]bool{}
	for _, p := range snap.Ports {
		last := p.Name[strings.LastIndex(p.Name, "/")+1:]
		if cage[last] {
			out[p.Name] = true
		}
	}
	return out
}
