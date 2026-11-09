package aruba

import (
	"regexp"
	"sort"
	"strings"
)

// Checks that came out of a Reddit thread about SwitchDesk: SSH algorithms,
// failed logins, port flaps, local user changes, stack members and the MAC
// address table (for finding where a client is plugged in).

// ---------- SSH algorithms ----------

var sshAlgoLine = regexp.MustCompile(`(?i)^\s*(ciphers?|macs?|key exchange(?: algorithms)?|kex(?: algorithms)?)\s*:\s*(.+)$`)

// ParseSSHAlgorithms reads the cipher, MAC and key exchange lists from
// "show ip ssh" (AOS-S) or "show ssh server" (AOS-CX).
func ParseSSHAlgorithms(out string) (ciphers, macs, kex []string) {
	split := func(s string) []string {
		var r []string
		for _, f := range strings.FieldsFunc(s, func(c rune) bool { return c == ',' || c == ' ' }) {
			if f = strings.TrimSpace(f); f != "" {
				r = append(r, f)
			}
		}
		return r
	}
	for _, line := range strings.Split(out, "\n") {
		m := sshAlgoLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		k := strings.ToLower(m[1])
		switch {
		case strings.HasPrefix(k, "cipher"):
			if ciphers == nil {
				ciphers = split(m[2])
			}
		case strings.HasPrefix(k, "mac"):
			if macs == nil {
				macs = split(m[2])
			}
		default:
			if kex == nil {
				kex = split(m[2])
			}
		}
	}
	return
}

// WeakSSH lists the offered algorithms that are considered outdated:
// CBC-mode and legacy ciphers, MD5 and SHA-1 MACs, and SHA-1 key exchange.
func WeakSSH(sec Security) []string {
	var weak []string
	for _, c := range sec.SSHCiphers {
		l := strings.ToLower(c)
		if strings.Contains(l, "cbc") || strings.Contains(l, "3des") || strings.Contains(l, "arcfour") || strings.Contains(l, "blowfish") || strings.Contains(l, "cast128") || strings.Contains(l, "rc4") {
			weak = append(weak, c)
		}
	}
	for _, m := range sec.SSHMACs {
		l := strings.ToLower(m)
		if strings.Contains(l, "md5") || strings.Contains(l, "sha1") || strings.HasSuffix(l, "-96") {
			weak = append(weak, m)
		}
	}
	for _, k := range sec.SSHKex {
		l := strings.ToLower(k)
		if strings.HasSuffix(l, "sha1") || strings.Contains(l, "group1-") {
			weak = append(weak, k)
		}
	}
	return weak
}

// ---------- events in the log ----------

var failedLoginRe = regexp.MustCompile(`(?i)invalid user name/password|invalid (user|password)|authentication fail|login fail|failed to authenticate|auth(entication)? failure|password incorrect|bad password`)

// FailedLogins counts failed login attempts in the recent log.
func FailedLogins(logs []LogLine) int {
	n := 0
	for _, l := range logs {
		if failedLoginRe.MatchString(l.Text) {
			n++
		}
	}
	return n
}

// AOS-S: "ports: port 5 is now off-line"; AOS-CX: "Interface 1/1/5 link state changed to down",
// "Link status for interface 1/1/5 is down", "port 1/1/5 is down".
var linkDownRe = regexp.MustCompile(`(?i)(?:port|interface)\s+((?:\d+/){0,2}[A-Z]?\d+|lag\d+|Trk\d+)\b[^\n]*?\b(?:is now off-?line|link (?:state )?(?:changed to |is )?down|is down|went down)`)

// PortFlaps counts how often each port went down in the recent log.
func PortFlaps(logs []LogLine) map[string]int {
	out := map[string]int{}
	for _, l := range logs {
		if m := linkDownRe.FindStringSubmatch(l.Text); m != nil {
			out[m[1]]++
		}
	}
	return out
}

// ---------- local users ----------

var cxUserRe = regexp.MustCompile(`^user\s+(\S+)\s+group\s+(\S+)`)

