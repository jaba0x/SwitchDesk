// Fake backend for previewing the UI in a plain browser (not shipped in the app).
(function () {
  const now = new Date().toISOString();
  const mins = (m) => new Date(Date.now() - m * 60000).toISOString();
  function ports(n, prefix, upPct, fiber) {
    const out = [];
    for (let i = 1; i <= n; i++) out.push({ name: prefix + i, up: Math.sin(i * 7.3) * 0.5 + 0.5 < upPct, enabled: i % 23 !== 0, type: "100/1000T", speed: "1000FDx" });
    for (let i = n + 1; i <= n + fiber; i++) out.push({ name: prefix + i, up: i !== n + fiber, enabled: true, type: "SFP+SR", speed: "10GigFD" });
    return out;
  }
  const sws = [
    { id: "core", name: "core-cx-01", host: "10.10.0.1", fam: "AOS-CX", model: "JL658A 6300M 24SFP+ 4SFP56", sw: "FL.10.13.1000", p: ports(24, "1/1/", 0.9, 4), role: "root" },
    { id: "agg1", name: "agg-cx-a", host: "10.10.0.2", fam: "AOS-CX", model: "JL678A 6100 24G CL4", sw: "PL.10.13.1000", p: ports(24, "1/1/", 0.6, 4), role: "member" },
    { id: "acc1", name: "acc-2930-f1", host: "10.10.1.11", fam: "AOS-S", model: "JL256A 2930F-48G-PoE+-4SFP+", sw: "WC.16.11.0015", p: ports(48, "", 0.7, 4), role: "member", issues: [["critical", "Security", "Telnet is enabled", "Credentials travel in clear text. Disable with: no telnet-server"], ["warning", "Config", "Unsaved configuration changes", "Running config differs from startup. Changes will be lost on reboot."]] },
    { id: "acc2", name: "acc-2930-f2", host: "10.10.1.12", fam: "AOS-S", model: "JL256A 2930F-48G-PoE+-4SFP+", sw: "WC.16.11.0015", p: ports(48, "", 0.5, 4), role: "member", issues: [["critical", "Hardware", "Fan problem", "Sys-2  | Fan Failed  | 1"], ["warning", "Spanning tree", "4 topology change(s) since last check", "Last change: 12 mins. Frequent changes usually mean a flapping link or an edge port without admin-edge."]] },
    { id: "acc3", name: "acc-2530-wh", host: "10.10.1.13", fam: "AOS-S", model: "J9773A 2530-24G-PoEP", sw: "YA.16.10.0021", p: ports(24, "", 0.4, 4), role: "disabled", issues: [["critical", "Spanning tree", "Spanning tree is disabled", "A single looped cable can take this segment down."], ["critical", "Security", "Default SNMP community in use", 'Community "public" or "private" is configured.']] },
    { id: "acc4", name: "acc-2930-f3", host: "10.10.1.14", offline: true },
    { id: "io1", name: "office-1930", host: "10.10.2.5", fam: "Instant On", model: "JL682A 1930 24G 4SFP+", sw: "1.0.7.0", p: ports(24, "", 0.55, 4), role: "member" },
  ];
  sws[1].p[25].blocking = true;
  const views = sws.map((s) => {
    const issues = (s.issues || []).map(([severity, category, title, detail]) => ({ switchId: s.id, switchName: s.name, severity, category, title, detail }));
    if (s.offline) issues.push({ switchId: s.id, switchName: s.name, severity: "critical", category: "Reachability", title: "Switch is offline", detail: "No answer on TCP 22 since Oct 23 09:41 (timeout)" });
    if (!s.offline) issues.push({ switchId: s.id, switchName: s.name, severity: "info", category: "Management", title: "No syslog server", detail: "Events are only kept in the switch's local buffer." });
    const order = { critical: 0, warning: 1, info: 2 };
    issues.sort((a, b) => order[a.severity] - order[b.severity]);
    return {
      id: s.id, name: s.name, host: s.host, port: 22, family: "auto", credentialId: "c1", site: "HQ",
      status: { online: !s.offline, checked: true, latencyMs: s.offline ? 0 : 2 + (s.host.length % 5), since: mins(s.offline ? 47 : 3000), collecting: false, lastCollect: mins(4) },
      summary: s.offline ? { family: "AOS-S", hostname: s.name, model: "JL256A 2930F-48G-PoE+-4SFP+", software: "WC.16.11.0015", ports: [], collectedAt: mins(300) } : { family: s.fam, hostname: s.name, model: s.model, software: s.sw, serial: "SG" + s.id.toUpperCase() + "123", uptime: "41 days", cpu: s.id === "acc2" ? 37 : 6, memory: 31, vlans: 6, portsUp: s.p.filter((x) => x.up).length, portsTotal: s.p.length, neighbors: 3, stpRole: s.role, collectedAt: mins(4), ports: s.p },
      issues, worst: issues[0]?.severity || "", disabled: false,
      firmware: s.offline ? {} : (() => {
        const run = s.sw && /^[A-Z]{2}\./.test(s.sw) ? s.sw : "";
        const avail = { "WC.16.11.0015": "WC.16.11.0021", "FL.10.13.1000": "FL.10.14.0001" }[run] || "";
        return { supported: s.fam !== "Instant On", running: run || s.sw, available: avail, image: avail ? avail.replace(/\./g, "_") + ".swi" : "", staged: s.id === "acc2",
          banks: { known: true, primary: run, secondary: s.id === "acc2" ? avail : run.startsWith("FL") ? "FL.10.12.1010" : run.startsWith("PL") ? "PL.10.12.1010" : run.startsWith("YA") ? "YA.16.10.0019" : "WC.16.10.0012", running: "primary", default: s.id === "acc2" ? "secondary" : "primary" },
          job: s.id === "acc1" ? { id: "j1", state: "running", progress: 0.42, step: "Copying WC_16_11_0021.swi to the secondary bank from 10.10.1.50", from: "WC.16.11.0015", to: "WC.16.11.0021" } : null };
      })(),
    };
  });
  views.push({ ...views[6], id: "old1", name: "old-2530-lab", host: "10.10.9.40", disabled: true, issues: [], worst: "", summary: { ...views[6].summary, model: "J9774A 2530-8G-PoEP", portsTotal: 10 } });
  const allIssues = views.flatMap((v) => v.issues);
  allIssues.push({ severity: "warning", category: "Spanning tree", switchName: "Network", title: "2 different spanning-tree roots", detail: "Expected one root per STP domain. Roots seen: 00:11:22:aa:bb:cc (core-cx-01, agg-cx-a, acc-2930-f1); 00:24:a8:10:00:01 (office-1930)." });
  const order = { critical: 0, warning: 1, info: 2 };
  allIssues.sort((a, b) => order[a.severity] - order[b.severity]);
  const dash = { disabled: 1, total: 8, online: 6, offline: 1, unknown: 0, critical: allIssues.filter((i) => i.severity === "critical").length, warning: allIssues.filter((i) => i.severity === "warning").length, info: allIssues.filter((i) => i.severity === "info").length, collecting: 0, issues: allIssues, switches: views, dataDir: "/Users/jaba/Library/Application Support/SwitchDesk" };
  const vlans = [
    { id: 1, name: "DEFAULT_VLAN", untagged: Array.from({ length: 40 }, (_, i) => String(i + 1)), tagged: [] },
    { id: 10, name: "Management", ip: "10.10.1.11 255.255.255.0", untagged: [], tagged: ["49", "50"] },
    { id: 20, name: "Servers", untagged: ["41", "42", "43"], tagged: ["49", "50"] },
    { id: 30, name: "Voice", untagged: [], tagged: ["1", "2", "3", "4", "5", "6", "49", "50"] },
    { id: 40, name: "Guest-WiFi", untagged: ["44"], tagged: ["49", "50"] },
    { id: 99, name: "Printers", untagged: ["45", "46"], tagged: ["49"] },
  ];
  const cfg = `; JL256A Configuration Editor; Created on release #WC.16.11.0015
hostname "acc-2930-f1"
module 1 type jl256a
snmp-server community "netops" operator
sntp server priority 1 10.10.0.53
timesync sntp
aaa authentication ssh login radius local
radius-server host 10.10.0.60 key "********"
web-management ssl
spanning-tree
spanning-tree 41-46 admin-edge-port
vlan 1
   name "DEFAULT_VLAN"
   untagged 1-40
   exit
vlan 10
   name "Management"
   tagged 49-50
   ip address 10.10.1.11 255.255.255.0
   exit
vlan 20
   name "Servers"
   untagged 41-43
   tagged 49-50
   exit
password manager`;
  const snap = {
    family: "aos-s", collectedAt: mins(4), duration: "6.4s",
    facts: { hostname: "acc-2930-f1", model: "JL256A 2930F-48G-PoE+-4SFP+ Switch", serial: "SG81JP1234", baseMac: "001122-334455", software: "WC.16.11.0015", uptime: "41 days", cpu: 6, memory: 31, location: "HQ floor 1, rack A2", configSaved: false },
    stp: { known: true, enabled: true, mode: "MSTP-operation", bridgePriority: "32768", bridgeMac: "00:11:22:33:44:55", rootPriority: "4096", rootMac: "00:11:22:aa:bb:cc", rootPort: "49", isRoot: false, topologyChanges: 18, lastChange: "2 hours", ports: [{ port: "49", role: "Root", state: "Forwarding", cost: "2000" }, { port: "50", role: "Alternate", state: "Blocking", cost: "2000" }, { port: "1", role: "Designated", state: "Forwarding", cost: "20000" }] },
    security: { sshEnabled: true, sshPort: "22", sshVersion: "2", telnetEnabled: true, httpPlain: false, httpsEnabled: true, snmpCommunities: ["netops"], snmpDefault: false, ntpServers: ["10.10.0.53"], logServers: [], aaa: ["aaa authentication ssh login radius local"], managerPassword: true },
    vlans, ports: sws[2].p.map((p, i) => ({ ...p, description: i === 48 ? "uplink core" : "" })),
    neighbors: [
      { protocol: "lldp", localPort: "49", sysName: "core-cx-01", chassisId: "00:11:22:aa:bb:cc", portId: "1/1/3", mgmtIp: "10.10.0.1", capabilities: "bridge, router", description: "Aruba JL658A 6300M FL.10.13.1000" },
      { protocol: "lldp", localPort: "50", sysName: "agg-cx-a", portId: "1/1/25", mgmtIp: "10.10.0.2", capabilities: "bridge", description: "Aruba JL678A 6100" },
      { protocol: "cdp", localPort: "7", sysName: "SEP00AABBCCDDEE", portId: "Port 1", mgmtIp: "10.10.30.21", capabilities: "Host Phone", description: "Cisco IP Phone 8841" },
      { protocol: "lldp", localPort: "12", sysName: "ap-lobby", portId: "eth0", mgmtIp: "10.10.40.4", capabilities: "wlan-access-point", description: "Aruba AP-505" },
    ],
    env: [{ kind: "fan", name: "Sys-1", status: "Sys-1 | Fan OK | 0", ok: true }, { kind: "psu", name: "1", status: "1 JL085A Powered", ok: true }],
    logs: [{ severity: "W", text: "W 10/23/26 21:14:02 00077 ports: port 12 is now off-line" }, { severity: "M", text: "M 10/23/26 20:02:51 00435 ports: port 50 is Disabled by Loop Protect" }, { severity: "I", text: "I 10/23/26 19:55:10 00076 ports: port 12 is now on-line" }],
    raw: { system: "  System Name        : acc-2930-f1\n  Software revision  : WC.16.11.0015", stp: "...", config: cfg },
    runningConfig: cfg, errors: [],
    portAccess: {
      servers: [{ host: "10.10.0.60", hasKey: true }], dot1xOn: true, macAuthOn: true,
      ports: [
        ...["1", "2", "3", "4", "5", "6"].map((port) => ({ port, dot1x: true, clientLimit: 2, reauthPeriod: 3600, unauthVlan: 20 })),
        { port: "7", dot1x: true, macAuth: true, clientLimit: 3 },
        { port: "30", macAuth: true },
        { port: "31", macAuth: true },
      ],
    },
    optics: [
      { port: "49", type: "SFP+SR", model: "J9150D", serial: "CN84KJ01A2", dom: true, tempC: 34.2, rx: [-2.7], tx: [-2.3], recent: false },
      { port: "50", type: "SFP+LR", model: "J9151E", serial: "CN84KJ09Q7", dom: true, tempC: 38.9, rx: [-23.9], tx: [-2.4], alarms: ["Rx power low alarm", "Rx power low warning"], recent: true },
      { port: "51", type: "SFP+SR", model: "J9150D", serial: "CN84KJ0333", dom: true, tempC: 31.0, rx: [-40], tx: [-2.5] },
      { port: "52", type: "SFP+DAC1", dom: false },
    ],
  };
  const topoNodes = views.map((v) => ({ id: v.id, label: v.name, kind: "switch", managed: true, host: v.host, model: v.summary?.model, online: v.status.online, severity: v.worst, issues: v.issues.length, stpRoot: v.id === "core" }));
  topoNodes.push({ id: "ext:fw", label: "fw-edge-01", kind: "router", host: "10.10.0.254", model: "FortiGate-100F", online: true });
  topoNodes.push({ id: "ext:ap", label: "ap-lobby", kind: "ap", host: "10.10.40.4", online: true });
  topoNodes.push({ id: "ext:ap2", label: "ap-meeting", kind: "ap", host: "10.10.40.5", online: true });
  const E = (from, fp, to, tp, extra = {}) => ({ id: from + fp + to, from, to, fromPort: fp, toPort: tp, protocols: ["lldp"], up: true, ...extra });
  const topo = { nodes: topoNodes, edges: [E("core", "1/1/1", "agg1", "1/1/25"), E("core", "1/1/2", "agg1", "1/1/26"), E("core", "1/1/3", "acc1", "49"), E("agg1", "1/1/27", "acc1", "50", { blocking: true }), E("core", "1/1/4", "acc2", "49"), E("agg1", "1/1/28", "acc3", "25"), E("core", "1/1/5", "acc4", "49", { up: false }), E("agg1", "1/1/20", "io1", "25"), E("core", "1/1/24", "ext:fw", "port3"), E("acc1", "12", "ext:ap", "eth0"), E("acc2", "9", "ext:ap2", "eth0")] };
  const io = views.find((v) => v.id === "io1");
  Object.assign(io, { port: 0, family: "instant-on", credentialId: "c2", site: "Office", cloud: { siteId: "s1", siteName: "Office", deviceId: "94:60:d5:12:34:56" } });
  io.status.latencyMs = 0;
  io.issues.splice(0, io.issues.length, { switchId: "io1", switchName: io.name, severity: "critical", category: "Spanning tree", title: "Loop detected", detail: "Ports: 14. Find the looped cable or device on these ports." }, { switchId: "io1", switchName: io.name, severity: "info", category: "Firmware", title: "Firmware update available", detail: "Running 1.0.7.0. Instant On updates are installed from the Instant On app or portal." });
  io.worst = "critical";
  const ioPorts = sws[6].p.map((p, i) => ({ ...p, speed: p.up ? "1G" : "", type: i >= 24 ? "SFP" : "", loop: i === 13, poeMw: i < 8 && p.up ? 4200 + i * 300 : 0, uplink: i === 24 }));
  const ioSnap = { family: "instant-on", collectedAt: mins(1), facts: { hostname: "office-1930", model: "JL682A", serial: "CN12KLM0QX", baseMac: "94:60:d5:12:34:56", software: "1.0.7.0", uptime: "41 days, 3h 12m", cpu: -1, memory: -1 },
    stp: { known: true, enabled: true, mode: "RSTP", bridgePriority: "32768", bridgeMac: "94:60:d5:12:34:56", isRoot: false, ports: [] }, security: {}, ports: ioPorts,
    neighbors: [{ protocol: "cloud", localPort: "25", sysName: "agg-cx-a", chassisId: "38:21:c7:00:00:02", capabilities: "Bridge" }, { protocol: "cloud", localPort: "3", sysName: "AP-Reception", chassisId: "94:60:d5:aa:00:01", description: "AP22", capabilities: "WLAN" }],
    cloud: { site: "Office", health: "warning", upToDate: false, firmware: "1.0.7.0", clients: 17, poeUsedW: 41.6, poeBudgetW: 195, alerts: [{ severity: "major", type: "loopDetected", text: "Loop detected (port 14)" }] }, errors: [] };
  const iow = JSON.parse(JSON.stringify(io));
  Object.assign(iow, { id: "io2", name: "SW-BRANCH-114", host: "192.168.13.3", port: 443, web: true, cloud: undefined, credentialId: "c1", site: "Digomi" });
  iow.summary = { ...iow.summary, hostname: "SW-BRANCH-114", model: "JL686A 1930 48p Gigabit CL4 PoE 4p SFP+ 370W", software: "InstantOn_1930_3.3.2.0 (5)" };
  iow.issues = [
    { switchId: "io2", switchName: "SW-BRANCH-114", severity: "critical", category: "Spanning tree", title: "Spanning tree is disabled", detail: "A single looped cable can take this segment down." },
    { switchId: "io2", switchName: "SW-BRANCH-114", severity: "warning", category: "Management", title: "Clock is off by 268 days", detail: "The switch's clock reads Jan 27 2026 23:21 (UTC). Log times and certificate checks depend on it. The SNTP server 172.30.0.1 has never answered." },
    { switchId: "io2", switchName: "SW-BRANCH-114", severity: "warning", category: "Spanning tree", title: "No loop protection", detail: "Spanning tree and loop protection are both off, so nothing stops a looped cable from flooding this segment." }];
  iow.worst = "critical";
  views.push(iow);
  const leg = JSON.parse(JSON.stringify(iow));
  Object.assign(leg, { id: "io3", name: "SW-OLD-1930", host: "10.10.2.9", port: 22, web: false, family: "instant-on", summary: null, issues: [{ switchId: "io3", switchName: "SW-OLD-1930", severity: "warning", category: "SSH", title: "Data collection failed", detail: "ssh: connect to host 10.10.2.9 port 22: connection refused" }] });
  views.push(leg);
  const webPorts = Array.from({ length: 52 }, (_, i) => ({ name: String(i + 1), enabled: true, up: [1, 9, 47, 6].includes(i), speed: [1, 9, 47, 6].includes(i) ? "1G" : "", type: i >= 48 ? "SFP+" : "100/1000T", poeMw: i === 1 || i === 9 ? 3200 : 0, loop: false, vlan: "1" }));
  const webSnap = { family: "instant-on", web: true, collectedAt: mins(2), duration: "3.1s",
    facts: { hostname: "SW-BRANCH-114", model: "JL686A 1930 48p Gigabit CL4 PoE 4p SFP+ 370W", serial: "CN0BKPJ14V", baseMac: "8c:85:c1:0f:50:0e", software: "InstantOn_1930_3.3.2.0 (5)", uptime: "71 days, 11h 46m", cpu: -1, memory: -1, location: "Tbilisi Energy", contact: "Network team", configSaved: false },
    stp: { known: true, enabled: false, mode: "MSTP", bridgePriority: "32768", bridgeMac: "8c:85:c1:0f:50:0e", topologyChanges: 0, ports: [] },
    security: { telnetEnabled: false, telnetPort: "closed", httpsEnabled: true, ntpServers: ["172.30.0.1"], logServers: ["10.0.0.195"], snmpCommunities: [] },
    ports: webPorts, vlans: [{ id: 1, name: "DEFAULT_VLAN", status: "Up", untagged: webPorts.map((p) => p.name), tagged: [] }],
    neighbors: [{ protocol: "lldp", localPort: "2", sysName: "GXP1625_c0:74:ad:24:1f:36", portId: "c0:74:ad:24:1f:36", portDescr: "eth0", mgmtIp: "192.168.13.121", capabilities: "bridge, telephone", description: "GXP1625 1.0.11.138" }, { protocol: "lldp", localPort: "48", sysName: "SW-DIGOMI-FL2", chassisId: "ec:50:aa:71:87:40", portId: "1/1/3", mgmtIp: "192.168.13.2", capabilities: "bridge, router", description: "Aruba R8N87A PL.10.13.1080" }],
    env: [{ kind: "fan", name: "Fan 1", status: "OK", ok: true }, { kind: "fan", name: "Fan 2", status: "OK", ok: true }, { kind: "psu", name: "Main", status: "OK", ok: true }, { kind: "temp", name: "Sensor", status: "61°C, OK", ok: true }],
    logs: [{ severity: "I", text: "01/28/26 03:05:45 %AAA-I-CONNECT: New http connection for user admin" }, { severity: "W", text: "01/28/26 03:01:53 %LINK-W-Down:  11" }], errors: [] };
  const detail = (id) => ({ ...views.find((v) => v.id === id), snapshot: id === "acc4" || id === "io3" ? null : id === "io1" ? ioSnap : id === "io2" ? webSnap : snap, backups: [{ name: "20261023-210400.cfg", at: mins(4), size: 2400 }, { name: "20261019-081200.cfg", at: mins(6000), size: 2300 }] });
  window.go = { main: { App: {
    Platform: async () => "darwin", Version: async () => "0.3.0",
    GetDashboard: async () => dash, GetSwitch: async (id) => detail(id), GetTopology: async () => topo, SaveLayout: async () => {},
    GetSettings: async () => ({ checkMethod: "port", firmwareAuto: "install", windowStart: "02:00", windowEnd: "05:00", pingInterval: 30, collectInterval: 15, parallel: 4, keepBackups: 20, cpuWarn: 80, memWarn: 90 }),
    Credentials: async () => [{ id: "c1", name: "Network admins", username: "admin" }, { id: "c2", name: "noc@example.ge", username: "noc@example.ge", kind: "instanton" }],
    DiscoverCloud: async () => { await new Promise((r) => setTimeout(r, 400)); return [
      { id: "s1", name: "Office", switches: [{ deviceId: "94:60:d5:12:34:56", name: "office-1930", model: "JL682A", ip: "10.10.2.5", online: true, ports: 28, addedAs: "io1" }, { deviceId: "94:60:d5:12:34:57", name: "office-1930-b", model: "JL682A", ip: "10.10.2.6", online: true, ports: 28 }] },
      { id: "s2", name: "Warehouse Rustavi", switches: [{ deviceId: "94:60:d5:22:00:01", name: "wh-1960-core", model: "JL808A", ip: "192.168.50.2", online: true, ports: 28 }, { deviceId: "94:60:d5:22:00:02", name: "wh-1830-dock", model: "JL812A", ip: "192.168.50.3", online: false, ports: 10 }] }]; },
    AddCloud: async (cid, picks) => picks.length,
    UseWebInterface: async () => 1, TestSwitch: async () => "Signed in to SW-BRANCH-114 (HPE Networking Instant On Switch 48p Gigabit CL4 PoE 4p SFP+ 370W 1930 JL686A)",
    VLANOverview: async () => views.filter((v) => v.summary?.ports?.length).map((v, i) => ({ id: v.id, name: v.name, online: true, family: v.summary.family, editable: v.summary.family !== "Instant On", readOnly: v.summary.family === "Instant On" ? "changed on the switch's web page" : "", vlans: vlans.filter((x, j) => (i + j) % 4 !== 3).map((x) => (v.id === "io1" && x.id === 30 ? { ...x, name: "VOIP" } : x)) })),
    GetBackup: async () => cfg.replace("aaa authentication ssh login radius local\n", ""),
    PreviewVLAN: async (id, ch) => ch.action === "delete" ? ["configure terminal", `no vlan ${ch.id}`, "end", ch.save ? "write memory" : null].filter(Boolean) : ["configure terminal", `vlan ${ch.id}`, ch.name ? `name "${ch.name}"` : null, ch.addUntagged?.length ? "untagged " + ch.addUntagged.join(",") : null, ch.addTagged?.length ? "tagged " + ch.addTagged.join(",") : null, "exit", "end", ch.save ? "write memory" : null].filter(Boolean),
    ApplyVLAN: async () => ({ ok: true, transcript: ["> configure terminal", "> vlan 50", "> exit"] }),
    PreviewPortAccess: async (id, ch) => ["configure terminal",
      ch.addServer ? `radius-server host ${ch.addServer.host} key "********"` : null,
      ch.removeServer ? `no radius-server host ${ch.removeServer}` : null,
      ch.setRole ? `port-access role ${ch.setRole.name}` : null, ch.setRole?.vlan ? `vlan access ${ch.setRole.vlan}` : null, ch.setRole ? "exit" : null,
      ch.deleteRole ? `no port-access role ${ch.deleteRole}` : null,
      ch.dot1x === true ? `aaa port-access authenticator ${ch.ports.join(",")}` : ch.dot1x === false ? `no aaa port-access authenticator ${ch.ports.join(",")}` : null,
      ch.macAuth === true ? `aaa port-access mac-based ${ch.ports.join(",")}` : ch.macAuth === false ? `no aaa port-access mac-based ${ch.ports.join(",")}` : null,
      ch.clientLimit !== null && ch.clientLimit !== undefined ? `aaa port-access authenticator ${ch.ports.join(",")} client-limit ${ch.clientLimit}` : null,
      ch.globalDot1x === true ? "aaa port-access authenticator active" : ch.globalDot1x === false ? "no aaa port-access authenticator active" : null,
      "end", ch.save ? "write memory" : null].filter(Boolean),
    ApplyPortAccess: async () => ({ ok: true, transcript: ["> configure terminal", "> aaa port-access authenticator 8", "> end"] }),
    RefreshPortClients: async () => [{ port: "1/1/15", mac: "00:0c:29:9b:cc:d0", user: "jdoe", ip: "10.3.0.21", method: "dot1x", status: "Authenticated", role: "RoleLan", roleStatus: "Applied", vlan: "10" }],
    RunCommand: async (id, c) => "  Status and Counters - VLAN Information\n\n  VLAN ID Name     | Status\n  1       DEFAULT  | Port-based",
    RefreshAll: async () => {}, Refresh: async () => {},
    DataFolder: async () => ({ path: "/Users/jaba/Library/Application Support/SwitchDesk", defaultPath: "/Users/jaba/Library/Application Support/SwitchDesk", custom: false }),
    PickDataFolder: async () => ({ path: "/Users/jaba/Dropbox/SwitchDesk", hasData: false, switches: 0, writable: true, same: false }),
    PickImportFile: async () => "/Users/jaba/Downloads/SwitchDesk-2026-10-23.sdbak",
    InspectImport: async () => ({ switches: 7, createdAt: new Date().toISOString(), history: true }),
    StartupNotice: async () => "",
    LastUpdateCheck: async () => ({ current: "0.3.0", latest: "0.4.0", available: true, url: "https://github.com/jaba0x/SwitchDesk/releases/tag/v0.4.0" }),
    CheckForUpdate: async () => ({ current: "0.3.0", latest: "0.4.0", available: true, url: "https://github.com/jaba0x/SwitchDesk/releases/tag/v0.4.0" }),
    OpenURL: async () => {},
    FirmwareImages: async () => [
      { file: "WC_16_11_0021.swi", version: { branch: "WC", text: "WC.16.11.0021" }, size: 33554432, addedAt: new Date(Date.now() - 864e5).toISOString(), source: "file" },
      { file: "ArubaOS-CX_6400-6300_10_14_0001.swi", version: { branch: "FL", text: "FL.10.14.0001" }, size: 681574400, addedAt: new Date(Date.now() - 3600e3).toISOString(), source: "networkingsupport.hpe.com" },
      { file: "YA_16_11_0019.swi", version: { branch: "YA", text: "YA.16.11.0019" }, size: 21000000, addedAt: new Date(Date.now() - 8e8).toISOString(), source: "file" }],
    UpgradeJobs: async () => [
      { id: "j1", switchName: "acc-2930-f1", from: "WC.16.11.0015", to: "WC.16.11.0021", reboot: true, state: "running", progress: 0.42, step: "Copying WC_16_11_0021.swi to the secondary bank from 10.10.1.50", log: ["23:41:02  Reading firmware banks", "23:41:03  Primary WC.16.11.0015, secondary WC.16.10.0012, running from primary", "23:41:03  Copying WC_16_11_0021.swi to the secondary bank from 10.10.1.50"] },
      { id: "j2", switchName: "acc-2930-f2", from: "WC.16.11.0015", to: "WC.16.11.0021", reboot: false, auto: true, state: "staged", step: "Ready: WC.16.11.0021 boots on the next restart" },
      { id: "j3", switchName: "core-cx-01", from: "FL.10.12.1010", to: "FL.10.13.1000", reboot: true, state: "done", step: "Updated to FL.10.13.1000" }],
    StartUpgrade: async (ids) => ids.map((id) => ({ id })), ClearFinishedUpgrades: async () => {}, OpenFirmwareFolder: async () => {}, SetDisabled: async () => {}, DeleteSwitches: async () => {}, OpenDataFolder: async () => {},
  } } };
  window.runtime = { EventsOn: () => {} };
})();