// ConfigUsers lists the local accounts in a running config, as "name (role)".
func ConfigUsers(f Family, cfg string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, line := range strings.Split(cfg, "\n") {
		h := strings.TrimSpace(line)
		switch f {
		case FamilyAOSCX:
			if m := cxUserRe.FindStringSubmatch(h); m != nil {
				add(m[1] + " (" + m[2] + ")")
			}
		default:
			// password manager user-name "admin" ...; aaa authentication local-user "bob" group "x" ...
			if strings.HasPrefix(h, "password ") {
				f := strings.Fields(h)
				if len(f) >= 4 && f[2] == "user-name" {
					add(unquote(f[3]) + " (" + f[1] + ")")
				}
			} else if strings.HasPrefix(h, "aaa authentication local-user ") {
				f := strings.Fields(h)
				role := ""
				for i := range f {
					if f[i] == "group" && i+1 < len(f) {
						role = unquote(f[i+1])
					}
				}
				if len(f) >= 4 {
					if role != "" {
						add(unquote(f[3]) + " (" + role + ")")
					} else {
						add(unquote(f[3]))
					}
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// DiffUsers says which accounts appeared and disappeared between two configs.
func DiffUsers(before, after []string) (added, removed []string) {
	was := map[string]bool{}
	for _, u := range before {
		was[u] = true
	}
	is := map[string]bool{}
	for _, u := range after {
		is[u] = true
		if !was[u] {
			added = append(added, u)
		}
	}
	for _, u := range before {
		if !is[u] {
			removed = append(removed, u)
		}
	}
	return
}

// ---------- MAC address table ----------

var macTableRe = regexp.MustCompile(`(?i)^\s*((?:[0-9a-f]{2}[:-]){5}[0-9a-f]{2}|[0-9a-f]{6}-[0-9a-f]{6})\s+(.+)$`)

// ParseMACTable reads "show mac-address" (AOS-S: MAC, Port, VLAN) and
// "show mac-address-table" (AOS-CX: MAC, VLAN, Type, Port).
func ParseMACTable(out string) []MACEntry {
	if out == "" || IsCLIError(out) {
		return nil
	}
	var res []MACEntry
	for _, line := range strings.Split(out, "\n") {
		m := macTableRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		fs := strings.Fields(m[2])
		if len(fs) == 0 {
			continue
		}
		e := MACEntry{MAC: NormMAC(m[1])}
		if len(fs) >= 3 && isNumber(fs[0]) && !isNumber(fs[1]) {
			// AOS-CX: VLAN, type, port
			e.VLAN, e.Port = fs[0], fs[len(fs)-1]
		} else {
			// AOS-S: port, VLAN
			e.Port = fs[0]
			if len(fs) >= 2 && isNumber(fs[1]) {
				e.VLAN = fs[1]
			}
		}
		res = append(res, e)
		if len(res) >= 50000 {
			break
		}
	}
	return res
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// ---------- stack members ----------

var stackMemberMAC = regexp.MustCompile(`^(?:[0-9a-f]{6}-[0-9a-f]{6}|(?:[0-9a-f]{2}:){5}[0-9a-f]{2})$`)

// Healthy member roles. AOS-CX 10.10 and later say "conductor" where older
// releases said "master" (issue #22).
var stackOKStatus = map[string]bool{"commander": true, "conductor": true, "master": true, "standby": true, "member": true, "active": true, "ready": true}
var stackTwoWord = map[string]bool{"version mismatch": true, "not joined": true, "not present": true, "fabric mismatch": true, "in progress": true}

// Normalize re-reads the stack against the current rules, so a snapshot saved
// by an older version doesn't keep a wrong verdict until the next collection:
// a single member is no stack, and each member's health follows its role.
func (st *Stack) Normalize() *Stack {
	if st == nil || len(st.Members) < 2 {
		return nil
	}
	for i := range st.Members {
		st.Members[i].OK = stackOKStatus[strings.ToLower(st.Members[i].Status)]
	}
	return st
}

// ParseStack reads AOS-S "show stacking" or AOS-CX "show vsf". It returns nil
// for a standalone switch.
func ParseStack(out string) *Stack {
	if out == "" || IsCLIError(out) {
		return nil
	}
	l := strings.ToLower(out)
	if strings.Contains(l, "stacking is disabled") || strings.Contains(l, "not configured") || strings.Contains(l, "not enabled") || strings.Contains(l, "not supported") {
		return nil
	}
	st := &Stack{}
	kv := parseKV(out)
	st.Topology = first(kv, "Stack Topology", "Topology")
	st.Status = first(kv, "Stack Status", "Status")
	inTable := false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimRight(line, " \r")
		if strings.HasPrefix(strings.TrimSpace(t), "---") {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		fs := strings.Fields(t)
		if len(fs) < 4 || !isNumber(fs[0]) || !stackMemberMAC.MatchString(strings.ToLower(fs[1])) {
			continue
		}
		rest := fs[2:]
		status := rest[len(rest)-1]
		rest = rest[:len(rest)-1]
		if len(rest) > 1 && stackTwoWord[strings.ToLower(rest[len(rest)-1]+" "+status)] {
			status = rest[len(rest)-1] + " " + status
			rest = rest[:len(rest)-1]
		}
		// AOS-S has a priority column between the model and the status
		if len(rest) > 1 && isNumber(rest[len(rest)-1]) {
			rest = rest[:len(rest)-1]
		}
		st.Members = append(st.Members, StackMember{ID: fs[0], MAC: NormMAC(fs[1]), Model: strings.Join(rest, " "), Status: status, OK: stackOKStatus[strings.ToLower(status)]})
	}
	// A standalone AOS-CX switch still answers "show vsf" with itself as
	// member 1 (the 6100, 6200 and 6300 all do), so one member is no stack.
	if len(st.Members) < 2 {
		return nil
	}
	return st
}
