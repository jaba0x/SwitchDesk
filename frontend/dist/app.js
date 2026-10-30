"use strict";
/* SwitchDesk UI. Talks to the Go side through window.go.main.App (Wails bindings). */

let api = null;
const S = { dash: null, detail: null, creds: [], route: [], tab: "overview", filter: "all", query: "", modalOpen: false, consoleHist: [], showEndpoints: false };

// ---------- helpers ----------
const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];
const esc = (v) => String(v ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const plural = (n, w) => `${n} ${w}${n === 1 ? "" : "s"}`;

function ago(t) {
  if (!t || t.startsWith("0001")) return "never";
  const s = Math.round((Date.now() - new Date(t)) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.round(s / 60)} min ago`;
  if (s < 86400) return `${Math.round(s / 3600)} h ago`;
  return new Date(t).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

const ICON = {
  dash: '<svg viewBox="0 0 24 24"><rect x="3" y="4" width="18" height="6" rx="1.5"/><rect x="3" y="14" width="18" height="6" rx="1.5"/><path d="M7 7h.01M10 7h.01M7 17h.01M10 17h.01"/></svg>',
  list: '<svg viewBox="0 0 24 24"><path d="M8 6h13M8 12h13M8 18h13M3.5 6h.01M3.5 12h.01M3.5 18h.01"/></svg>',
  map: '<svg viewBox="0 0 24 24"><circle cx="5" cy="12" r="2.5"/><circle cx="19" cy="5" r="2.5"/><circle cx="19" cy="19" r="2.5"/><path d="M7.3 11 16.7 6M7.3 13l9.4 5"/></svg>',
  vlan: '<svg viewBox="0 0 24 24"><path d="M4 6h16M4 12h16M4 18h16"/><path d="M8 4v4M14 10v4M11 16v4"/></svg>',
  gear: '<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H9a1.6 1.6 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V9a1.6 1.6 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1z"/></svg>',
  refresh: '<svg viewBox="0 0 24 24"><path d="M21 12a9 9 0 1 1-2.6-6.4M21 4v5h-5"/></svg>',
  plus: '<svg viewBox="0 0 24 24"><path d="M12 5v14M5 12h14"/></svg>',
  back: '<svg viewBox="0 0 24 24"><path d="M15 18l-6-6 6-6"/></svg>',
  pause: '<svg viewBox="0 0 24 24"><path d="M9 5v14M15 5v14"/></svg>',
  play: '<svg viewBox="0 0 24 24"><path d="M7 5l12 7-12 7z"/></svg>',
  trash: '<svg viewBox="0 0 24 24"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/></svg>',
  chip: '<svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" rx="1.5"/><path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4"/></svg>',
  up: '<svg viewBox="0 0 24 24"><path d="M12 19V5M6 11l6-6 6 6"/></svg>',
  search: '<svg viewBox="0 0 24 24"><circle cx="11" cy="11" r="6.5"/><path d="M20 20l-4.2-4.2"/></svg>',
  copy: '<svg viewBox="0 0 24 24"><rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15V6a2 2 0 0 1 2-2h8"/></svg>',
};

// copyText prefers the Wails clipboard: the web view's own clipboard API
// isn't available on every platform.
async function copyText(text) {
  try {
    if (window.runtime?.ClipboardSetText) {
      if (!(await window.runtime.ClipboardSetText(text))) throw new Error();
    } else await navigator.clipboard.writeText(text);
    toast(`Copied ${text}`);
  } catch {
    toast("Couldn't copy to the clipboard", true);
  }
}
// a value with a copy button; the click doesn't reach the row behind it
const copyable = (v) => `<span class="copyv"><span>${esc(v)}</span><button type="button" title="Copy ${esc(v)}" data-copy-text="${esc(v)}">${ICON.copy}</button></span>`;

function toast(text, err) {
  const t = document.createElement("div");
  t.className = "toast" + (err ? " err" : "");
  t.textContent = text;
  $("#toasts").appendChild(t);
  setTimeout(() => t.remove(), err ? 7000 : 3500);
}

async function call(fn, ...args) {
  try {
    return await api[fn](...args);
  } catch (e) {
    throw new Error(typeof e === "string" ? e : e?.message || String(e));
  }
}

// ---------- faceplate ----------
// SFP+SR, SFP+DA1, 1000SX, 1000LX, 100FX, 10GbE-LR…
function isFiber(p) { return /sfp|xfp|qsfp|fiber|dac|(sr|lr|lrm|er|sx|lx|zx|lh|bx|fx|ex)\d*$/i.test(p.type || "") || /^Trk|^lag/i.test(p.name); }

function leds(ports, stp, big, optics) {
  const opt = Object.fromEntries((optics || []).map((o) => [o.port, o]));
  if (!ports || !ports.length) return `<div class="leds none">No port data yet</div>`;
  const blocking = new Set((stp?.ports || []).filter((p) => /Blocking|Discarding/.test(p.state)).map((p) => p.port));
  const phys = ports.filter((p) => !/^(Trk|lag)/i.test(p.name));
  // Laid out like the hardware (issue #14): each stack member on its own,
  // and within it the copper ports, the SFP ports and module ports (A1…)
  // as separate blocks with a gap between them.
  const member = (n) => (n.split("/").length > 1 ? n.split("/")[0] : "");
  const members = [...new Set(phys.map((p) => member(p.name)))];
  const stacked = members.length > 1;
  const isMod = (p) => /(^|\/)[A-Z]\d+$/i.test(p.name);
  // Dual-personality (combo) ports, such as 21-24 on a 2930M 24G, report
  // copper or fiber depending on what's plugged in. When fiber shows up only
  // among a member's last four ports, those four stay one block whatever is
  // in them, so the layout doesn't change with the media.
  const tail = new Set();
  for (const m of members) {
    const ps = phys.filter((p) => member(p.name) === m && !isMod(p));
    const first = ps.findIndex(isFiber);
    if (ps.length > 8 && first >= 0 && first >= ps.length - 4) ps.slice(-4).forEach((p) => tail.add(p));
  }
  const groups = [];
  for (const p of phys) {
    const m = member(p.name), mod = isMod(p), t = tail.has(p), f = t || mod || isFiber(p);
    const last = groups[groups.length - 1];
    if (!last || last.member !== m || last.fiber !== f || last.mod !== mod || last.tail !== t) groups.push({ member: m, fiber: f, mod, tail: t, ports: [], first: !last || last.member !== m });
    groups[groups.length - 1].ports.push(p);
  }
  const html = groups.map((g) => {
    const rows = g.ports.length > 8 ? 2 : 1;
    const cells = g.ports.map((p) => {
      const cls = !p.enabled ? "x" : p.loop ? "l" : blocking.has(p.name) ? "b" : p.up ? "u" : "";
      const tip = `Port ${p.name}${p.description ? " · " + p.description : ""}: ${!p.enabled ? "disabled" : p.up ? "up" : "down"}${p.speed ? " " + p.speed : ""}${p.vlan ? ", VLAN " + p.vlan : ""}${blocking.has(p.name) ? ", STP blocking" : ""}${p.loop ? ", loop detected" : ""}${opt[p.name]?.rx?.length ? `, Rx ${opt[p.name].rx.map(dbm).join("/")} dBm` : ""}${p.poeMw ? `, PoE ${(p.poeMw / 1000).toFixed(1)} W` : ""}${p.uplink ? ", uplink" : ""}${p.trunk ? ", " + I18N.tr("trunk") + " " + p.trunk : ""}`;
      return `<i class="${cls}" title="${esc(tip)}"></i>`;
    }).join("");
    const grp = `<span class="ledgrp" style="--r:${rows}" title="${g.mod ? "Module ports" : g.tail ? "Combo and SFP ports" : g.fiber ? "SFP ports" : ""}">${cells}</span>`;
    return stacked && g.first ? `${groups[0] === g ? "" : "</div></div>"}<div class="ledmember">${big ? `<span class="ledmlabel">Member ${esc(g.member)}</span>` : ""}<div class="ledrow">${grp}` : grp;
  }).join("");
  return `<div class="leds${big ? " big" : ""}${stacked ? " stack" : ""}">${stacked ? html + "</div></div>" : html}</div>`;
}

// what the faceplate colours mean
function ledKey(ports) {
  const k = [["u", "Up"], ["", "Down"], ["x", "Disabled"], ["b", "STP blocking"]];
  if ((ports || []).some((p) => p.loop)) k.push(["l", "Loop"]);
  return `<span class="ledkey">${k.map(([c, l]) => `<span><i class="${c}"></i>${l}</span>`).join("")}</span>`;
}

// "12/48", plus how many of the rest are disabled rather than down
function portsUp(s) {
  const off = (s.ports || []).filter((p) => !p.enabled).length;
  return `${s.portsUp}/${s.portsTotal}${off ? ` <span class="faint" title="${plural(off, "port")} disabled on the switch">· ${off} disabled</span>` : ""}`;
}

// ---------- shell ----------
function renderSide() {
  const r = S.route[0] || "dashboard";
  const crit = S.dash?.critical || 0;
  const fwCount = (S.dash?.switches || []).filter((v) => !v.disabled && v.firmware?.available && !v.firmware?.staged).length;
  const item = (key, icon, label, extra = "") =>
    `<button class="nav${r === key || (key === "switches" && r === "switch") ? " on" : ""}" data-go="#/${key}">${ICON[icon]}<span>${label}</span>${extra}</button>`;
  $("#side").innerHTML = `
    <div class="brand"><span class="brand-mark">${"<i></i>".repeat(8)}</span><b>SwitchDesk</b></div>
    ${item("dashboard", "dash", "Dashboard", crit ? `<span class="count">${crit}</span>` : "")}
    ${item("switches", "list", "Switches")}
    ${item("map", "map", "Network map")}
    ${item("vlans", "vlan", "VLANs")}
    ${item("firmware", "chip", "Firmware", fwCount ? `<span class="count info">${fwCount}</span>` : "")}
    ${item("settings", "gear", "Settings")}
    <div class="side-foot">${S.dash ? `${S.dash.online} of ${S.dash.total} online` : ""}${S.dash?.collecting ? `<br>Collecting from ${plural(S.dash.collecting, "switch").replace("switchs", "switches")}` : ""}
      <a class="credit" onclick="openURL('https://clihero.com')" title="clihero.com">
        <img src="clihero-logo-white.png" alt="clihero.com">
        <span>Built by Jaba Macharashvili</span></a></div>`;
}

// URLs go through a data attribute, never into inline JavaScript
document.addEventListener("click", (e) => {
  const u = e.target.closest("[data-open-url]");
  if (u) openURL(u.dataset.openUrl);
  const c = e.target.closest("[data-copy-text]");
  if (c) { e.stopPropagation(); copyText(c.dataset.copyText); }
  // port access actions: names read from the switch stay out of inline JavaScript
  const a = e.target.closest("[data-pa]");
  if (a) {
    e.stopPropagation();
    const fn = { removeRadius, paEditOne, openRoleEditor, deleteRole }[a.dataset.pa];
    if (fn) fn(a.dataset.arg);
  }
});

document.addEventListener("click", (e) => {
  if (e.target.closest(".nogo")) return; // checkboxes and row buttons don't open the switch
  const go = e.target.closest("[data-go]");
  if (go) { e.preventDefault(); location.hash = go.dataset.go; }
});

window.addEventListener("hashchange", route);

async function route() {
  try {
    await showRoute();
  } catch (e) {
    showError("Opening " + (location.hash || "#/dashboard"), e);
  }
}

async function showRoute() {
  // a dialog belongs to the page it was opened on
  if (S.modalOpen) { $(".veil")?.remove(); S.modalOpen = false; }
  const h = location.hash.replace(/^#\/?/, "") || "dashboard";
  S.route = h.split("/").map(decodeURIComponent);
  if (S.route[0] !== "map") SDMap.stop();
  renderSide();
  const [page, a, b] = S.route;
  const main = $("#main");
  main.scrollTop = 0;
  if (page === "switch") {
    S.tab = b || "overview";
    await loadDetail(a);
  } else if (page === "switches") renderSwitches();
  else if (page === "map") renderMap();
  else if (page === "vlans") renderVLANs();
  else if (page === "firmware") renderFirmware();
  else if (page === "settings") renderSettings();
  else renderDashboard();
}

async function refreshData() {
  S.dash = await call("GetDashboard");
  renderSide();
  if (S.modalOpen) return;
  const page = S.route[0] || "dashboard";
  if (page === "dashboard") renderDashboard();
  if (page === "switches") renderSwitches(true);
  if (page === "switch" && S.detail) {
    // these tabs follow every change; the others reload once a collection brings new data,
    // except where a redraw would throw away what the user is doing
    const live = ["overview", "stp", "ports", "neighbors", "logs"].includes(S.tab);
    const v = S.dash?.switches.find((x) => x.id === S.detail.id);
    const fresh = v?.summary?.collectedAt && v.summary.collectedAt !== S.detail.summary?.collectedAt;
    if (live || (fresh && !["console", "config"].includes(S.tab))) loadDetail(S.route[1], true);
    updateDetailHead();
  }
  if (page === "map") SDMap.reload();
  if (page === "firmware") renderFirmware(true);
  if (page === "vlans") renderVLANs(true);
}

// ---------- dashboard ----------
function renderDashboard() {
  const d = S.dash;
  if (!d) return;
  const main = $("#main");
  if (!d.total) {
    main.innerHTML = `<div class="page"><div class="head"><h1>Dashboard</h1></div>
      <div class="panel"><div class="empty"><h3>No switches yet</h3>
      <p>Add your switches by address. SwitchDesk logs in over SSH, reads their state and keeps checking them.</p>
      <button class="btn primary" onclick="openSwitchModal()">${ICON.plus}Add switch</button>
      <button class="btn" onclick="openBulkModal()">Add many</button></div></div></div>`;
    return;
  }
  const pct = (n) => (d.total ? (n / d.total) * 100 : 0);
  const cats = {};
  for (const i of (d.issues || []).filter(sevMatch)) cats[i.category] = (cats[i.category] || 0) + 1;
  if (S.icat && !cats[S.icat]) S.icat = "";
  const refocus = document.activeElement?.id === "iq" ? [document.activeElement.selectionStart, document.activeElement.selectionEnd] : null;
  const chip = (k, label, n) => `<button class="chip${S.filter === k ? " on" : ""}" onclick="S.filter='${k}';renderDashboard()">${label}${n !== undefined ? ` ${n}` : ""}</button>`;
  const bySev = { critical: 0, warning: 1, info: 2, "": 3 };
  // offline first, then by worst problem; disabled switches go last
  const rank = (v) => (v.disabled ? 2 : v.status.checked && !v.status.online ? 0 : 1);
  const plates = [...d.switches].sort((a, b) => rank(a) - rank(b) || bySev[a.worst || ""] - bySev[b.worst || ""]);
  main.innerHTML = `<div class="page">
    <div class="head"><h1>Dashboard</h1><span class="grow"></span>
      <button class="btn" onclick="refreshAll(this)">${ICON.refresh}Collect now</button></div>
    ${d.loading ? `<div class="panel" style="padding:10px 14px;margin-bottom:14px" role="status">Loading saved data… ${d.total - d.loading} of ${d.total} switches</div>` : ""}
    <div class="fleet">
      <div class="fleet-line">
        <span class="stat" data-go="#/switches"><span class="big">${d.online}</span><span class="lbl">online</span></span>
        <span class="stat" data-go="#/switches"><span class="big ${d.offline ? "bad" : ""}">${d.offline}</span><span class="lbl">offline</span></span>
        ${d.unknown ? `<span><span class="big faint">${d.unknown}</span><span class="lbl">not checked yet</span></span>` : ""}
        ${d.disabled ? `<span class="stat" data-go="#/switches"><span class="big faint">${d.disabled}</span><span class="lbl">disabled</span></span>` : ""}
        <span><span class="big ${d.critical ? "bad" : ""}">${d.critical}</span><span class="lbl">critical</span></span>
        <span><span class="big ${d.warning ? "warnc" : ""}">${d.warning}</span><span class="lbl">warnings</span></span>
      </div>
      <div class="bar" title="${d.online} online, ${d.offline} offline"><i style="width:${pct(d.online)}%;background:var(--up)"></i><i style="width:${pct(d.offline)}%;background:var(--crit)"></i></div>
    </div>
    <div class="dash">
      <section class="panel">
        <h2>Problems<span class="grow"></span><span class="chips">${chip("all", "Critical and warnings")}${chip("critical", "Critical", d.critical)}${chip("warning", "Warnings", d.warning)}${chip("info", "Notes", d.info)}</span></h2>
        <div class="ifilter"><span class="searchbox">${ICON.search}<input type="text" id="iq" placeholder="Filter problems  /" value="${esc(S.iq || "")}" title="Matches the problem, the switch name and the details, e.g. telnet, fan, core, optical"></span>
          <select id="icat"><option value="">All categories</option>${Object.keys(cats).sort().map((c) => `<option value="${esc(c)}" ${c === S.icat ? "selected" : ""}>${esc(c)} (${cats[c]})</option>`).join("")}</select>
          <span class="faint" id="icount"></span></div>
        <div class="issues-scroll" id="issuelist"></div>
      </section>
      <section class="panel">
        <h2>Switches<span class="grow"></span>${ledKey(d.switches.flatMap((v) => v.summary?.ports || []))}<span style="width:16px"></span><a data-go="#/switches" class="muted" style="font-weight:400;font-size:13px">All switches</a></h2>
        <div class="rack issues-scroll">${plates.map(plate).join("")}</div>
      </section>
    </div></div>`;
  renderIssueList();
  const iq = $("#iq");
  iq.oninput = () => { S.iq = iq.value; renderIssueList(); };
  iq.onkeydown = (e) => { if (e.key === "Escape" && iq.value) { e.stopPropagation(); iq.value = S.iq = ""; renderIssueList(); } };
  $("#icat").onchange = (e) => { S.icat = e.target.value; renderIssueList(); };
  if (refocus) { iq.focus(); iq.setSelectionRange(refocus[0], refocus[1]); }
}

// the problems list on the dashboard: severity chips, a category and free text
function sevMatch(i) { return S.filter === "all" || !S.filter ? i.severity !== "info" : i.severity === S.filter; }
function renderIssueList() {
  const box = $("#issuelist");
  if (!box || !S.dash) return;
  const words = (S.iq || "").toLowerCase().split(/\s+/).filter(Boolean);
  const all = (S.dash.issues || []).filter(sevMatch);
  const list = all.filter((i) => (!S.icat || i.category === S.icat) &&
    words.every((w) => `${i.title} ${i.switchName} ${i.detail} ${i.category}`.toLowerCase().includes(w)));
  const filtered = words.length || S.icat;
  box.innerHTML = list.length ? list.map(issueRow).join("") : `<div class="empty"><h3>${filtered ? "No matches" : "Nothing to fix"}</h3><p>${filtered ? "No problems match this filter." : S.filter === "all" ? "No critical problems or warnings across your switches." : "No items in this category."}</p>${filtered ? `<button class="btn small" onclick="S.iq='';S.icat='';renderDashboard()">Clear filter</button>` : ""}</div>`;
  $("#icount").textContent = filtered ? `${list.length} of ${all.length}` : "";
}

function issueRow(i) {
  const go = i.switchId ? `data-go="#/switch/${i.switchId}"` : "";
  return `<div class="issue" ${go}><div><span class="sev ${i.severity}">${{ critical: "Critical", warning: "Warning", info: "Note" }[i.severity]}</span></div>
    <div><div><span class="t">${esc(i.title)}</span> <span class="w">on ${esc(i.switchName)}</span></div>${i.detail ? `<div class="d">${esc(i.detail)}</div>` : ""}</div></div>`;
}

function statusDot(v) {
  if (!v.status.checked) return `<span class="dot" title="Not checked yet"></span>`;
  return v.status.online ? `<span class="dot up" title="Online"></span>` : `<span class="dot down" title="Offline"></span>`;
}

function plate(v) {
  const s = v.summary;
  const name = v.name || s?.hostname || v.host;
  const meta = v.disabled ? `<span class="faint">disabled</span>` : !v.status.online && v.status.checked ? `<span class="bad">offline ${ago(v.status.since)}</span>` :
    `${esc(v.host)}${v.status.latencyMs ? ` <span class="faint">${v.status.latencyMs} ms</span>` : ""}`;
  return `<div class="plate${v.disabled ? " off" : ""}" data-go="#/switch/${v.id}">
    <div class="nm">${v.disabled ? `<span class="dot"></span>` : statusDot(v)}<b>${esc(name)}</b>${v.worst ? `<span class="sev ${v.worst}">${(v.issues || []).length}</span>` : ""}${v.status.collecting ? `<span class="faint" style="font-size:12px">collecting</span>` : ""}</div>
    <div class="meta">${meta}</div>
    <div class="leds-wrap" style="grid-column:1/-1">${s?.ports?.length ? leds(s.ports, { ports: s.ports.filter((p) => p.blocking).map((p) => ({ port: p.name, state: "Blocking" })) }) : s ? `<div class="faint" style="font-size:12.5px">${esc(s.model || s.family)} ${s.portsTotal ? `· ${portsUp(s)} ports up` : ""}</div>` : `<div class="faint" style="font-size:12.5px">No data collected yet</div>`}</div>
  </div>`;
}

async function refreshAll(btn) {
  btn?.classList.add("spin");
  await call("RefreshAll");
  toast("Collecting from all online switches");
  setTimeout(() => btn?.classList.remove("spin"), 1500);
}

// ---------- switches list ----------
// search matches name, address, site, model, software, family and status words
function switchMatches(v, q) {
  if (!q) return true;
  const s = v.summary || {};
  const state = v.disabled ? "disabled paused" : !v.status.checked ? "unknown" : v.status.online ? "online up" : "offline down";
  const hay = [v.name, v.host, v.site, v.notes, s.hostname, s.model, s.software, s.family, s.serial, state, v.worst, v.worst ? "problem" : ""].join(" ").toLowerCase();
  return q.toLowerCase().split(/\s+/).filter(Boolean).every((w) => hay.includes(w));
}

S.sel = new Set();
const switchesWord = (n) => `${n} switch${n === 1 ? "" : "es"}`;

function renderSwitches(soft) {
  const d = S.dash;
  if (!d) return;
  const rows = d.switches.filter((v) => switchMatches(v, S.query));
  const ids = new Set(d.switches.map((v) => v.id));
  for (const id of S.sel) if (!ids.has(id)) S.sel.delete(id);
  const allOn = rows.length > 0 && rows.every((v) => S.sel.has(v.id));
  const table = `<table class="swlist"><thead><tr>
      <th class="cb nogo"><input type="checkbox" title="Select all shown" ${allOn ? "checked" : ""} onchange="selectAll(this.checked)"></th>
      <th></th><th>Name</th><th>Address</th><th>Site</th><th>Model</th><th>Software</th><th>Firmware</th><th class="num">CPU</th><th class="num">Mem</th><th class="num">Ports up</th><th>STP</th><th>Problems</th><th>Collected</th><th></th></tr></thead><tbody>
    ${rows.map((v) => {
      const s = v.summary || {};
      return `<tr class="click${v.disabled ? " off" : ""}${S.sel.has(v.id) ? " picked" : ""}" data-go="#/switch/${v.id}">
      <td class="cb nogo"><input type="checkbox" ${S.sel.has(v.id) ? "checked" : ""} onchange="toggleSel('${v.id}', this.checked)"></td>
      <td>${v.disabled ? `<span class="dot" title="Disabled"></span>` : statusDot(v)}</td>
      <td>${esc(v.name || s.hostname || "")}${v.disabled ? ` <span class="tag">disabled</span>` : ""}</td><td class="mono">${esc(v.host)}</td><td>${esc(v.site)}</td>
      <td title="${esc(s.family || "")}">${esc(s.model || "")}</td><td class="mono">${esc(s.software || "")}</td><td>${fwCell(v)}</td>
      <td class="num">${s.cpu >= 0 && s.cpu !== undefined ? s.cpu + "%" : ""}</td><td class="num">${s.memory >= 0 && s.memory !== undefined ? s.memory + "%" : ""}</td>
      <td class="num">${s.portsTotal ? portsUp(s) : ""}</td><td>${esc(s.stpRole || "")}</td>
      <td>${v.worst ? `<span class="sev ${v.worst}">${(v.issues || []).length}</span>` : ""}</td><td class="faint">${v.status.collecting ? "now" : ago(s.collectedAt)}</td>
      <td class="acts nogo">${canUpdate(v) ? `<button class="ib" title="Update firmware to ${esc(v.firmware.available)}" onclick="openUpgradeDialog(['${v.id}'])">${ICON.up}</button>` : ""}<button class="ib" title="${v.disabled ? "Enable" : "Disable"}" onclick="setDisabled(['${v.id}'], ${!v.disabled})">${v.disabled ? ICON.play : ICON.pause}</button><button class="ib danger" title="Delete" onclick="deleteSwitches(['${v.id}'])">${ICON.trash}</button></td></tr>`;
    }).join("")}</tbody></table>
    ${rows.length ? "" : `<div class="empty">No switches match “${esc(S.query)}”. <a onclick="clearSearch()">Clear search</a></div>`}`;
  const n = S.sel.size;
  const bar = n ? `<div class="selbar"><b>${n} selected</b><span class="grow"></span>
      <button class="btn small" onclick="refreshSel()">${ICON.refresh}Collect now</button>
      <button class="btn small" onclick="openUpgradeDialog([...S.sel])">${ICON.up}Update firmware</button>
      <button class="btn small" onclick="setDisabled([...S.sel], true)">${ICON.pause}Disable</button>
      <button class="btn small" onclick="setDisabled([...S.sel], false)">${ICON.play}Enable</button>
      <button class="btn small danger" onclick="deleteSwitches([...S.sel])">${ICON.trash}Delete</button>
      <button class="btn small" onclick="S.sel.clear();renderSwitches(true)">Clear</button></div>` : "";
  const counts = `${d.total}${d.disabled ? ` <span class="faint">(${d.disabled} disabled)</span>` : ""}${S.query ? ` <span class="faint">${rows.length} shown</span>` : ""}`;
  if (soft && $("#swtable")) {
    $("#swtable").innerHTML = d.total ? table : emptyList();
    $("#selbar").innerHTML = bar;
    $("#swcount").innerHTML = counts;
    return;
  }
  $("#main").innerHTML = `<div class="page">
    <div class="head"><h1>Switches</h1><span class="sub" id="swcount">${counts}</span><span class="grow"></span>
      <span class="searchbox">${ICON.search}<input type="text" id="swsearch" placeholder="Search switches  /" value="${esc(S.query)}" oninput="S.query=this.value;renderSwitches(true)" title="Name, address, site, model or software. Also: online, offline, disabled, problem"></span>
      <button class="btn" onclick="openBulkModal()">Add many</button>
      <button class="btn primary" onclick="openSwitchModal()">${ICON.plus}Add switch</button></div>
    <div id="selbar">${bar}</div>
    <div class="panel" id="swtable">${d.total ? table : emptyList()}</div></div>`;
}

function emptyList() {
  return `<div class="empty"><h3>No switches yet</h3><p>Add one by address, or paste a list with Add many.</p></div>`;
}

function clearSearch() {
  S.query = "";
  if ($("#swsearch")) $("#swsearch").value = "";
  renderSwitches(true);
}

function toggleSel(id, on) {
  on ? S.sel.add(id) : S.sel.delete(id);
  renderSwitches(true);
}

function selectAll(on) {
  for (const v of S.dash.switches.filter((v) => switchMatches(v, S.query))) on ? S.sel.add(v.id) : S.sel.delete(v.id);
  renderSwitches(true);
}

function nameOf(id) {
  const v = S.dash.switches.find((x) => x.id === id);
  return v ? v.name || v.summary?.hostname || v.host : id;
}

async function setDisabled(ids, disabled) {
  if (!ids.length) return;
  try {
    await call("SetDisabled", ids, disabled);
    toast(`${disabled ? "Disabled" : "Enabled"} ${ids.length === 1 ? nameOf(ids[0]) : switchesWord(ids.length)}`);
    S.dash = await call("GetDashboard");
    renderSide();
    if ((S.route[0] || "") === "switches") renderSwitches(true);
    else if (S.route[0] === "switch") loadDetail(S.route[1]);
  } catch (e) { toast(e.message, true); }
}

async function deleteSwitches(ids) {
  if (!ids.length) return;
  const what = ids.length === 1 ? nameOf(ids[0]) : switchesWord(ids.length);
  if (!(await askConfirm(`Delete ${what} from SwitchDesk, with their saved configs? The switches themselves aren't changed.`))) return;
  try {
    await call("DeleteSwitches", ids);
    ids.forEach((id) => S.sel.delete(id));
    toast(`Deleted ${what}`);
    S.dash = await call("GetDashboard");
    renderSide();
    if (S.route[0] === "switch") location.hash = "#/switches";
    else renderSwitches(true);
  } catch (e) { toast(e.message, true); }
}

async function refreshSel() {
  const ids = [...S.sel].filter((id) => !S.dash.switches.find((v) => v.id === id)?.disabled);
  for (const id of ids) await call("Refresh", id);
  toast(`Collecting from ${switchesWord(ids.length)}`);
}

// ---------- switch detail ----------
async function loadDetail(id, soft) {
  try {
    S.detail = await call("GetSwitch", id);
  } catch (e) {
    $("#main").innerHTML = `<div class="page"><div class="empty"><h3>Switch not found</h3><a data-go="#/switches">Back to switches</a></div></div>`;
    return;
  }
  renderDetail(soft);
}

function updateDetailHead() {
  const v = S.dash?.switches.find((x) => x.id === S.detail.id);
  if (v && $("#detail-status")) $("#detail-status").innerHTML = detailStatus(v);
}

function detailStatus(v) {
  if (v.disabled) return `<span class="faint">Disabled</span>`;
  if (v.status.collecting) return `<span class="faint">Collecting…</span>`;
  if (!v.status.checked) return `<span class="faint">${S.settings?.checkMethod === "off" ? "Not checked" : "Checking…"}</span>`;
  if (v.cloud) return v.status.online ? `<span class="ok">Online</span> <span class="faint">via Instant On</span>` : `<span class="bad">Offline since ${new Date(v.status.since).toLocaleString()}</span>`;
  return v.status.online ? `<span class="ok">Online</span> <span class="faint">${v.status.latencyMs} ms</span>` : `<span class="bad">Offline since ${new Date(v.status.since).toLocaleString()}</span>`;
}

const TABS = [["overview", "Overview"], ["stp", "Spanning tree"], ["ports", "Ports"], ["vlans", "VLANs"], ["access", "Port access"], ["neighbors", "Neighbors"], ["config", "Config"], ["logs", "Logs"], ["console", "Console"], ["raw", "Raw output"]];
// Instant On cloud switches: only what the portal provides
const CLOUD_TABS = ["overview", "ports", "neighbors"];
// Instant On in local management: everything the web interface reports
const WEB_TABS = ["overview", "stp", "ports", "vlans", "neighbors", "logs"];
const tabsFor = (d) => (d.cloud ? TABS.filter(([k]) => CLOUD_TABS.includes(k)) : d.web ? TABS.filter(([k]) => WEB_TABS.includes(k)) : TABS);

function renderDetail(soft) {
  const d = S.detail, snap = d.snapshot, s = d.summary;
  const name = d.name || s?.hostname || d.host;
  const counts = { vlans: snap?.vlans?.length, access: snap?.portAccess?.ports?.length, neighbors: snap?.neighbors?.length, ports: snap?.ports?.length };
  if ((d.cloud && !CLOUD_TABS.includes(S.tab)) || (d.web && !WEB_TABS.includes(S.tab))) S.tab = "overview";
  const body = snap ? tabBody(S.tab, d, snap) : noDataYet(d);
  const tabs = tabsFor(d).map(([k, l]) => `<button class="tab${S.tab === k ? " on" : ""}" data-go="#/switch/${d.id}/${k}">${l}${counts[k] ? `<span class="n">${counts[k]}</span>` : ""}</button>`).join("");
  if (soft && $("#tabbody")) {
    $("#tabbody").innerHTML = body;
    $("#detail-status").innerHTML = detailStatus(d);
    $(".tabs").innerHTML = tabs;
    return;
  }
  const banners = [];
  if (d.disabled) banners.push(`<div class="banner" style="background:rgba(139,149,167,.08);border-color:var(--line)"><div class="grow"><b>This switch is disabled.</b> SwitchDesk doesn't check or read it and raises no problems for it. The data below is from its last reading.</div><button class="btn" onclick="setDisabled(['${d.id}'], false)">${ICON.play}Enable</button></div>`);
  if (d.status.hostKeyChanged) banners.push(`<div class="banner"><div class="grow"><b>${d.web ? "The switch's identity changed." : "The SSH host key changed."}</b> ${d.web ? "It presented a different HTTPS certificate, or stopped offering encrypted login." : ""} SwitchDesk stopped connecting to protect your credentials. If this switch was replaced or reset, trust the new key.</div><button class="btn" onclick="trustKey()">Trust new key</button></div>`);
  if (!d.web && !d.cloud && d.family === "instant-on") {
    const others = (S.dash?.switches || []).filter((v) => !v.web && !v.cloud && v.family === "instant-on").length;
    banners.push(`<div class="banner"><div class="grow"><b>Instant On switches have no SSH.</b> Read this one through its web interface instead, with the same login.</div><button class="btn" onclick="useWeb(['${d.id}'])">Use web interface</button>${others > 1 ? `<button class="btn" onclick="useWeb([])">All ${others} Instant On switches</button>` : ""}</div>`);
  }
  if (d.status.authFailed && d.cloud) banners.push(`<div class="banner"><div class="grow"><b>Instant On sign-in failed.</b> ${esc(d.status.collectError)}</div><button class="btn" data-go="#/settings">Open settings</button></div>`);
  else if (d.status.authFailed) banners.push(`<div class="banner"><div class="grow"><b>Login failed.</b> ${esc(d.status.collectError)}. Fix the credentials, then collect again.</div><button class="btn" onclick="openSwitchModal(S.detail)">Edit switch</button></div>`);
  $("#main").innerHTML = `<div class="page">
    <div class="head"><button class="btn small" data-go="#/switches" title="Back">${ICON.back}</button><h1>${esc(name)}</h1><span id="detail-status">${detailStatus(d)}</span><span class="grow"></span>
      ${d.disabled ? "" : `<button class="btn" onclick="collectOne(this)">${ICON.refresh}Collect now</button>`}
      <button class="btn" onclick="setDisabled(['${d.id}'], ${!d.disabled})">${d.disabled ? ICON.play + "Enable" : ICON.pause + "Disable"}</button>
      <button class="btn" onclick="openSwitchModal(S.detail)">Edit</button></div>
    <div class="idline">${d.cloud ? `<span><b>Instant On cloud</b></span>${d.host ? `<span class="mono">${esc(d.host)}</span>` : ""}` : d.web ? `<span class="mono"><b>${esc(d.host)}</b></span><span>Web interface${d.port === 443 ? " (HTTPS)" : d.port === 80 ? " (HTTP)" : ":" + d.port}</span>` : `<span class="mono"><b>${esc(d.host)}</b>${d.port !== 22 ? ":" + d.port : ""}</span>`}
      ${s ? `${d.cloud ? "" : `<span>${esc(s.family)}</span>`}<span><b>${esc(s.model || "Unknown model")}</b></span><span class="mono">${esc(s.software)}</span><span>Up ${esc(s.uptime || "?")}</span>` : ""}
      ${d.site ? `<span>Site <b>${esc(d.site)}</b></span>` : ""}
      ${s ? `<span class="faint">Collected ${ago(s.collectedAt)}${snap?.duration ? ` in ${snap.duration}` : ""}</span>` : ""}</div>
    ${banners.join("")}
    <div class="tabs">${tabs}</div>
    <div id="tabbody">${body}</div></div>`;
  afterTab();
}

function cloudCard(snap) {
  const c = snap.cloud;
  const health = { good: `<span class="ok">Good</span>`, warning: `<span class="warnc">Warning</span>`, critical: `<span class="bad">Critical</span>` }[c.health] || `<span class="faint">${esc(c.health || "unknown")}</span>`;
  const loops = (snap.ports || []).filter((p) => p.loop).map((p) => p.name);
  return `<section class="panel"><h2>Instant On</h2><div class="body"><dl class="kv">
    <dt>Site</dt><dd>${esc(c.site)}</dd><dt>Health</dt><dd>${health}</dd>
    <dt>Firmware</dt><dd>${c.upToDate === false ? `<span class="warnc">Update available</span>` : c.upToDate ? `<span class="ok">Up to date</span>` : `<span class="faint">unknown</span>`}</dd>
    <dt>Wired clients</dt><dd>${c.clients}</dd>
    ${c.poeBudgetW ? `<dt>PoE</dt><dd>${meter(Math.round((c.poeUsedW / c.poeBudgetW) * 100), 90)} <span class="faint">${Math.round(c.poeUsedW)} of ${Math.round(c.poeBudgetW)} W</span></dd>` : ""}
    <dt>Loops</dt><dd>${loops.length ? `<span class="bad">Ports ${esc(loops.join(", "))}</span>` : `<span class="ok">None detected</span>`}</dd>
    <dt>Alerts</dt><dd>${c.alerts?.length ? c.alerts.length : `<span class="ok">None</span>`}</dd>
  </dl><p class="faint" style="font-size:12.5px;margin:10px 0 0">Read from the Instant On portal. Configuration changes are made in the Instant On app or portal.</p></div></section>`;
}

async function useWeb(ids) {
  try {
    const n = await call("UseWebInterface", ids);
    toast(`${plural(n, "switch").replace("switchs", "switches")} now read through the web interface. Reading now.`);
    if (S.detail) loadDetail(S.detail.id);
  } catch (e) { toast(e.message, true); }
}

function noDataYet(d) {
  return `<div class="panel"><div class="empty"><h3>No data collected yet</h3>
    <p>${d.status.collectError ? esc(d.status.collectError) : d.status.online || (!d.status.checked && S.settings?.checkMethod === "off") ? (S.settings?.collectInterval ? "Collection starts automatically. Use Collect now to start it immediately." : "Use Collect now to read it.") : d.cloud ? "Waiting for the Instant On portal." : d.web ? "The switch isn't answering on its web port right now." : "The switch isn't answering on SSH right now."}</p>
    <button class="btn" onclick="collectOne()">${ICON.refresh}Collect now</button></div></div>`;
}

async function collectOne(btn) {
  btn?.classList.add("spin");
  await call("Refresh", S.detail.id);
  toast("Collecting from " + (S.detail.name || S.detail.host));
  setTimeout(() => btn?.classList.remove("spin"), 1500);
}

async function trustKey() {
  await call("TrustHostKey", S.detail.id);
  toast("New host key will be trusted on the next connection");
}

function tabBody(tab, d, snap) {
  switch (tab) {
    case "stp": return tabSTP(snap);
    case "ports": return tabPorts(snap);
    case "vlans": return tabVLANs(d, snap);
    case "access": return tabAccess(d, snap);
    case "neighbors": return tabNeighbors(snap);
    case "config": return tabConfig(d, snap);
    case "logs": return tabLogs(snap);
    case "console": return tabConsole(d);
    case "raw": return tabRaw(snap);
    default: return tabOverview(d, snap);
  }
}

function meter(v, warn) {
  if (v === undefined || v < 0) return `<span class="faint">unknown</span>`;
  const c = v >= warn ? "var(--crit)" : v >= warn * 0.75 ? "var(--warn)" : "var(--up)";
  return `<span class="meter"><span class="track"><i style="width:${Math.min(v, 100)}%;background:${c}"></i></span>${v}%</span>`;
}

function checkRow(good, text, detail) {
  const ico = good === null ? `<span class="ico faint">–</span>` : good ? `<span class="ico ok">✓</span>` : `<span class="ico bad">✕</span>`;
  return `<div class="check-row">${ico}<span>${text}${detail ? ` <span class="faint">${esc(detail)}</span>` : ""}</span></div>`;
}

function tabOverview(d, snap) {
  const f = snap.facts, st = snap.stp, sec = snap.security, set = S.settings || { cpuWarn: 80, memWarn: 90 };
  const isS = snap.family === "aos-s";
  const env = snap.env || [];
  return `${leds(snap.ports, st, true, snap.optics)}<div style="margin:8px 0 16px">${ledKey(snap.ports)}</div>
  <div class="grid2">
    <section class="panel"><h2>Problems on this switch</h2>
      ${d.issues?.length ? d.issues.map(issueRow).join("") : `<div class="empty" style="padding:22px"><h3>Nothing to fix</h3></div>`}</section>
    <section class="panel"><h2>System</h2><div class="body"><dl class="kv">
      <dt>Hostname</dt><dd>${esc(f.hostname)}</dd><dt>Model</dt><dd>${esc(f.model)}</dd><dt>Serial</dt><dd class="mono">${esc(f.serial)}</dd>
      <dt>Base MAC</dt><dd class="mono">${esc(f.baseMac)}</dd><dt>Software</dt><dd class="mono">${esc(f.software)}</dd><dt>Uptime</dt><dd>${esc(f.uptime)}</dd>
      ${f.cpu < 0 && f.memory < 0 ? "" : `<dt>CPU</dt><dd>${meter(f.cpu, set.cpuWarn)}</dd><dt>Memory</dt><dd>${meter(f.memory, set.memWarn)}</dd>`}
      ${f.location ? `<dt>Location</dt><dd>${esc(f.location)}</dd>` : ""}${f.contact ? `<dt>Contact</dt><dd>${esc(f.contact)}</dd>` : ""}
      ${f.configSaved !== undefined && f.configSaved !== null ? `<dt>Startup config</dt><dd>${f.configSaved ? `<span class="ok">Saved</span>` : `<span class="warnc">Unsaved changes</span>`}</dd>` : ""}
    </dl></div></section>
    ${firmwareCard(d)}
    ${snap.cloud ? cloudCard(snap) : `<section class="panel"><h2>Management access</h2><div class="body">
      ${snap.web ? checkRow(d.port === 443 ? true : null, d.port === 443 ? "Read over HTTPS" : "Read over plain HTTP", d.port === 443 ? "" : "set the port to 443 to use HTTPS") : checkRow(sec.sshEnabled, "SSH enabled", sec.sshPort ? "port " + sec.sshPort + (sec.sshVersion ? ", version " + sec.sshVersion : "") : "")}
      ${checkRow(!sec.telnetEnabled, sec.telnetEnabled ? "Telnet is enabled" : "Telnet disabled", portSeen(sec.telnetPort, 23))}
      ${isS ? checkRow(!sec.httpPlain, sec.httpPlain ? "Plain HTTP web management enabled" : "Plain HTTP disabled", portSeen(sec.httpPort, 80)) : ""}
      ${checkRow(sec.httpsEnabled ? true : null, sec.httpsEnabled ? "HTTPS web management" : "HTTPS not configured")}
      ${checkRow(sec.snmpCommunities?.length ? !sec.snmpDefault : null, sec.snmpCommunities?.length ? (sec.snmpDefault ? "Default SNMP community configured" : "SNMP communities set") : "SNMP not configured", sec.snmpCommunities?.length ? plural(sec.snmpCommunities.length, "community").replace("communitys", "communities") : "")}
      ${isS ? checkRow(sec.managerPassword, sec.managerPassword ? "Manager password set" : "No manager password") : ""}
      ${checkRow(sec.ntpServers?.length > 0, sec.ntpServers?.length ? "Time sync" : "No NTP/SNTP server", (sec.ntpServers || []).join(", "))}
      ${checkRow(sec.logServers?.length ? true : null, sec.logServers?.length ? "Remote syslog" : "No syslog server", (sec.logServers || []).join(", "))}
      ${snap.web ? "" : checkRow(sec.aaa?.length ? true : null, sec.aaa?.length ? "RADIUS/TACACS+ configured" : "Local accounts only")}
    </div></section>`}
    <section class="panel"><h2>Spanning tree<span class="grow"></span>${snap.cloud ? "" : `<a data-go="#/switch/${d.id}/stp" style="font-weight:400;font-size:13px">Details</a>`}</h2><div class="body"><dl class="kv">
      <dt>Status</dt><dd>${!st.known ? `<span class="faint">unknown</span>` : st.enabled ? `<span class="ok">Enabled</span>` : `<span class="bad">Disabled</span>`}</dd>
      ${st.enabled ? `<dt>Mode</dt><dd>${esc(st.mode)}</dd><dt>Role</dt><dd>${st.isRoot ? "Root bridge" : snap.cloud ? "Member" : "Member, root port " + esc(st.rootPort || "?")}</dd>
      <dt>Priority</dt><dd>${esc(st.bridgePriority || "?")}${st.rootPriority ? ` <span class="faint">(root ${esc(st.rootPriority)})</span>` : ""}</dd>
      ${snap.cloud ? "" : `<dt>Topology changes</dt><dd>${st.topologyChanges}${st.lastChange ? ` <span class="faint">last ${esc(st.lastChange)}</span>` : ""}</dd>`}` : ""}
    </dl></div></section>
    ${env.length ? `<section class="panel"><h2>Hardware</h2><div class="body">${env.map((e) => checkRow(e.ok, { fan: "Fan", psu: "Power supply", temp: "Temperature" }[e.kind] + " " + esc(e.name), e.status)).join("")}</div></section>` : ""}
    ${snap.errors?.length ? `<section class="panel"><h2>Collection notes</h2><div class="body faint" style="font-size:13px">${snap.errors.map(esc).join("<br>")}</div></section>` : ""}
  </div>`;
}

// what SwitchDesk saw when it tried the port itself
function portSeen(state, port) {
  return { open: `port ${port} answers`, closed: `port ${port} refused`, filtered: `no answer on port ${port}` }[state] || "";
}

function tabSTP(snap) {
  const st = snap.stp;
  if (!st.known) return `<div class="panel"><div class="empty"><h3>No spanning-tree data</h3><p>The switch didn't answer "show spanning-tree". Check Raw output.</p></div></div>`;
  return `<div class="grid2" style="margin-bottom:16px"><section class="panel"><h2>Bridge</h2><div class="body"><dl class="kv">
    <dt>Status</dt><dd>${st.enabled ? `<span class="ok">Enabled</span>` : `<span class="bad">Disabled</span>`}</dd><dt>Mode</dt><dd>${esc(st.mode)}</dd>
    <dt>This bridge</dt><dd class="mono">${esc(st.bridgeMac)} <span class="faint">priority ${esc(st.bridgePriority)}</span></dd>
    <dt>Root bridge</dt><dd class="mono">${esc(st.rootMac)} <span class="faint">priority ${esc(st.rootPriority)}</span></dd>
    <dt>Role</dt><dd>${st.isRoot ? "This switch is root" : "Root reached via port " + esc(st.rootPort)}</dd>
    <dt>Topology changes</dt><dd>${st.topologyChanges}</dd><dt>Last change</dt><dd>${esc(st.lastChange || "-")}</dd></dl></div></section></div>
    <section class="panel"><h2>Ports in spanning tree</h2>${st.ports?.length ? `<table><thead><tr><th>Port</th><th>Role</th><th>State</th><th class="num">Cost</th></tr></thead><tbody>
    ${st.ports.filter((p) => p.role !== "Disabled").map((p) => `<tr><td class="mono">${esc(p.port)}</td><td>${esc(p.role)}</td><td class="${/Blocking|Discarding/.test(p.state) ? "warnc" : p.state === "Forwarding" ? "ok" : ""}">${esc(p.state)}</td><td class="num">${esc(p.cost)}</td></tr>`).join("")}
    </tbody></table><div class="faint" style="padding:8px 12px;font-size:12.5px">${st.ports.filter((p) => p.role === "Disabled").length} disabled ports hidden</div>` : `<div class="empty">No port table in the output.</div>`}</section>`;
}

function tabPorts(snap) {
  const p = snap.ports || [];
  const memb = portVLANs(snap);
  // a disabled port is down too; say which, so a shut port isn't taken for a dead link
  const state = (x) => (!x.enabled ? ["x", "Disabled", ""] : x.up ? ["u", "Up", "ok"] : ["", "Down", "faint"]);
  return `${leds(p, snap.stp, true, snap.optics)}<div style="margin:8px 0 16px">${ledKey(p)}</div>${opticsPanel(snap)}<section class="panel">
    <table><thead><tr><th>Port</th><th>Name</th><th>Status</th><th>Speed</th><th>Type</th><th>VLANs</th></tr></thead><tbody>
    ${p.map((x) => {
      const [c, label, cls] = state(x);
      return `<tr><td class="mono">${esc(x.name)}</td><td>${esc(x.description)}</td><td class="${cls}"><span class="ledkey" style="color:inherit;font-size:inherit"><span><i class="${c}"></i>${label}</span></span></td>
      <td>${esc(x.speed)}</td><td>${esc(x.type)}</td><td class="mono">${esc(memb[x.name] || x.vlan || "")}</td></tr>`;
    }).join("")}
    </tbody></table></section>`;
}

// Transceivers with their optical levels. Thresholds are the optic's own when
// it reports them (AOS-CX); AOS-S shows its recent alarms instead.
const dbm = (v) => (v === undefined || v === null ? "" : v <= -40 ? "no light" : v.toFixed(1));
function opticJudge(o, upPorts) {
  const rx = o.rx?.length ? Math.min(...o.rx) : null;
  const lt = (a, b) => a !== null && b !== undefined && b !== null && a <= b;
  const gt = (a, b) => a !== null && b !== undefined && b !== null && a >= b;
  const flags = (o.alarms || []).map((a) => a.toLowerCase());
  if (!o.dom) return ["faint", "No DOM"];
  if (upPorts && upPorts[o.port] === false && (rx === null || rx <= -40)) return ["faint", "Not connected"];
  if (lt(rx, o.rxLowAlarm) || gt(rx, o.rxHighAlarm) || (!o.recent && flags.some((f) => f.includes("alarm")))) return ["bad", "Alarm"];
  if (lt(rx, o.rxLowWarn) || gt(rx, o.rxHighWarn) || flags.length) return ["warnc", o.recent ? "Recent alarm" : "Warning"];
  if (rx !== null && rx <= -40) return ["warnc", "No light"];
  return rx !== null ? ["ok", "OK"] : ["faint", ""];
}
function opticsPanel(snap) {
  const o = snap.optics || [];
  if (!o.length) return "";
  const up = Object.fromEntries((snap.ports || []).map((p) => [p.name, p.up]));
  const rng = (lo, hi) => (lo !== undefined && lo !== null ? `${dbm(lo)} to ${hi !== undefined && hi !== null ? dbm(hi) : "?"}` : "");
  return `<section class="panel" style="margin-bottom:16px"><h2>Optics<span class="grow"></span><span class="faint" style="font-weight:400;font-size:12.5px">Power in dBm</span></h2>
    <table><thead><tr><th>Port</th><th>Type</th><th class="num">Rx</th><th class="num">Tx</th><th>Rx warning range</th><th class="num">Temp</th><th>Status</th><th>Serial</th></tr></thead><tbody>
    ${o.map((x) => {
      const [cls, label] = opticJudge(x, up);
      const lanes = (v) => (v?.length > 1 ? v.map(dbm).join(" / ") : dbm(v?.[0]));
      return `<tr><td class="mono">${esc(x.port)}</td><td>${esc(x.type)}${x.model ? ` <span class="faint">${esc(x.model)}</span>` : ""}</td>
        <td class="num mono ${cls === "bad" || cls === "warnc" ? cls : ""}">${lanes(x.rx)}</td><td class="num mono">${lanes(x.tx)}</td>
        <td class="mono faint">${rng(x.rxLowWarn, x.rxHighWarn)}</td><td class="num">${x.tempC !== undefined && x.tempC !== null ? x.tempC.toFixed(0) + "°C" : ""}</td>
        <td class="${cls}">${label}${x.alarms?.length ? ` <span class="faint" style="font-size:12px">${esc(x.alarms.join(", "))}</span>` : ""}</td><td class="mono faint">${esc(x.serial || "")}</td></tr>`;
    }).join("")}</tbody></table></section>`;
}

function portVLANs(snap) {
  const m = {};
  for (const v of snap.vlans || []) {
    for (const p of v.untagged || []) m[p] = (m[p] ? m[p] + " " : "") + v.id + "u";
    for (const p of v.tagged || []) m[p] = (m[p] ? m[p] + " " : "") + v.id + "t";
  }
  return m;
}

function compress(ports) {
  if (!ports?.length) return "";
  const groups = new Map();
  const other = [];
  for (const p of ports) {
    const m = /^(.*?)(\d+)$/.exec(p);
    if (!m) { other.push(p); continue; }
    if (!groups.has(m[1])) groups.set(m[1], []);
    groups.get(m[1]).push(+m[2]);
  }
  const parts = [];
  for (const [pre, ns] of groups) {
    ns.sort((a, b) => a - b);
    let s = ns[0], prev = ns[0];
    const emit = () => parts.push(s === prev ? pre + s : `${pre}${s}-${pre}${prev}`);
    for (const n of ns.slice(1)) { if (n === prev + 1) { prev = n; continue; } emit(); s = prev = n; }
    emit();
  }
  return parts.concat(other).join(", ");
}

function tabVLANs(d, snap) {
  const v = snap.vlans || [];
  const canEdit = snap.family !== "instant-on";
  return `<section class="panel"><h2>VLANs<span class="grow"></span>${canEdit ? `<button class="btn small primary" onclick="openVLANEditor()">${ICON.plus}New VLAN</button>` : `<span class="faint" style="font-weight:400;font-size:13px">Instant On VLANs are changed ${d.web ? "on the switch's web page" : "in the Instant On app or portal"}</span>`}</h2>
    ${v.length ? `<table><thead><tr><th class="num">ID</th><th>Name</th><th>IP address</th><th>Untagged</th><th>Tagged</th><th></th></tr></thead><tbody>
    ${v.map((x) => `<tr><td class="num mono">${x.id}</td><td>${esc(x.name)}</td><td class="mono">${esc(x.ip)}</td><td class="mono">${esc(compress(x.untagged))}</td><td class="mono">${esc(compress(x.tagged))}</td>
      <td style="white-space:nowrap;text-align:right">${canEdit ? `<button class="btn small" onclick="openVLANEditor(${x.id})">Edit</button> ${x.id !== 1 ? `<button class="btn small danger" onclick="deleteVLAN(${x.id})">Delete</button>` : ""}` : ""}</td></tr>`).join("")}
    </tbody></table>` : `<div class="empty">No VLANs found in the configuration.</div>`}</section>`;
}

function tabNeighbors(snap) {
  const n = snap.neighbors || [];
  const byIP = new Map((S.dash?.switches || []).map((s) => [s.host, s]));
  const byName = new Map((S.dash?.switches || []).map((s) => [(s.summary?.hostname || s.name || "").toLowerCase(), s]));
  return `<section class="panel">${n.length ? `<table><thead><tr><th>Local port</th><th>Neighbor</th><th>Remote port</th><th>Address</th><th>Protocol</th><th>Capabilities</th><th>Description</th></tr></thead><tbody>
    ${n.map((x) => {
      const known = byIP.get(x.mgmtIp) || byName.get((x.sysName || "").toLowerCase());
      return `<tr><td class="mono">${esc(x.localPort)}</td><td>${known ? `<a data-go="#/switch/${known.id}">${esc(x.sysName)}</a>` : esc(x.sysName || x.chassisId)}</td><td class="mono">${esc(x.portDescr && /^[0-9a-f]{2}([ :-]?[0-9a-f]{2}){5}$/i.test(x.portId) ? x.portDescr : x.portId)}</td>
      <td class="mono">${esc(x.mgmtIp)}</td><td>${x.protocol === "cloud" ? "Instant On" : x.protocol.toUpperCase()}</td><td>${esc(x.capabilities)}</td><td class="faint" style="max-width:340px">${esc(x.description)}</td></tr>`;
    }).join("")}</tbody></table>` : `<div class="empty"><h3>No LLDP or CDP neighbors</h3><p>Make sure LLDP is enabled on this switch and its peers.</p></div>`}</section>`;
}

function tabLogs(snap) {
  const l = snap.logs || [];
  const sev = { E: ["Error", "critical"], M: ["Major", "critical"], W: ["Warning", "warning"], I: ["Info", ""], D: ["Debug", ""] };
  return `<section class="panel">${l.length ? `<table><tbody>${l.map((x) => `<tr><td style="width:84px">${sev[x.severity]?.[1] ? `<span class="sev ${sev[x.severity][1]}">${sev[x.severity][0]}</span>` : `<span class="faint">${sev[x.severity]?.[0] || x.severity}</span>`}</td><td class="mono" style="user-select:text">${esc(x.text)}</td></tr>`).join("")}</tbody></table>` : `<div class="empty">No log lines collected.</div>`}</section>`;
}

let cfgView = { name: "", text: "", compare: "", search: "" };

function tabConfig(d, snap) {
  if (!cfgView.text || cfgView.forId !== d.id) cfgView = { forId: d.id, name: "", text: snap.runningConfig || "", compare: "", search: "" };
  const list = [`<div class="${!cfgView.name ? "on" : ""}" onclick="showBackup('')">Running config<div class="faint" style="font-size:12px">${ago(snap.collectedAt)}</div></div>`]
    .concat((d.backups || []).map((b) => `<div class="${cfgView.name === b.name ? "on" : ""}" onclick="showBackup('${b.name}')">${new Date(b.at).toLocaleString()}<div class="faint" style="font-size:12px">${(b.size / 1024).toFixed(1)} KB</div></div>`));
  return `<div class="split"><section class="panel"><h2>Saved copies</h2><div class="list-pick">${list.join("")}</div>
    <div class="faint" style="padding:10px 12px;font-size:12.5px">A copy is kept each time the config changes.</div></section>
    <section><div class="row" style="margin-bottom:10px;align-items:center">
      <input type="text" id="cfgsearch" placeholder="Find in config" value="${esc(cfgView.search)}" oninput="cfgView.search=this.value;drawConfig()">
      <select id="cfgcmp" onchange="compareBackup(this.value)" style="max-width:260px"><option value="">Compare with…</option>${(d.backups || []).filter((b) => b.name !== cfgView.name).map((b) => `<option value="${b.name}" ${cfgView.compare === b.name ? "selected" : ""}>${new Date(b.at).toLocaleString()}</option>`).join("")}</select>
      <span style="flex:none"><button class="btn" onclick="exportConfig()">Save as file</button></span></div>
      <pre class="code mono" id="cfgpre"></pre></section></div>`;
}

async function showBackup(name) {
  cfgView.name = name;
  cfgView.compare = "";
  cfgView.text = name ? await call("GetBackup", S.detail.id, name) : S.detail.snapshot.runningConfig;
  $("#tabbody").innerHTML = tabConfig(S.detail, S.detail.snapshot);
  afterTab();
}

async function compareBackup(name) {
  cfgView.compare = name;
  cfgView.cmpText = name ? await call("GetBackup", S.detail.id, name) : "";
  drawConfig();
}

function drawConfig() {
  const pre = $("#cfgpre");
  if (!pre) return;
  if (cfgView.compare) {
    const ops = diffLines(cfgView.cmpText.split("\n"), cfgView.text.split("\n"));
    const changed = ops.filter((o) => o[0] !== " ").length;
    pre.innerHTML = changed ? ops.map(([t, l]) => t === "+" ? `<span class="add">+ ${esc(l)}</span>` : t === "-" ? `<span class="del">- ${esc(l)}</span>` : `  ${esc(l)}\n`).join("") : "No differences.";
    return;
  }
  const q = cfgView.search.trim();
  let html = esc(cfgView.text || "No configuration collected.");
  if (q) html = html.replace(new RegExp(esc(q).replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi"), (m) => `<mark>${m}</mark>`);
  pre.innerHTML = html;
  pre.querySelector("mark")?.scrollIntoView({ block: "center" });
}

// small LCS diff; configs are a few thousand lines at most
function diffLines(a, b) {
  const n = a.length, m = b.length;
  if (n * m > 9e6) return [["-", `(${n} lines)`], ["+", `(${m} lines) too large to compare here`]];
  const dp = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const out = [];
  let i = 0, j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) { out.push([" ", a[i]]); i++; j++; }
    else if (dp[i + 1][j] >= dp[i][j + 1]) out.push(["-", a[i++]]);
    else out.push(["+", b[j++]]);
  }
  while (i < n) out.push(["-", a[i++]]);
  while (j < m) out.push(["+", b[j++]]);
  return out;
}

async function exportConfig() {
  const d = S.detail;
  const name = `${(d.summary?.hostname || d.name || d.host).replace(/[^\w.-]/g, "_")}-${cfgView.name ? cfgView.name.replace(".cfg", "") : "running"}.cfg`;
  try {
    const p = await call("ExportText", name, cfgView.text);
    toast("Saved to " + p);
  } catch (e) { if (e.message !== "cancelled") toast(e.message, true); }
}

function tabConsole(d) {
  return `<section class="panel"><h2>Run a command<span class="grow"></span><span class="faint" style="font-weight:400;font-size:13px">Read-only: show, ping and traceroute</span></h2><div class="body">
    <form class="row" onsubmit="runCmd(event)" style="align-items:center"><input type="text" id="cmd" class="mono" placeholder="show interfaces brief" autocomplete="off" spellcheck="false" onkeydown="cmdHistory(event)"><span style="flex:none"><button class="btn primary" id="runbtn">Run</button></span></form>
    <div style="height:12px"></div><pre class="code mono console-out" id="cmdout">${esc(S.consoleOut || "Output appears here.")}</pre></div></section>`;
}

let histPos = -1;
function cmdHistory(e) {
  if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
  e.preventDefault();
  const h = S.consoleHist;
  if (!h.length) return;
  histPos = e.key === "ArrowUp" ? Math.min(histPos + 1, h.length - 1) : Math.max(histPos - 1, -1);
  e.target.value = histPos < 0 ? "" : h[histPos];
}

async function runCmd(e) {
  e.preventDefault();
  const cmd = $("#cmd").value.trim();
  if (!cmd) return;
  S.consoleHist = [cmd, ...S.consoleHist.filter((c) => c !== cmd)].slice(0, 50);
  histPos = -1;
  const btn = $("#runbtn");
  btn.disabled = true;
  $("#cmdout").textContent = "Running " + cmd + " …";
  try {
    S.consoleOut = `# ${cmd}\n` + (await call("RunCommand", S.detail.id, cmd));
  } catch (err) {
    S.consoleOut = `# ${cmd}\n${err.message}`;
  }
  btn.disabled = false;
  if ($("#cmdout")) $("#cmdout").textContent = S.consoleOut;
}

function tabRaw(snap) {
  const keys = Object.keys(snap.raw || {});
  const cur = S.rawKey && keys.includes(S.rawKey) ? S.rawKey : keys[0];
  return `<div class="split"><section class="panel"><div class="list-pick">${keys.map((k) => `<div class="${k === cur ? "on" : ""}" onclick="S.rawKey='${k}';$('#tabbody').innerHTML=tabRaw(S.detail.snapshot)">${esc(k)}</div>`).join("")}</div></section>
    <pre class="code mono">${esc(snap.raw?.[cur] || "(empty)")}</pre></div>`;
}

function afterTab() {
  if (S.tab === "config") drawConfig();
  if (S.tab === "console") $("#cmd")?.focus();
}

// ---------- VLAN editor ----------
function openVLANEditor(id) {
  const snap = S.detail.snapshot;
  const existing = id ? snap.vlans.find((v) => v.id === id) : null;
  const ports = new Set((snap.ports || []).map((p) => p.name));
  for (const v of snap.vlans || []) for (const p of [...(v.tagged || []), ...(v.untagged || [])]) ports.add(p);
  const order = [...ports].sort((a, b) => a.localeCompare(b, undefined, { numeric: true }));
  const state = {};
  for (const p of order) state[p] = "";
  if (existing) {
    for (const p of existing.untagged || []) state[p] = "U";
    for (const p of existing.tagged || []) state[p] = "T";
  }
  const orig = { ...state };
  const m = modal(`<h2>${existing ? `Edit VLAN ${id}` : "New VLAN"}</h2><div class="body">
    <div class="row"><label class="f" style="flex:0 0 120px"><span>VLAN ID</span><input type="number" id="vid" min="1" max="4094" value="${id || ""}" ${existing ? "disabled" : ""}></label>
      <label class="f"><span>Name</span><input type="text" id="vname" maxlength="32" value="${esc(existing?.name || "")}"></label></div>
    <div class="muted" style="font-size:13px">Click a port to cycle: untagged, tagged, not a member.</div>
    <div class="legend"><span><i style="background:var(--up)"></i>Untagged (access)</span><span><i style="background:var(--info)"></i>Tagged (trunk)</span></div>
    <div class="portpick" id="pp">${order.map((p) => `<button type="button" class="pp ${state[p]}" data-p="${esc(p)}">${esc(p)}${state[p] ? `<sup>${state[p]}</sup>` : ""}</button>`).join("")}</div>
    <div style="height:14px"></div><label class="check"><input type="checkbox" id="vsave" checked> Save to startup config (write memory)</label>
    <div id="verr"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="vprev">Review changes</button></div>`, true);
  $("#pp", m).addEventListener("click", (e) => {
    const b = e.target.closest(".pp");
    if (!b) return;
    const p = b.dataset.p;
    state[p] = { "": "U", U: "T", T: "" }[state[p]];
    b.className = "pp " + state[p];
    b.innerHTML = esc(p) + (state[p] ? `<sup>${state[p]}</sup>` : "");
  });
  $("#vprev", m).onclick = async () => {
    const ch = {
      action: existing ? "update" : "create",
      id: +$("#vid", m).value,
      name: $("#vname", m).value.trim() !== (existing?.name || "") ? $("#vname", m).value.trim() : "",
      addUntagged: order.filter((p) => state[p] === "U" && orig[p] !== "U"),
      addTagged: order.filter((p) => state[p] === "T" && orig[p] !== "T"),
      removePorts: order.filter((p) => state[p] === "" && orig[p] !== ""),
      save: $("#vsave", m).checked,
    };
    if (existing && !ch.name && !ch.addUntagged.length && !ch.addTagged.length && !ch.removePorts.length) {
      $("#verr", m).innerHTML = `<div class="msg err">Nothing changed.</div>`;
      return;
    }
    try {
      const cmds = await call("PreviewVLAN", S.detail.id, ch);
      closeModal();
      confirmCommands(ch, cmds);
    } catch (e) {
      $("#verr", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
    }
  };
}

function deleteVLAN(id) {
  const ch = { action: "delete", id, save: true };
  call("PreviewVLAN", S.detail.id, ch).then((cmds) => confirmCommands(ch, cmds)).catch((e) => toast(e.message, true));
}

// note is optional HTML shown as a warning above the commands.
function confirmCommands(ch, cmds, method = "ApplyVLAN", note = "") {
  const name = S.detail.name || S.detail.summary?.hostname || S.detail.host;
  const glob = ch.globalDot1x ?? ch.globalMacAuth;
  const apply = ch.action === "delete" ? "Delete VLAN" : ch.removeServer ? "Remove server" : ch.deleteRole ? "Delete role" : glob !== undefined ? (glob ? "Turn on" : "Turn off") : "Apply to switch";
  const title = ch.action === "delete" ? `Delete VLAN ${ch.id}` : ch.removeServer ? `Remove RADIUS server ${esc(ch.removeServer)}` : ch.deleteRole ? `Delete role ${esc(ch.deleteRole)}`
    : glob !== undefined ? `Turn ${ch.globalDot1x !== undefined ? "802.1X" : "MAC authentication"} ${glob ? "on" : "off"} for the switch` : "Review changes";
  const m = modal(`<h2>${title}</h2><div class="body">
    <p class="muted" style="margin-top:0">These lines will be sent to <b style="color:var(--text)">${esc(name)}</b>. Sending stops at the first line the switch rejects${ch.save ? ", and nothing is saved to startup config in that case" : ""}.</p>
    ${note ? `<div class="msg warnm" style="margin-bottom:10px">${note}</div>` : ""}
    <pre class="code mono" id="vtx">${esc(cmds.join("\n"))}</pre><div id="vres"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="vapply">${apply}</button></div>`, true);
  $("#vapply", m).onclick = async () => {
    const b = $("#vapply", m);
    b.disabled = true;
    b.textContent = "Sending…";
    try {
      const r = await call(method, S.detail.id, ch);
      $("#vtx", m).textContent = r.transcript.join("\n");
      $("#vres", m).innerHTML = r.ok ? `<div class="msg okm">Done. The switch is being re-read.</div>` : `<div class="msg err">${esc(r.error)}</div>`;
      // show the accepted change behind the dialog now; the re-read confirms it later
      if (r.ok) loadDetail(S.detail.id, true);
      b.remove();
      $("[data-close]", m).textContent = "Close";
    } catch (e) {
      $("#vres", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
      b.disabled = false;
      b.textContent = "Try again";
    }
  };
}

// ---------- port access (802.1X / MAC authentication) ----------
function paPorts(snap) {
  const names = (snap.ports || []).map((p) => p.name);
  for (const a of snap.portAccess?.ports || []) if (!names.includes(a.port)) names.push(a.port);
  return names;
}
// ports that lead to other network gear: a login there can cut the link
function paInfra(snap) {
  const m = {};
  for (const n of snap.neighbors || []) if (/bridge|router|wlan|access.?point/i.test(n.capabilities || "")) m[n.localPort] = n.sysName || n.chassisId || "a neighbor";
  return m;
}
const reauthText = (s) => (!s ? "Off" : s % 3600 === 0 ? `${s / 3600} h` : s % 60 === 0 ? `${s / 60} min` : `${s} s`);
// AOS-CX role slots on a port: [config name, label, short tag]
const ROLE_SLOTS = [["auth-role", "After login", "Auth"], ["reject-role", "Login rejected", "Reject"], ["critical-role", "RADIUS unreachable", "Critical"], ["preauth-role", "Before login", "Pre-auth"]];
const roleTags = (a) => ROLE_SLOTS.filter(([k]) => a.roles?.[k]).map(([k, l, s]) => `<span class="rtag" title="${l}"><i>${s}</i>${esc(a.roles[k])}</span>`).join("");
const paClass = (a) => (a?.dot1x && a?.macAuth ? "ab" : a?.dot1x ? "a1" : a?.macAuth ? "am" : "");
const paBadge = (a) => (a?.dot1x && a?.macAuth ? "1X+M" : a?.dot1x ? "1X" : a?.macAuth ? "MAC" : "");

function tabAccess(d, snap) {
  const pa = snap.portAccess;
  if (!pa) return `<section class="panel"><div class="empty"><h3>Port access isn't available here</h3><p>802.1X and MAC authentication are managed on AOS-S and AOS-CX switches over SSH.${snap.family === "aos-s" || snap.family === "aos-cx" ? " Collect this switch again to read its setup." : ""}</p></div></section>`;
  const cx = snap.family === "aos-cx";
  const all = paPorts(snap);
  if (S.paFor !== d.id) { S.paFor = d.id; S.paSel = new Set(); S.paq = ""; }
  for (const p of S.paSel) if (!all.includes(p)) S.paSel.delete(p);
  const by = Object.fromEntries((pa.ports || []).map((a) => [a.port, a]));
  const info = Object.fromEntries((snap.ports || []).map((p) => [p.name, p]));
  const infra = paInfra(snap);
  const servers = pa.servers || [];
  const n1x = pa.ports.filter((a) => a.dot1x).length, nmac = pa.ports.filter((a) => a.macAuth).length;
  const open = all.length - pa.ports.length;
  const glob = (on, kind) => `${on ? `<span class="sev okchip">On for the switch</span>` : `<span class="sev warning">Off for the switch</span>`}
    <button class="btn small" onclick="paGlobal('${kind}', ${!on})">${on ? "Turn off" : "Turn on"}</button>`;
  const stat = (v, l, sub, cls = "") => `<div class="pastat"><div class="v ${cls}">${v}</div><div class="l">${l}</div><div class="s">${sub}</div></div>`;
  const onPort = {};
  for (const c of pa.clients || []) onPort[c.port] = (onPort[c.port] || 0) + 1;
  const tile = (p) => {
    const a = by[p], up = info[p]?.up;
    const tip = `Port ${p}${a ? `: ${[a.dot1x && "802.1X", a.macAuth && "MAC authentication"].filter(Boolean).join(" + ")}` : ": no login asked"}${onPort[p] ? `, ${plural(onPort[p], "client")}` : ""}${infra[p] ? `, connects to ${infra[p]}` : ""}${info[p] && !up ? ", link down" : ""}${info[p]?.description ? " (" + info[p].description + ")" : ""}`;
    return `<button type="button" class="pp pa ${paClass(a)}${S.paSel.has(p) ? " sel" : ""}${infra[p] ? " infra" : ""}${info[p] && !up ? " down" : ""}" data-p="${esc(p)}" title="${esc(tip)}">${esc(p)}<sup>${paBadge(a) || "&nbsp;"}</sup></button>`;
  };
  const onoff = (v) => (v ? `<span class="sev okchip">On</span>` : `<span class="faint">–</span>`);
  return `<div class="pastats">
      ${stat(servers.length, plural(servers.length, "RADIUS server").replace(/^\d+ /, ""), servers.length ? `<span class="faint">${esc(servers.map((s) => s.host).join(", "))}</span>` : `<span class="warnc">Nobody can log in without one</span>`, servers.length ? "" : "warnc")}
      ${stat(n1x, "Ports with 802.1X", glob(pa.dot1xOn, "dot1x"))}
      ${stat(nmac, "Ports with MAC authentication", cx ? glob(pa.macAuthOn, "mac") : `<span class="faint">Turned on per port</span>`)}
      ${stat(open, "Open ports", `<span class="faint">No login asked</span>`)}
    </div>
    <section class="panel" style="margin-bottom:16px"><h2>RADIUS servers<span class="grow"></span><button class="btn small primary" onclick="openRadiusEditor()">${ICON.plus}Add server</button></h2>
    ${servers.length ? `<table><thead><tr><th>Server</th>${cx ? "<th>VRF</th>" : ""}<th>Shared secret</th><th></th></tr></thead><tbody>
      ${servers.map((s) => `<tr><td class="mono">${esc(s.host)}</td>${cx ? `<td class="mono">${esc(s.vrf || "default")}</td>` : ""}<td>${s.hasKey ? `<span class="ok">Set</span>` : `<span class="warnc">Not set</span>`}</td>
        <td style="text-align:right"><button class="btn small danger" data-pa="removeRadius" data-arg="${esc(s.host)}">Remove</button></td></tr>`).join("")}</tbody></table>`
    : `<div class="empty" style="padding:22px"><h3>No RADIUS server</h3><p>802.1X and MAC authentication check every login with a RADIUS server, such as ClearPass or Microsoft NPS. Add one before turning them on.</p></div>`}</section>
    ${cx ? rolesPanel(snap, pa) : ""}
    <section class="panel"><h2>Ports<span class="grow"></span>
      <span class="chips" style="font-weight:400"><button class="chip" onclick="paSelect('all')">All</button><button class="chip" onclick="paSelect('open')">Open edge ports</button><button class="chip" onclick="paSelect('auth')">With login</button><button class="chip" onclick="paSelect('none')">None</button></span>
      <button class="btn small primary" id="paedit" onclick="openPortAccessEditor()" ${S.paSel.size ? "" : "disabled"}>${S.paSel.size ? `Edit ${plural(S.paSel.size, "port")}` : "Select ports to edit"}</button></h2>
      <div class="body">
        <div class="legend" style="margin-top:0"><span><i style="background:rgba(76,195,138,.55)"></i>802.1X</span><span><i style="background:rgba(110,168,254,.55)"></i>MAC authentication</span><span><i style="background:linear-gradient(135deg,rgba(76,195,138,.7) 50%,rgba(110,168,254,.7) 50%)"></i>Both</span><span><i style="background:var(--warn);border-radius:50%;width:7px;height:7px"></i>Links to a switch or AP</span><span class="faint">Click ports to select, Shift-click for a range</span></div>
        <div class="portpick" id="pagrid" onclick="paClick(event)">${all.map(tile).join("")}</div>
      </div>
      ${pa.ports.length ? `<table><thead><tr><th>Port</th><th>802.1X</th><th>MAC auth</th><th class="num">Client limit</th><th>Re-authentication</th>${cx ? "<th>Roles</th>" : "<th>Unauth VLAN</th>"}<th>Link</th><th>Description</th></tr></thead><tbody>
        ${pa.ports.map((a) => `<tr class="click" data-pa="paEditOne" data-arg="${esc(a.port)}"><td class="mono">${esc(a.port)}</td><td>${onoff(a.dot1x)}</td><td>${onoff(a.macAuth)}</td>
          <td class="num">${a.clientLimit || `<span class="faint">default</span>`}</td><td>${a.reauthPeriod ? "Every " + reauthText(a.reauthPeriod) : `<span class="faint">Off</span>`}</td>
          ${cx ? `<td>${roleTags(a) || `<span class="faint">–</span>`}</td>` : `<td class="mono">${a.unauthVlan || `<span class="faint">–</span>`}</td>`}
          <td class="${info[a.port]?.up ? "ok" : "faint"}">${info[a.port] ? (info[a.port].up ? "Up" : "Down") : ""}</td><td>${esc(info[a.port]?.description || "")}</td></tr>`).join("")}</tbody></table>`
      : `<div class="empty" style="padding:18px;border-top:1px solid var(--line-soft)">No port asks for a login yet. Select ports above and turn on 802.1X or MAC authentication.</div>`}
    </section>
    ${cx ? clientsPanel(snap, pa) : ""}`;
}

// ---------- connected clients (AOS-CX show port-access clients) ----------
const METHOD = { dot1x: "802.1X", "mac-auth": "MAC auth", "device-profile": "Device profile", "port-security": "Port security" };
function clientStatus(s) {
  const t = (s || "").toLowerCase();
  const cls = /success|authenticated|authorized|applied/.test(t) && !/unauth|not /.test(t) ? "okchip" : /progress|pending|held|connecting/.test(t) ? "info" : t ? "critical" : "";
  return t ? `<span class="sev ${cls}">${esc(s)}</span>` : `<span class="faint">–</span>`;
}
function clientVLAN(c, snap) {
  if (!c.vlan && !c.tagged) return `<span class="faint">–</span>`;
  const name = (snap.vlans || []).find((v) => String(v.id) === c.vlan)?.name || "";
  return `${c.vlan ? `<span class="mono">${esc(c.vlan)}</span>${name ? ` <span class="faint">${esc(name)}</span>` : ""}` : ""}${c.tagged ? `<span class="vlk">Tagged</span><span class="mono">${esc(c.tagged)}</span>` : ""}`;
}
function clientsPanel(snap, pa) {
  const cs = pa.clients || [];
  const t = cs.length > 5 ? (S.paq || "").trim().toLowerCase() : ""; // the filter box only shows above 5
  const head = `<h2>Connected clients&nbsp;<span class="faint" style="font-weight:400">${cs.length}</span><span class="grow"></span>
    <span class="faint" style="font-size:12.5px;font-weight:400">Read ${ago(pa.clientsAt)}</span>
    ${cs.length > 5 ? `<span class="searchbox" style="font-weight:400">${ICON.search}<input type="text" id="paq" placeholder="Filter clients" value="${esc(S.paq || "")}" oninput="paFilter(this.value)" style="width:200px"></span>` : ""}
    <button class="btn small" id="pacref" onclick="refreshPortClients()">${ICON.refresh}Refresh</button></h2>`;
  if (!pa.clientsRead) return `<section class="panel" style="margin-top:16px">${head}<div class="empty" style="padding:22px"><h3>Clients weren't read</h3><p>The switch didn't answer <span class="mono">show port-access clients</span> at the last collection. Click Refresh to try again.</p></div></section>`;
  if (!cs.length) return `<section class="panel" style="margin-top:16px">${head}<div class="empty" style="padding:22px"><h3>No clients</h3><p>Nobody is logged in on a port with 802.1X or MAC authentication right now.</p></div></section>`;
  const rows = [...cs].sort((a, b) => a.port.localeCompare(b.port, undefined, { numeric: true }) || a.mac.localeCompare(b.mac))
    .map((c) => [c, [c.port, c.mac, c.user, c.ip, METHOD[c.method] || c.method, c.status, c.role, c.vlan].join(" ").toLowerCase()]);
  const hit = (q) => !t || q.includes(t);
  return `<section class="panel" style="margin-top:16px">${head}
    <table id="pactab"><thead><tr><th>Port</th><th>Device</th><th>IP address</th><th>Login</th><th>Status</th><th>Role</th><th>VLAN</th><th>Connected</th></tr></thead><tbody>
    ${rows.map(([c, q]) => `<tr data-q="${esc(q)}"${hit(q) ? "" : ` style="display:none"`}>
      <td class="mono">${esc(c.port)}</td>
      <td><span class="mono">${copyable(c.mac)}</span>${c.user && c.user !== c.mac ? `<div class="faint" style="font-size:12.5px">${esc(c.user)}</div>` : ""}</td>
      <td class="mono">${esc(c.ip || "")}</td>
      <td>${esc(METHOD[c.method] || c.method || "")}</td>
      <td>${clientStatus(c.status)}</td>
      <td>${c.role ? `<span class="mono">${esc(c.role)}</span>${c.roleStatus && !/applied/i.test(c.roleStatus) ? ` <span class="sev warning" title="Role status">${esc(c.roleStatus)}</span>` : ""}` : `<span class="faint">–</span>`}</td>
      <td>${clientVLAN(c, snap)}</td>
      <td class="faint">${esc(c.session || "")}</td></tr>`).join("")}</tbody></table>
    <div class="empty" id="pacnone" style="padding:14px;${rows.some(([, q]) => hit(q)) ? "display:none" : ""}">No client matches the filter.</div></section>`;
}
// filters rows in place, so typing doesn't re-render the page and lose focus
function paFilter(q) {
  S.paq = q;
  const t = q.trim().toLowerCase();
  let shown = 0;
  for (const r of $$("#pactab tbody tr")) {
    const on = !t || r.dataset.q.includes(t);
    r.style.display = on ? "" : "none";
    shown += on;
  }
  const none = $("#pacnone");
  if (none) none.style.display = shown ? "none" : "";
}
async function refreshPortClients() {
  const b = $("#pacref");
  if (b) { b.disabled = true; b.lastChild.textContent = "Reading…"; }
  try {
    await call("RefreshPortClients", S.detail.id);
    await loadDetail(S.detail.id, true);
  } catch (e) {
    toast(e.message, true);
    if (b) { b.disabled = false; b.lastChild.textContent = "Refresh"; }
  }
}

// A role's VLANs as the switch knows them. The access or native VLAN may be
// given by number, by name, or both; a trunk role adds tagged VLANs.
function roleVLAN(r, snap) {
  const id = r.vlan || (snap.vlans || []).find((v) => v.name === r.vlanName)?.id || 0;
  const name = r.vlanName || (snap.vlans || []).find((v) => v.id === id)?.name || "";
  const mode = r.vlanMode === "trunk" || r.vlanMode === "native" ? "trunk" : "access";
  // the untagged VLAN is in the allowed list too; it's shown once, as untagged
  const tagged = (r.tagged || []).filter((t) => t !== id);
  return { id, name, mode, tagged, set: !!(r.vlan || r.vlanName || tagged.length) };
}
const roleVLANText = (r, snap) => {
  const v = roleVLAN(r, snap), u = [v.id, v.name].filter(Boolean).join(" ");
  if (!v.set) return "";
  if (v.mode === "access") return "VLAN " + u;
  return [u && "untagged " + u, v.tagged.length && "tagged " + compress(v.tagged.map(String))].filter(Boolean).join(", ");
};
function roleVLANCell(r, snap) {
  const v = roleVLAN(r, snap);
  if (!v.set) return `<span class="faint">–</span>`;
  const u = v.id || v.name ? `<span class="mono">${v.id || ""}</span> <span class="faint">${esc(v.name)}</span>` : "";
  if (v.mode === "access") return `<span class="tag" title="vlan access">Single VLAN</span> ${u}`;
  return `<span class="tag" title="vlan trunk native / vlan trunk allowed">Trunk</span>
    ${u ? `<span class="vlk">Untagged</span>${u}` : ""}${v.tagged.length ? `<span class="vlk">Tagged</span><span class="mono">${esc(compress(v.tagged.map(String)))}</span>` : ""}`;
}

function rolesPanel(snap, pa) {
  const roles = pa.roles || [];
  const used = (name) => (pa.ports || []).filter((a) => Object.values(a.roles || {}).includes(name)).map((a) => a.port);
  return `<section class="panel" style="margin-bottom:16px"><h2>Roles<span class="grow"></span><button class="btn small primary" onclick="openRoleEditor()">${ICON.plus}New role</button></h2>
    ${roles.length ? `<table><thead><tr><th>Role</th><th>VLAN</th><th>Description</th><th>Used on</th><th></th></tr></thead><tbody>
      ${roles.map((r) => {
        const u = used(r.name);
        return `<tr><td class="mono">${esc(r.name)}</td><td>${roleVLANCell(r, snap)}</td>
          <td>${esc(r.description || "")}</td><td class="mono">${u.length ? esc(compress(u)) : `<span class="faint" style="font-family:var(--sans)">Not used</span>`}</td>
          <td style="text-align:right;white-space:nowrap"><button class="btn small" data-pa="openRoleEditor" data-arg="${esc(r.name)}">Edit</button>
          <button class="btn small danger" data-pa="deleteRole" data-arg="${esc(r.name)}" ${u.length ? `disabled title="Used on ports ${esc(compress(u))}"` : ""}>Delete</button></td></tr>`;
      }).join("")}</tbody></table>`
    : `<div class="empty" style="padding:22px"><h3>No roles yet</h3><p>A role decides which VLAN a client lands in: after it logs in, when its login is rejected, while the RADIUS server can't be reached, or before it logs in. AOS-CX has none by default. Create one, then assign it to ports.</p></div>`}</section>`;
}

function openRoleEditor(name) {
  const snap = S.detail.snapshot, existing = name ? (snap.portAccess.roles || []).find((r) => r.name === name) : null;
  const was = existing ? roleVLAN(existing, snap) : { id: 0, mode: "access", tagged: [] };
  const vlans = snap.vlans || [];
  const opts = (sel) => `<option value="0">None</option>${vlans.map((v) => `<option value="${v.id}" ${sel === v.id ? "selected" : ""}>${v.id}${v.name ? " · " + esc(v.name) : ""}</option>`).join("")}`;
  const tagged = new Set(was.tagged);
  const m = modal(`<h2>${existing ? `Edit role ${esc(name)}` : "New role"}</h2><div class="body">
    <div class="row"><label class="f"><span>Name</span><input type="text" id="rlname" maxlength="64" value="${esc(name || "")}" ${existing ? "disabled" : ""} placeholder="RoleLan" spellcheck="false"></label>
      <label class="f"><span>Description</span><input type="text" id="rldesc" maxlength="64" value="${esc(existing?.description || "")}" placeholder="Optional"></label></div>
    <div class="pafield" style="grid-template-columns:150px auto 1fr;border-top:1px solid var(--line-soft)"><b>VLANs</b>
      <div class="seg" id="rlmode"><button type="button" data-v="access" class="${was.mode === "access" ? "on" : ""}">Single VLAN</button><button type="button" data-v="trunk" class="${was.mode === "trunk" ? "on" : ""}">Trunk</button></div>
      <span class="now" id="rlhint"></span></div>
    <div id="rlacc" style="margin-top:14px"><label class="f" style="max-width:320px"><span>VLAN (untagged)</span><select id="rlvlan">${opts(was.mode === "access" ? was.id : 0)}</select></label></div>
    <div id="rltrk" style="margin-top:14px">
      <label class="f" style="max-width:320px"><span>Untagged VLAN (native)</span><select id="rlnative">${opts(was.mode === "trunk" ? was.id : 0)}</select></label>
      <div class="muted" style="font-size:13px;margin-bottom:6px">Tagged VLANs <span class="faint">· click to add or remove. The untagged VLAN (green, marked U) is always allowed on the trunk too.</span></div>
      <div class="portpick" id="rltag">${vlans.map((v) => `<button type="button" class="pp vl${tagged.has(v.id) ? " T" : ""}" data-v="${v.id}" title="${esc(v.name || "")}">${v.id}${v.name ? `<small>${esc(v.name)}</small>` : ""}</button>`).join("")}</div>
      ${was.tagged.filter((id) => !vlans.some((v) => v.id === id)).length ? `<div class="faint" style="font-size:12.5px;margin-top:6px">Also tagged, but not on this switch: ${esc(compress(was.tagged.filter((id) => !vlans.some((v) => v.id === id)).map(String)))}</div>` : ""}
    </div>
    <div style="height:12px"></div><label class="check"><input type="checkbox" id="rlsave" checked> Save to startup config (write memory)</label>
    <div id="rlerr"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="rlprev">Review changes</button></div>`, true);
  const mode = () => $("#rlmode .on", m).dataset.v;
  const show = () => {
    $("#rlacc", m).style.display = mode() === "access" ? "" : "none";
    $("#rltrk", m).style.display = mode() === "trunk" ? "" : "none";
    $("#rlhint", m).textContent = mode() === "access"
      ? "The port gets one VLAN, untagged (vlan access). For a PC or printer."
      : "The port gets an untagged VLAN plus tagged VLANs (vlan trunk native / allowed). For a phone with a PC behind it, or an AP.";
  };
  // the untagged VLAN is always allowed on the trunk, so its tile shows as tagged and stays on
  const paintNative = () => {
    const native = +$("#rlnative", m).value;
    $$("#rltag .pp", m).forEach((b) => {
      const id = +b.dataset.v;
      b.classList.toggle("N", id === native);
      b.classList.toggle("T", id === native || tagged.has(id));
      b.title = id === native ? "Untagged VLAN: always allowed on the trunk" : b.dataset.name;
    });
  };
  $$("#rltag .pp", m).forEach((b) => { b.dataset.name = b.title; });
  paintNative();
  $("#rlnative", m).addEventListener("change", paintNative);
  show();
  $("#rlmode", m).addEventListener("click", (e) => {
    const b = e.target.closest("button");
    if (!b) return;
    $$("#rlmode button", m).forEach((x) => x.classList.toggle("on", x === b));
    show();
  });
  $("#rltag", m).addEventListener("click", (e) => {
    const b = e.target.closest(".pp");
    if (!b || b.classList.contains("N")) return;
    const id = +b.dataset.v;
    tagged.has(id) ? tagged.delete(id) : tagged.add(id);
    b.classList.toggle("T", tagged.has(id));
  });
  $("#rlprev", m).onclick = async () => {
    const md = mode(), vlan = +$(md === "access" ? "#rlvlan" : "#rlnative", m).value;
    // unchanged: send the VLAN back as the switch has it, so a VLAN given by name stays that way
    const same = existing && vlan === was.id && md === was.mode;
    const ch = { setRole: { name: $("#rlname", m).value.trim(), description: $("#rldesc", m).value.trim(), vlanMode: md,
      vlan: same ? existing.vlan || 0 : vlan, vlanName: same ? existing.vlanName || "" : "",
      // the untagged VLAN goes in the allowed list as well, or the switch drops its traffic
      tagged: md === "trunk" ? [...new Set([...tagged, ...(vlan ? [vlan] : [])])].sort((a, b) => a - b) : [] }, save: $("#rlsave", m).checked };
    if (!existing && (snap.portAccess.roles || []).some((r) => r.name === ch.setRole.name)) {
      $("#rlerr", m).innerHTML = `<div class="msg err">Role ${esc(ch.setRole.name)} already exists. Edit it from the list instead.</div>`;
      return;
    }
    try {
      const cmds = await call("PreviewPortAccess", S.detail.id, ch);
      closeModal();
      confirmCommands(ch, cmds, "ApplyPortAccess");
    } catch (e) {
      $("#rlerr", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
    }
  };
}

function deleteRole(name) {
  const ch = { deleteRole: name, save: true };
  call("PreviewPortAccess", S.detail.id, ch).then((cmds) => confirmCommands(ch, cmds, "ApplyPortAccess")).catch((e) => toast(e.message, true));
}

function paClick(e) {
  const b = e.target.closest(".pp");
  if (!b) return;
  const p = b.dataset.p, all = paPorts(S.detail.snapshot);
  if (e.shiftKey && S.paLast && all.includes(S.paLast)) {
    const [i, j] = [all.indexOf(S.paLast), all.indexOf(p)].sort((x, y) => x - y);
    const add = !S.paSel.has(p);
    for (const q of all.slice(i, j + 1)) add ? S.paSel.add(q) : S.paSel.delete(q);
  } else {
    S.paSel.has(p) ? S.paSel.delete(p) : S.paSel.add(p);
  }
  S.paLast = p;
  $$("#pagrid .pp").forEach((x) => x.classList.toggle("sel", S.paSel.has(x.dataset.p)));
  const ed = $("#paedit");
  ed.disabled = !S.paSel.size;
  ed.textContent = S.paSel.size ? `Edit ${plural(S.paSel.size, "port")}` : "Select ports to edit";
}

function paSelect(kind) {
  const snap = S.detail.snapshot, pa = snap.portAccess, infra = paInfra(snap);
  const auth = new Set((pa.ports || []).map((a) => a.port));
  const all = paPorts(snap);
  S.paSel = new Set(kind === "all" ? all : kind === "auth" ? [...auth] : kind === "open" ? all.filter((p) => !auth.has(p) && !infra[p]) : []);
  $("#tabbody").innerHTML = tabAccess(S.detail, snap);
}

function paEditOne(p) {
  S.paSel = new Set([p]);
  $("#tabbody").innerHTML = tabAccess(S.detail, S.detail.snapshot);
  openPortAccessEditor();
}

function openPortAccessEditor() {
  const snap = S.detail.snapshot, pa = snap.portAccess, cx = snap.family === "aos-cx";
  const all = paPorts(snap);
  const ports = all.filter((p) => S.paSel.has(p));
  if (!ports.length) return;
  const by = Object.fromEntries((pa.ports || []).map((a) => [a.port, a]));
  const infra = paInfra(snap);
  const now = (k) => {
    const n = ports.filter((p) => by[p]?.[k]).length;
    return n === 0 ? "Off on all" : n === ports.length ? "On on all" : `On on ${n} of ${ports.length}`;
  };
  const seg = (k) => `<div class="seg" data-k="${k}"><button type="button" data-v="" class="on">Keep</button><button type="button" data-v="1">On</button><button type="button" data-v="0">Off</button></div>`;
  const vlans = (snap.vlans || []).filter((v) => v.id !== 1);
  const roles = pa.roles || [];
  const keepRole = (k) => {
    const vals = [...new Set(ports.map((p) => by[p]?.roles?.[k] || ""))];
    return vals.length === 1 ? `Keep (${esc(vals[0] || "none")})` : "Keep (differs per port)";
  };
  const m = modal(`<h2>Port access on ${ports.length === 1 ? "port " + esc(ports[0]) : plural(ports.length, "port")}</h2><div class="body">
    ${ports.length > 1 ? `<div class="mono faint" style="font-size:12.5px;margin:-4px 0 12px;user-select:text">${esc(all.length === ports.length ? "All ports" : ports.join(", "))}</div>` : ""}
    <div class="pafield"><b>802.1X</b>${seg("dot1x")}<span class="now">${now("dot1x")}. Users and devices log in with their own account.</span></div>
    <div class="pafield"><b>MAC authentication</b>${seg("macAuth")}<span class="now">${now("macAuth")}. The device's MAC address is checked, for printers, phones and APs.</span></div>
    <div id="paoptsoff" class="faint" style="font-size:13px;margin-top:14px"></div>
    <div id="paopts">
    <div class="row" style="margin-top:16px">
      <label class="f"><span>Client limit</span><input type="number" id="palimit" min="0" max="${cx ? 256 : 32}" placeholder="Keep"></label>
      <label class="f"><span>Re-authentication</span><select id="pareauth"><option value="">Keep</option><option value="0">Off</option><option value="1800">Every 30 min</option><option value="3600">Every hour</option><option value="14400">Every 4 hours</option><option value="28800">Every 8 hours</option><option value="86400">Every 24 hours</option></select></label>
      ${cx ? "" : `<label class="f"><span>Unauthenticated VLAN</span><select id="paunauth"><option value="">Keep</option><option value="0">None</option>${vlans.map((v) => `<option value="${v.id}">${v.id}${v.name ? " · " + esc(v.name) : ""}</option>`).join("")}</select></label>`}
    </div>
    <div class="faint" style="font-size:12.5px;margin-top:-4px">Leave a field on Keep to leave it as it is. A client limit of 0 goes back to the switch default.${cx ? "" : " Clients that fail or skip the login are placed in the unauthenticated VLAN."}</div>
    ${cx ? `<div class="pasub">Roles<span class="faint">Which role, and so which VLAN, a client gets in each situation</span></div>
      ${roles.length ? "" : `<div class="msg warnm" style="margin-bottom:10px">This switch has no roles yet. Create them under Roles on the Port access tab first.</div>`}
      <div class="rolegrid">${ROLE_SLOTS.map(([k, l]) => `<label class="f"><span>${l}</span><select data-role="${k}"><option value="__keep">${keepRole(k)}</option><option value="">None</option>${roles.map((r) => `<option value="${esc(r.name)}">${esc(r.name)}${roleVLANText(r, snap) ? " · " + esc(roleVLANText(r, snap)) : ""}</option>`).join("")}</select></label>`).join("")}</div>` : ""}
    </div>
    <div id="pawarn" style="margin-top:12px"></div>
    <div style="height:10px"></div><label class="check"><input type="checkbox" id="pasave" checked> Save to startup config (write memory)</label>
    <div id="paerr"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="paprev">Review changes</button></div>`, true);
  const val = (k) => $(`.seg[data-k="${k}"] .on`, m).dataset.v;
  // whether a port still asks for a login once the toggles are applied
  const willAuth = (p) => (val("dot1x") === "" ? !!by[p]?.dot1x : val("dot1x") === "1") || (val("macAuth") === "" ? !!by[p]?.macAuth : val("macAuth") === "1");
  // options only mean something on a port with a login, so they show once every selected port has one
  const opts = () => {
    const open = ports.filter((p) => !willAuth(p));
    $("#paopts", m).style.display = open.length ? "none" : "";
    $("#paoptsoff", m).textContent = !open.length ? ""
      : open.length === ports.length ? `Turn on 802.1X or MAC authentication to set the client limit, re-authentication${cx ? " and roles" : " and unauthenticated VLAN"}.`
      : `${open.length === 1 ? "Port" : "Ports"} ${compress(open)} ${open.length === 1 ? "doesn't" : "don't"} ask for a login. Turn 802.1X or MAC authentication on for every selected port to change the other options.`;
    if (open.length) {
      // hidden fields go back to Keep, so nothing unseen is sent
      $("#palimit", m).value = "";
      for (const s of $$("#paopts select", m)) s.selectedIndex = 0;
    }
  };
  const warn = () => {
    const turnOn = val("dot1x") === "1" || val("macAuth") === "1";
    const risky = ports.filter((p) => infra[p]);
    const w = [];
    // AOS-CX keeps these on the interface; the backend removes them with the login
    const cleared = cx ? ports.filter((p) => !willAuth(p) && (by[p]?.dot1x || by[p]?.macAuth) && (by[p]?.clientLimit || Object.keys(by[p]?.roles || {}).length)) : [];
    if (cleared.length) w.push(`<div class="msg warnm">${cleared.length === 1 ? "Port" : "Ports"} <b>${esc(compress(cleared))}</b> will no longer ask for a login, so the roles and client limit set there are removed too.</div>`);
    if (turnOn && risky.length) w.push(`<div class="msg warnm"><b>${esc(risky.map((p) => `${p} (${infra[p]})`).join(", "))}</b> ${risky.length === 1 ? "links" : "link"} to another switch or access point. A login on ${risky.length === 1 ? "that port" : "those ports"} can cut the link, and with it SwitchDesk's own connection.</div>`);
    if (turnOn && !(pa.servers || []).length) w.push(`<div class="msg warnm">No RADIUS server is set, so nobody on these ports will be able to log in. Add a server first.</div>`);
    $("#pawarn", m).innerHTML = w.join("");
  };
  m.addEventListener("click", (e) => {
    const b = e.target.closest(".seg button");
    if (!b) return;
    b.parentElement.querySelectorAll("button").forEach((x) => x.classList.toggle("on", x === b));
    opts();
    warn();
  });
  opts();
  $("#paprev", m).onclick = async () => {
    const tri = (k) => (val(k) === "" ? null : val(k) === "1");
    const num = (v) => (v === "" || v === undefined ? null : +v);
    const ch = {
      ports, dot1x: tri("dot1x"), macAuth: tri("macAuth"),
      clientLimit: num($("#palimit", m).value.trim()), reauthPeriod: num($("#pareauth", m).value),
      unauthVlan: cx ? null : num($("#paunauth", m).value), save: $("#pasave", m).checked, roles: null,
    };
    for (const s of $$("select[data-role]", m)) if (s.value !== "__keep") (ch.roles ||= {})[s.dataset.role] = s.value;
    if ([ch.dot1x, ch.macAuth, ch.clientLimit, ch.reauthPeriod, ch.unauthVlan, ch.roles].every((v) => v === null)) {
      $("#paerr", m).innerHTML = `<div class="msg err">Nothing changed.</div>`;
      return;
    }
    try {
      const cmds = await call("PreviewPortAccess", S.detail.id, ch);
      closeModal();
      confirmCommands(ch, cmds, "ApplyPortAccess");
    } catch (e) {
      $("#paerr", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
    }
  };
}

function openRadiusEditor() {
  const cx = S.detail.snapshot.family === "aos-cx";
  const m = modal(`<h2>Add RADIUS server</h2><div class="body">
    <p class="muted" style="margin-top:0;font-size:13px">The switch sends every 802.1X and MAC authentication login to this server, such as ClearPass or Microsoft NPS. Add the switch as a client there with the same shared secret.</p>
    <div class="row"><label class="f"><span>Server IP address or host name</span><input type="text" id="rhost" placeholder="10.0.0.5" autocomplete="off" spellcheck="false"></label>
      ${cx ? `<label class="f" style="flex:0 0 160px"><span>VRF</span><input type="text" id="rvrf" placeholder="default" spellcheck="false"></label>` : ""}</div>
    <label class="f"><span>Shared secret</span><input type="password" id="rkey" maxlength="64" autocomplete="new-password"></label>
    <label class="check"><input type="checkbox" id="rshow"> Show secret</label>
    <div style="height:10px"></div><label class="check"><input type="checkbox" id="rsave" checked> Save to startup config (write memory)</label>
    <div id="rerr"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="rprev">Review changes</button></div>`);
  $("#rshow", m).onchange = (e) => { $("#rkey", m).type = e.target.checked ? "text" : "password"; };
  $("#rprev", m).onclick = async () => {
    const ch = { addServer: { host: $("#rhost", m).value.trim(), key: $("#rkey", m).value, vrf: cx ? $("#rvrf", m).value.trim() : "" }, save: $("#rsave", m).checked };
    try {
      const cmds = await call("PreviewPortAccess", S.detail.id, ch);
      closeModal();
      confirmCommands(ch, cmds, "ApplyPortAccess");
    } catch (e) {
      $("#rerr", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
    }
  };
}

// paGlobal turns 802.1X or MAC authentication on or off for the whole switch.
function paGlobal(kind, on) {
  const pa = S.detail.snapshot.portAccess;
  const label = kind === "dot1x" ? "802.1X" : "MAC authentication";
  const ports = (pa.ports || []).filter((a) => (kind === "dot1x" ? a.dot1x : a.macAuth)).map((a) => a.port);
  const ch = { [kind === "dot1x" ? "globalDot1x" : "globalMacAuth"]: on, save: true };
  let note = "";
  if (!on && ports.length) note = `${plural(ports.length, "port")} (${esc(compress(ports))}) stop checking logins with ${label}. Devices there may then reach the network without logging in. The port settings stay, and come back when you turn it on again.`;
  else if (!on) note = `No port uses ${label} now, so nothing changes until a port is given it.`;
  else if (!(pa.servers || []).length) note = `There's no RADIUS server yet, so no ${label} login can succeed. Add one first.`;
  else if (!ports.length) note = `No port uses ${label} yet. Turn it on per port below to ask for a login.`;
  call("PreviewPortAccess", S.detail.id, ch).then((cmds) => confirmCommands(ch, cmds, "ApplyPortAccess", note)).catch((e) => toast(e.message, true));
}

function removeRadius(host) {
  const ch = { removeServer: host, save: true };
  call("PreviewPortAccess", S.detail.id, ch).then((cmds) => confirmCommands(ch, cmds, "ApplyPortAccess")).catch((e) => toast(e.message, true));
}

// ---------- modals ----------
function modal(html, wide) {
  closeModal();
  const v = document.createElement("div");
  v.className = "veil";
  v.innerHTML = `<div class="modal${wide ? " wide" : ""}" role="dialog">${html}</div>`;
  v.addEventListener("click", (e) => { if (e.target === v || e.target.closest("[data-close]")) closeModal(); });
  document.body.appendChild(v);
  S.modalOpen = true;
  setTimeout(() => v.querySelector("input:not([disabled]),select,textarea")?.focus(), 30);
  return v;
}
// askConfirm replaces window.confirm(), which the app's embedded web view on
// macOS doesn't show (it answers "cancel" at once). It opens above any open
// dialog and leaves that dialog as it is.
function askConfirm(text, okLabel = "Delete", danger = true) {
  return new Promise((resolve) => {
    const v = document.createElement("div");
    v.className = "cveil";
    v.innerHTML = `<div class="modal" role="alertdialog" style="max-width:440px"><div class="body"><p style="margin:0;line-height:1.5">${esc(text)}</p></div>
      <div class="foot"><span class="grow"></span><button class="btn" data-no>Cancel</button><button class="btn ${danger ? "danger" : "primary"}" data-yes>${esc(okLabel)}</button></div></div>`;
    const done = (ok) => { document.removeEventListener("keydown", key, true); v.remove(); resolve(ok); };
    const key = (e) => {
      if (e.key === "Escape") { e.stopPropagation(); e.preventDefault(); done(false); }
      if (e.key === "Enter") { e.stopPropagation(); e.preventDefault(); done(true); }
    };
    v.addEventListener("click", (e) => { if (e.target === v || e.target.closest("[data-no]")) done(false); else if (e.target.closest("[data-yes]")) done(true); });
    document.addEventListener("keydown", key, true);
    document.body.appendChild(v);
    setTimeout(() => v.querySelector("[data-yes]")?.focus(), 30);
  });
}

function closeModal() {
  $(".veil")?.remove();
  if (S.modalOpen) { S.modalOpen = false; refreshData(); }
}
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && S.modalOpen) closeModal();
  const box = $("#swsearch") || $("#iq");
  if (e.key === "/" && !S.modalOpen && !/INPUT|TEXTAREA|SELECT/.test(document.activeElement?.tagName) && box) { e.preventDefault(); box.focus(); }
});

function credOptions(sel, kind = "") {
  return S.creds.filter((c) => (c.kind || "") === kind).map((c) => `<option value="${c.id}" ${c.id === sel ? "selected" : ""}>${esc(c.name)} (${esc(c.username)})</option>`).join("") + `<option value="__new">${kind ? "New Instant On account…" : "New credentials…"}</option>`;
}
const sshCreds = () => S.creds.filter((c) => !c.kind);
const cloudCreds = () => S.creds.filter((c) => c.kind === "instanton");
const FAMILIES = [["auto", "Detect automatically"], ["aos-cx", "AOS-CX"], ["aos-s", "AOS-S / ProCurve"]];
const BULK_FAMILIES = [...FAMILIES, ["instant-on", "Instant On (web interface, port 443)"]];
const SWITCH_TYPES = [...FAMILIES, ["instant-on", "Instant On (web interface)"]];

const addChips = (on) => `<div class="chips" style="margin-bottom:14px">${[["ssh", "Over SSH", "openSwitchModal()"], ["web", "Instant On (local)", "openSwitchModal(null, 'web')"], ["cloud", "Instant On cloud", "openCloudModal()"]].map(([k, l, f]) => `<button class="chip${k === on ? " on" : ""}" ${k === on ? "" : `onclick="${f}"`}>${l}</button>`).join("")}</div>`;

async function openSwitchModal(sw, mode) {
  if (sw?.cloud) return openCloudEdit(sw);
  // Instant On 1830/1930/1960 have no SSH: picking Instant On as the type
  // reads the switch through its web interface instead
  let web = mode === "web" || !!sw?.web || sw?.family === "instant-on";
  S.creds = await call("Credentials");
  const e = sw || { port: web ? 443 : 22, family: web ? "instant-on" : "auto", credentialId: sshCreds()[0]?.id };
  const m = modal(`<h2>${sw ? "Edit switch" : "Add switch"}</h2><div class="body">
    ${sw ? "" : addChips(web ? "web" : "ssh")}
    <p class="muted" id="webinfo" style="font-size:13px;margin-top:0;${web ? "" : "display:none"}">For Instant On switches in local management mode. SwitchDesk signs in to the switch's own web page with its admin login and reads it from there.</p>
    <div class="row"><label class="f"><span>Address</span><input type="text" id="sh" class="mono" placeholder="10.0.0.11" value="${esc(e.host || "")}"></label>
      <label class="f" style="flex:0 0 90px"><span id="splabel">${web ? "Port" : "SSH port"}</span><input type="number" id="sp" value="${web && (!e.port || e.port === 22) ? 443 : e.port || 22}"></label></div>
    <p class="faint" id="webhint" style="font-size:12.5px;margin:-4px 0 12px;${web ? "" : "display:none"}">443 for HTTPS, or 80 if the switch only answers on HTTP. Test connection tries the other one too.</p>
    <label class="f"><span>Name <span class="faint">(optional, the hostname is used otherwise)</span></span><input type="text" id="sn" value="${esc(e.name || "")}"></label>
    <div class="row"><label class="f"><span>Credentials</span><select id="sc">${credOptions(e.credentialId)}</select></label>
      <label class="f"><span>Switch type</span><select id="sf">${SWITCH_TYPES.map(([k, l]) => `<option value="${k}" ${k === (web ? "instant-on" : e.family || "auto") ? "selected" : ""}>${l}</option>`).join("")}</select></label></div>
    <div id="newcred" style="display:none" class="row"><label class="f"><span>Username</span><input type="text" id="cu" value="admin"></label><label class="f"><span>Password</span><input type="password" id="cp"></label></div>
    <label class="f"><span>Site</span><input type="text" id="ss" value="${esc(e.site || "")}"></label>
    <label class="f"><span>Notes</span><input type="text" id="so" value="${esc(e.notes || "")}"></label>
    <div id="smsg"></div></div>
    <div class="foot">${sw ? `<button class="btn danger" id="sdel">Remove</button>` : ""}<span class="grow"></span><button class="btn" id="stest">Test connection</button><button class="btn" data-close>Cancel</button><button class="btn primary" id="ssave">${sw ? "Save" : "Add switch"}</button></div>`);
  const sc = $("#sc", m);
  $("#sf", m).onchange = () => {
    web = $("#sf", m).value === "instant-on";
    $("#webinfo", m).style.display = $("#webhint", m).style.display = web ? "" : "none";
    $("#splabel", m).textContent = web ? "Port" : "SSH port";
    const sp = $("#sp", m);
    if (web && (+sp.value === 22 || !sp.value)) sp.value = 443;
    if (!web && (+sp.value === 443 || +sp.value === 80)) sp.value = 22;
    const chips = $(".chips", m);
    if (chips) chips.outerHTML = addChips(web ? "web" : "ssh");
  };
  const toggle = () => ($("#newcred", m).style.display = sc.value === "__new" || !sshCreds().length ? "flex" : "none");
  sc.onchange = toggle;
  if (!sshCreds().length) sc.value = "__new";
  toggle();
  const msg = (t, ok) => ($("#smsg", m).innerHTML = `<div class="msg ${ok ? "okm" : "err"}">${esc(t)}</div>`);
  const collect = async () => {
    let cid = sc.value;
    if (cid === "__new") {
      const c = await call("SaveCredential", { name: $("#cu", m).value.trim(), username: $("#cu", m).value.trim(), password: $("#cp", m).value });
      S.creds = await call("Credentials");
      sc.innerHTML = credOptions(c.id);
      sc.value = c.id;
      toggle();
      cid = c.id;
    }
    return { ...(sw || {}), host: $("#sh", m).value.trim(), port: +$("#sp", m).value || (web ? 443 : 22), name: $("#sn", m).value.trim(), credentialId: cid, family: web ? "instant-on" : $("#sf", m).value, web, site: $("#ss", m).value.trim(), notes: $("#so", m).value.trim() };
  };
  $("#stest", m).onclick = async () => {
    const b = $("#stest", m);
    b.disabled = true;
    b.textContent = "Connecting…";
    try { msg(await call("TestSwitch", await collect()), true); } catch (err) { msg(err.message); }
    b.disabled = false;
    b.textContent = "Test connection";
  };
  $("#ssave", m).onclick = async () => {
    try {
      const saved = await call("SaveSwitch", await collect());
      closeModal();
      toast(sw ? "Saved" : "Added " + saved.host + ". Collecting now.");
      if (!sw) location.hash = "#/switch/" + saved.id;
      else loadDetail(saved.id);
    } catch (err) { msg(err.message); }
  };
  if (sw) $("#sdel", m).onclick = async () => {
    if (!(await askConfirm(`Remove ${sw.name || sw.host} and its saved configs from SwitchDesk? The switch itself isn't changed.`, "Remove"))) return;
    await call("DeleteSwitch", sw.id);
    closeModal();
    location.hash = "#/switches";
  };
}

// Instant On switches managed from the cloud have no SSH; they're read from
// the Instant On portal with an Instant On account.
async function openCloudModal() {
  S.creds = await call("Credentials");
  const first = cloudCreds()[0]?.id;
  const m = modal(`<h2>Add switch</h2><div class="body">
    ${addChips("cloud")}
    <p class="muted" style="font-size:13px;margin-top:0">For Instant On switches managed in the Instant On app or portal. SwitchDesk signs in to the portal with your Instant On account and reads the switches from there.</p>
    <label class="f"><span>Instant On account</span><select id="ic">${credOptions(first, "instanton")}</select></label>
    <div id="inew" class="row" style="display:none"><label class="f"><span>Email</span><input type="text" id="iu" autocomplete="off"></label><label class="f"><span>Password</span><input type="password" id="ip"></label></div>
    <p class="faint" id="ihint" style="font-size:12.5px;margin:-4px 0 12px;display:none">Two-step verification has to be off for this account. A separate view-only account works well.</p>
    <div id="ilist"></div><div id="imsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn" id="ifind">Find switches</button><button class="btn primary" id="iadd" disabled>Add switches</button></div>`, true);
  const ic = $("#ic", m);
  const toggle = () => {
    const show = ic.value === "__new" || !cloudCreds().length;
    $("#inew", m).style.display = show ? "flex" : "none";
    $("#ihint", m).style.display = show ? "block" : "none";
  };
  if (!cloudCreds().length) ic.value = "__new";
  ic.onchange = () => { toggle(); $("#ilist", m).innerHTML = ""; $("#iadd", m).disabled = true; };
  toggle();
  const msg = (t, ok) => ($("#imsg", m).innerHTML = t ? `<div class="msg ${ok ? "okm" : "err"}">${esc(t)}</div>` : "");
  let sites = [];
  const account = async () => {
    if (ic.value !== "__new") return ic.value;
    const email = $("#iu", m).value.trim();
    const c = await call("SaveCredential", { name: email, username: email, password: $("#ip", m).value, kind: "instanton" });
    S.creds = await call("Credentials");
    ic.innerHTML = credOptions(c.id, "instanton");
    ic.value = c.id;
    toggle();
    return c.id;
  };
  const picked = () => [...m.querySelectorAll("input[data-dev]:checked")];
  const sync = () => {
    const n = picked().length;
    $("#iadd", m).disabled = !n;
    $("#iadd", m).textContent = n ? `Add ${n} switch${n === 1 ? "" : "es"}` : "Add switches";
  };
  $("#ifind", m).onclick = async () => {
    const b = $("#ifind", m);
    b.disabled = true;
    b.textContent = "Signing in…";
    msg("");
    try {
      const cid = await account();
      sites = await call("DiscoverCloud", cid);
      const total = sites.reduce((n, s) => n + s.switches.length, 0);
      $("#ilist", m).innerHTML = !sites.length ? `<div class="empty">This account has no sites.</div>` : sites.map((s, si) => `
        <div class="cloudsite"><div class="cloudsite-h"><b>${esc(s.name)}</b><span class="faint">${s.error ? esc(s.error) : plural(s.switches.length, "switch").replace("switchs", "switches")}</span></div>
        ${s.switches.map((d, di) => `<label class="check-row${d.addedAs ? " faint" : ""}"><input type="checkbox" data-dev="${si}:${di}" ${d.addedAs ? "disabled" : "checked"}>
          <span class="dot ${d.online ? "up" : "down"}"></span><span style="flex:1">${esc(d.name)} <span class="faint">${esc(d.model)}</span></span>
          <span class="mono faint">${esc(d.ip || "")}</span>${d.addedAs ? `<span class="faint" style="font-size:12px">already added</span>` : ""}</label>`).join("")}</div>`).join("");
      m.querySelectorAll("input[data-dev]").forEach((x) => (x.onchange = sync));
      sync();
      if (!total) msg("No switches found on this account's sites.");
    } catch (err) { msg(err.message); }
    b.disabled = false;
    b.textContent = "Find switches";
  };
  $("#iadd", m).onclick = async () => {
    const picks = picked().map((x) => {
      const [si, di] = x.dataset.dev.split(":").map(Number);
      const s = sites[si], d = s.switches[di];
      return { siteId: s.id, siteName: s.name, deviceId: d.deviceId, name: d.name, ip: d.ip };
    });
    try {
      const n = await call("AddCloud", ic.value, picks);
      closeModal();
      toast(`Added ${plural(n, "switch").replace("switchs", "switches")} from Instant On`);
      location.hash = "#/switches";
    } catch (err) { msg(err.message); }
  };
}

async function openCloudEdit(sw) {
  S.creds = await call("Credentials");
  const m = modal(`<h2>Edit switch</h2><div class="body">
    <p class="muted" style="font-size:13px;margin-top:0">Read from the Instant On portal: site <b>${esc(sw.cloud.siteName)}</b>, device <span class="mono">${esc(sw.cloud.deviceId)}</span>.</p>
    <label class="f"><span>Name <span class="faint">(optional, the portal name is used otherwise)</span></span><input type="text" id="en" value="${esc(sw.name || "")}"></label>
    <label class="f"><span>Instant On account</span><select id="ec">${credOptions(sw.credentialId, "instanton").replace(/<option value="__new">.*?<\/option>/, "")}</select></label>
    <label class="f"><span>Notes</span><input type="text" id="eo" value="${esc(sw.notes || "")}"></label><div id="emsg"></div></div>
    <div class="foot"><button class="btn danger" id="edel">Remove</button><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="esave">Save</button></div>`);
  $("#esave", m).onclick = async () => {
    try {
      await call("SaveSwitch", { ...sw, name: $("#en", m).value.trim(), credentialId: $("#ec", m).value, notes: $("#eo", m).value.trim() });
      closeModal();
      toast("Saved");
      loadDetail(sw.id);
    } catch (err) { $("#emsg", m).innerHTML = `<div class="msg err">${esc(err.message)}</div>`; }
  };
  $("#edel", m).onclick = async () => {
    if (!(await askConfirm(`Remove ${sw.name || sw.host} from SwitchDesk? Nothing changes on the switch or in Instant On.`, "Remove"))) return;
    await call("DeleteSwitch", sw.id);
    closeModal();
    location.hash = "#/switches";
  };
}

async function openBulkModal() {
  S.creds = await call("Credentials");
  const m = modal(`<h2>Add many switches</h2><div class="body">
    <label class="f"><span>One switch per line: an address, "name, address", or a range like 10.0.0.10-10.0.0.40</span>
    <textarea id="bt" rows="9" placeholder="10.0.0.11\ncore-cx, 10.0.0.1\n10.0.1.20-10.0.1.29"></textarea></label>
    <div class="row"><label class="f"><span>Credentials</span><select id="bc">${credOptions(sshCreds()[0]?.id)}</select></label>
      <label class="f"><span>Switch type</span><select id="bf">${BULK_FAMILIES.map(([k, l]) => `<option value="${k}">${l}</option>`).join("")}</select></label></div>
    <div id="bnew" class="row" style="display:none"><label class="f"><span>Username</span><input type="text" id="bu" value="admin"></label><label class="f"><span>Password</span><input type="password" id="bp"></label></div>
    <label class="f"><span>Site</span><input type="text" id="bs"></label><div id="bmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="bgo">Add switches</button></div>`);
  const bc = $("#bc", m);
  const toggle = () => ($("#bnew", m).style.display = bc.value === "__new" || !sshCreds().length ? "flex" : "none");
  if (!sshCreds().length) bc.value = "__new";
  bc.onchange = toggle;
  toggle();
  $("#bgo", m).onclick = async () => {
    try {
      let cid = bc.value;
      if (cid === "__new") cid = (await call("SaveCredential", { name: $("#bu", m).value.trim(), username: $("#bu", m).value.trim(), password: $("#bp", m).value })).id;
      const r = await call("BulkAdd", $("#bt", m).value, cid, $("#bs", m).value.trim(), $("#bf", m).value);
      if (r.errors?.length) {
        $("#bmsg", m).innerHTML = `<div class="msg err">Added ${r.added}. Skipped:<br>${r.errors.map(esc).join("<br>")}</div>`;
      } else {
        closeModal();
        toast(`Added ${plural(r.added, "switch").replace("switchs", "switches")}. Collecting now.`);
        location.hash = "#/dashboard";
      }
    } catch (err) { $("#bmsg", m).innerHTML = `<div class="msg err">${esc(err.message)}</div>`; }
  };
}

// ---------- network map ----------
function renderMap() {
  $("#main").innerHTML = `<div class="page full"><div class="mapwrap">
    <canvas id="map"></canvas>
    <div class="maptools"><h1>Network map</h1>
      <button class="btn small" onclick="SDMap.fit()">Fit to window</button>
      <button class="btn small" onclick="SDMap.relayout()">Arrange again</button>
      <label class="check btn small"><input type="checkbox" ${S.showEndpoints ? "checked" : ""} onchange="S.showEndpoints=this.checked;SDMap.reload()"> Phones and hosts</label>
      <span style="flex:1"></span><span class="faint" id="mapstat" style="font-size:13px"></span></div>
    <div class="maplegend"><span><i style="border-color:#5d7088"></i>Link</span><span><i style="border-color:var(--warn);border-top-style:dashed"></i>STP blocking</span><span><i style="border-color:var(--crit)"></i>Port down</span><span>Hover or select a switch to see port numbers</span></div>
    <div class="panel mapcard" id="mapcard" style="display:none"></div></div></div>`;
  SDMap.start($("#map"), {
    load: () => call("GetTopology", S.showEndpoints),
    saveLayout: (l) => call("SaveLayout", l),
    select: showMapCard,
    status: (t) => { const el = $("#mapstat"); if (el) el.textContent = t; },
  });
}

function showMapCard(n, topo) {
  const c = $("#mapcard");
  if (!c) return;
  if (!n) { c.style.display = "none"; return; }
  const links = topo.edges.filter((e) => e.from === n.id || e.to === n.id).map((e) => {
    const mine = e.from === n.id ? e.fromPort : e.toPort, other = e.from === n.id ? e.to : e.from, theirs = e.from === n.id ? e.toPort : e.fromPort;
    const on = topo.nodes.find((x) => x.id === other);
    return `<tr><td class="mono">${esc(mine)}</td><td>${esc(on?.label || other)}</td><td class="mono">${esc(theirs)}</td></tr>`;
  });
  c.style.display = "block";
  c.innerHTML = `<h3>${esc(n.label)}</h3><div class="muted" style="font-size:13px;margin-bottom:10px">${n.managed ? (n.online ? `<span class="ok">Online</span>` : `<span class="bad">Offline</span>`) + " " : ""}<span class="mono">${esc(n.host)}</span>${n.model ? "<br>" + esc(n.model) : ""}${n.stpRoot ? "<br>Spanning-tree root" : ""}</div>
    ${links.length ? `<table style="font-size:13px"><thead><tr><th style="padding:4px 6px">Port</th><th style="padding:4px 6px">To</th><th style="padding:4px 6px">Port</th></tr></thead><tbody>${links.join("").replaceAll("<td", '<td style="padding:4px 6px"')}</tbody></table>` : ""}
    ${n.managed ? `<div style="margin-top:12px"><button class="btn small primary" data-go="#/switch/${n.id}">Open switch</button></div>` : `<div class="faint" style="font-size:12.5px;margin-top:10px">Seen by LLDP/CDP, not in your inventory.</div>`}`;
}

// ---------- VLAN matrix ----------
// commonName is the name most switches give a VLAN.
function commonName(data, id) {
  const names = {};
  data.forEach((s) => { const v = (s.vlans || []).find((x) => x.id === id); if (v?.name) names[v.name] = (names[v.name] || 0) + 1; });
  return { common: Object.entries(names).sort((a, b) => b[1] - a[1])[0]?.[0] || "", names };
}

async function renderVLANs(soft) {
  const data = (await call("VLANOverview")) || [];
  data.forEach((s) => { s.vlans ||= []; });
  S.vlanData = data;
  if (soft && (S.modalOpen || S.route[0] !== "vlans")) return;
  if (!data.length) {
    $("#main").innerHTML = `<div class="page"><div class="head"><h1>VLANs</h1></div><div class="panel"><div class="empty"><h3>No VLAN data yet</h3><p>VLANs appear here once switches have been collected.</p></div></div></div>`;
    return;
  }
  const scroll = $("#vlmx") ? [$("#vlmx").scrollLeft, $("#vlmx").scrollTop] : null;
  const canEdit = data.some((s) => s.editable);
  const ids = [...new Set(data.flatMap((s) => s.vlans.map((v) => v.id)))].sort((a, b) => a - b);
  const rows = ids.map((id) => {
    const { common, names } = commonName(data, id);
    const cells = data.map((s) => {
      const v = s.vlans.find((x) => x.id === id);
      if (!v) return `<td class="faint">·</td>`;
      const n = (v.untagged?.length || 0) + (v.tagged?.length || 0);
      const diff = v.name && common && v.name !== common;
      return `<td><a class="cellv ${diff ? "diff" : "has"}" data-go="#/switch/${s.id}/vlans" title="${esc(v.name)}: ${v.untagged?.length || 0} untagged, ${v.tagged?.length || 0} tagged${diff ? " (name differs)" : ""}">${n || "–"}</a></td>`;
    }).join("");
    const mism = Object.keys(names).length > 1;
    const del = id !== 1 && data.some((s) => s.editable && s.vlans.some((x) => x.id === id));
    const edit = canEdit ? `<td style="text-align:right;white-space:nowrap"><button class="btn small" onclick="openNetVLAN(${id})">Edit</button>${del ? ` <button class="btn small danger" onclick="openNetVLANDelete(${id})">Delete</button>` : ""}</td>` : "";
    return `<tr><td><span class="mono">${id}</span> ${esc(common)}${mism ? ` <span class="sev warning" title="${esc(Object.keys(names).join(", "))}">names differ</span>` : ""}</td>${cells}${edit}</tr>`;
  });
  $("#main").innerHTML = `<div class="page"><div class="head"><h1>VLANs</h1><span class="sub">${ids.length} across ${plural(data.length, "switch").replace("switchs", "switches")}</span><span class="grow"></span>
      ${canEdit ? `<button class="btn primary" onclick="openNetVLAN()">${ICON.plus}New VLAN</button>` : ""}</div>
    <p class="muted" style="margin-top:-6px">Each cell shows how many ports carry the VLAN on that switch. Click one to edit it there${canEdit ? ", or use Edit to change the VLAN on every switch at once" : ""}.</p>
    <div class="panel" id="vlmx" style="overflow:auto"><table class="matrix"><thead><tr><th>VLAN</th>${data.map((s) => `<th><a data-go="#/switch/${s.id}/vlans" style="color:inherit" title="${esc(s.family)}${s.editable ? "" : ": VLANs " + esc(s.readOnly)}">${esc(s.name)}</a></th>`).join("")}${canEdit ? "<th></th>" : ""}</tr></thead><tbody>${rows.join("")}</tbody></table></div></div>`;
  if (scroll) [$("#vlmx").scrollLeft, $("#vlmx").scrollTop] = scroll;
}

// ---------- network-wide VLAN editor ----------
// The VLAN's name and which switches carry it are set once; each switch then
// gets the create, rename or delete it needs. Port membership differs per
// switch and stays on each switch's VLANs tab.
function netVLANPlan(s, id, name, want, save) {
  const v = (s.vlans || []).find((x) => x.id === id);
  if (want && !v) return { action: "create", id, name, save };
  if (!want && v) return { action: "delete", id, save };
  if (want && v && name && v.name !== name) return { action: "update", id, name, save };
  return null;
}

function netVLANLabel(s, ch) {
  if (!ch) return `<span class="faint">no change</span>`;
  if (ch.action === "create") return `<span class="sev okchip">add</span>`;
  if (ch.action === "update") return `<span class="sev info">rename</span>`;
  const v = s.vlans.find((x) => x.id === ch.id);
  const n = (v?.untagged?.length || 0) + (v?.tagged?.length || 0);
  return `<span class="sev critical">remove</span>${n ? ` <span class="faint" style="font-size:12px">from ${plural(n, "port")}</span>` : ""}`;
}

function openNetVLAN(id) {
  const data = S.vlanData || [];
  const isNew = !id;
  const common = isNew ? "" : commonName(data, id).common;
  const has = (s, vid) => (s.vlans || []).some((x) => x.id === vid);
  const want = {};
  for (const s of data) want[s.id] = isNew ? s.editable : has(s, id);
  const m = modal(`<h2>${isNew ? "New VLAN on several switches" : `VLAN ${id} on all switches`}</h2><div class="body">
    <div class="row"><label class="f" style="flex:0 0 120px"><span>VLAN ID</span><input type="number" id="nvid" min="1" max="4094" value="${id || ""}" ${isNew ? "" : "disabled"}></label>
      <label class="f"><span>Name</span><input type="text" id="nvname" maxlength="32" value="${esc(common)}" placeholder="${isNew ? "" : "leave empty to keep each switch's name"}"></label></div>
    <div class="muted" style="font-size:13px;margin-bottom:8px">Tick the switches that should carry this VLAN. Ports stay as they are; set port membership on each switch's VLANs tab.</div>
    <table><thead><tr><th style="width:28px"><input type="checkbox" id="nvall" title="All switches that can be changed"></th><th>Switch</th><th>Now</th><th>Change</th></tr></thead><tbody id="nvrows"></tbody></table>
    <div style="height:14px"></div><label class="check"><input type="checkbox" id="nvsave" checked> Save to startup config (write memory)</label>
    <div id="nverr"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="nvprev">Review changes</button></div>`, true);
  const vid = () => +$("#nvid", m).value;
  const plans = () => {
    const name = $("#nvname", m).value.trim(), save = $("#nvsave", m).checked;
    return data.map((s) => ({ s, ch: s.editable && vid() ? netVLANPlan(s, vid(), name, want[s.id], save) : null }));
  };
  const draw = () => {
    const v = vid();
    $("#nvrows", m).innerHTML = plans().map(({ s, ch }) => {
      const cur = (s.vlans || []).find((x) => x.id === v);
      const locked = !s.editable || (v === 1 && cur); // VLAN 1 can't be deleted
      const now = cur ? (cur.name ? esc(cur.name) : `<span class="faint">no name</span>`) : `<span class="faint">not present</span>`;
      return `<tr><td><input type="checkbox" data-s="${s.id}" ${want[s.id] ? "checked" : ""} ${locked ? "disabled" : ""}></td>
        <td>${esc(s.name)} <span class="faint" style="font-size:12px">${esc(s.family)}</span>${s.online ? "" : ` <span class="sev warning">offline</span>`}</td>
        <td>${v ? now : ""}</td><td>${s.editable ? (v ? netVLANLabel(s, ch) : "") : `<span class="faint" style="font-size:12px">VLANs ${esc(s.readOnly)}</span>`}</td></tr>`;
    }).join("");
    const ed = data.filter((s) => s.editable);
    $("#nvall", m).checked = ed.length > 0 && ed.every((s) => want[s.id]);
  };
  $("#nvrows", m).addEventListener("change", (e) => {
    const c = e.target.closest("[data-s]");
    if (c) { want[c.dataset.s] = c.checked; draw(); }
  });
  $("#nvall", m).onchange = (e) => {
    for (const s of data) if (s.editable && !(vid() === 1 && has(s, 1))) want[s.id] = e.target.checked;
    draw();
  };
  $("#nvid", m).oninput = () => {
    for (const s of data) want[s.id] = s.editable; // a new VLAN goes on every switch that can take it
    draw();
  };
  $("#nvname", m).oninput = draw;
  $("#nvsave", m).onchange = draw;
  $("#nvprev", m).onclick = async () => {
    const err = (t) => { $("#nverr", m).innerHTML = `<div class="msg err">${esc(t)}</div>`; };
    const v = vid();
    if (!(v >= 1 && v <= 4094)) return err("VLAN ID must be 1-4094.");
    if (isNew && data.some((s) => has(s, v))) return err(`VLAN ${v} already exists on some switches. Close this and use Edit on its row instead.`);
    const todo = plans().filter((p) => p.ch);
    if (!todo.length) return err("Nothing changed.");
    const b = $("#nvprev", m);
    b.disabled = true;
    const previews = await Promise.all(todo.map(async (p) => {
      try { return { ...p, cmds: await call("PreviewVLAN", p.s.id, p.ch) }; } catch (e) { return { ...p, error: e.message }; }
    }));
    netVLANReview(v, previews);
  };
  draw();
}

// Deletes a VLAN from the ticked switches. Only switches that carry it and can
// be changed are offered; the review step is the same as for edits.
function openNetVLANDelete(id) {
  const data = (S.vlanData || []).filter((s) => (s.vlans || []).some((x) => x.id === id));
  const name = commonName(data, id).common;
  const pick = {};
  for (const s of data) pick[s.id] = s.editable;
  const m = modal(`<h2>Delete VLAN ${id}${name ? ` (${esc(name)})` : ""}</h2><div class="body">
    <div class="muted" style="font-size:13px;margin-bottom:8px">Tick the switches to delete this VLAN from. Ports that carry it lose it.</div>
    <table><thead><tr><th style="width:28px"><input type="checkbox" id="ndall" title="All switches that can be changed"></th><th>Switch</th><th>Name</th><th>Ports</th></tr></thead><tbody id="ndrows"></tbody></table>
    <div style="height:14px"></div><label class="check"><input type="checkbox" id="ndsave" checked> Save to startup config (write memory)</label>
    <div id="nderr"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn danger" id="ndprev">Review changes</button></div>`, true);
  const draw = () => {
    $("#ndrows", m).innerHTML = data.map((s) => {
      const v = s.vlans.find((x) => x.id === id);
      const n = (v.untagged?.length || 0) + (v.tagged?.length || 0);
      return `<tr><td><input type="checkbox" data-s="${s.id}" ${pick[s.id] ? "checked" : ""} ${s.editable ? "" : "disabled"}></td>
        <td>${esc(s.name)} <span class="faint" style="font-size:12px">${esc(s.family)}</span>${s.online ? "" : ` <span class="sev warning">offline</span>`}</td>
        <td>${v.name ? esc(v.name) : `<span class="faint">no name</span>`}</td>
        <td>${s.editable ? (n ? plural(n, "port") : `<span class="faint">none</span>`) : `<span class="faint" style="font-size:12px">VLANs ${esc(s.readOnly)}</span>`}</td></tr>`;
    }).join("");
    const ed = data.filter((s) => s.editable);
    $("#ndall", m).checked = ed.length > 0 && ed.every((s) => pick[s.id]);
  };
  $("#ndrows", m).addEventListener("change", (e) => {
    const c = e.target.closest("[data-s]");
    if (c) { pick[c.dataset.s] = c.checked; draw(); }
  });
  $("#ndall", m).onchange = (e) => {
    for (const s of data) if (s.editable) pick[s.id] = e.target.checked;
    draw();
  };
  $("#ndprev", m).onclick = async () => {
    const save = $("#ndsave", m).checked;
    const todo = data.filter((s) => s.editable && pick[s.id]).map((s) => ({ s, ch: { action: "delete", id, save } }));
    if (!todo.length) { $("#nderr", m).innerHTML = `<div class="msg err">Tick at least one switch.</div>`; return; }
    $("#ndprev", m).disabled = true;
    const previews = await Promise.all(todo.map(async (p) => {
      try { return { ...p, cmds: await call("PreviewVLAN", p.s.id, p.ch) }; } catch (e) { return { ...p, error: e.message }; }
    }));
    netVLANReview(id, previews);
  };
  draw();
}

function netVLANReview(id, items) {
  const ok = items.filter((p) => !p.error);
  const removes = ok.filter((p) => p.ch.action === "delete").length;
  const block = (p, i) => `<div class="panel" style="margin:0 0 10px;padding:10px 12px">
      <div style="display:flex;align-items:center;gap:8px;margin-bottom:6px"><b>${esc(p.s.name)}</b>${netVLANLabel(p.s, p.ch)}<span class="grow" style="flex:1"></span><span id="nvst${i}"></span></div>
      ${p.error ? `<div class="msg err">${esc(p.error)}. This switch is skipped.</div>` : `<pre class="code mono" id="nvtx${i}" style="margin:0">${esc(p.cmds.join("\n"))}</pre>`}</div>`;
  const m = modal(`<h2>Review VLAN ${id} changes</h2><div class="body">
    <p class="muted" style="margin-top:0">${plural(ok.length, "switch").replace("switchs", "switches")} will be changed one after the other. On each switch, sending stops at the first line it rejects${ok[0]?.ch.save ? ", and nothing is saved to startup config there" : ""}. A failure on one switch doesn't stop the others.</p>
    ${removes ? `<div class="msg err" style="margin-bottom:12px">VLAN ${id} is removed from ${plural(removes, "switch").replace("switchs", "switches")}. Ports that carried it lose it.</div>` : ""}
    ${items.map(block).join("")}<div id="nvres"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="nvapply" ${ok.length ? "" : "disabled"}>Apply to ${plural(ok.length, "switch").replace("switchs", "switches")}</button></div>`, true);
  $("#nvapply", m).onclick = async () => {
    const b = $("#nvapply", m);
    b.disabled = true;
    b.textContent = "Sending…";
    $("[data-close]", m).disabled = true;
    let good = 0;
    for (const [i, p] of items.entries()) {
      if (p.error) continue;
      const st = $(`#nvst${i}`, m);
      st.innerHTML = `<span class="faint">sending…</span>`;
      try {
        const r = await call("ApplyVLAN", p.s.id, p.ch);
        $(`#nvtx${i}`, m).textContent = r.transcript.join("\n");
        st.innerHTML = r.ok ? `<span class="sev okchip">done</span>` : `<span class="sev critical" title="${esc(r.error)}">failed</span>`;
        if (!r.ok) $(`#nvtx${i}`, m).insertAdjacentHTML("afterend", `<div class="msg err">${esc(r.error)}</div>`);
        if (r.ok) good++;
      } catch (e) {
        st.innerHTML = `<span class="sev critical">failed</span>`;
        $(`#nvtx${i}`, m).insertAdjacentHTML("afterend", `<div class="msg err">${esc(e.message)}</div>`);
      }
    }
    $("#nvres", m).innerHTML = good === ok.length
      ? `<div class="msg okm">Done on ${plural(good, "switch").replace("switchs", "switches")}. They're being re-read.</div>`
      : `<div class="msg err">${good} of ${ok.length} switches changed. See the errors above.</div>`;
    b.remove();
    const c = $("[data-close]", m);
    c.disabled = false;
    c.textContent = "Close";
  };
}

// ---------- settings ----------
async function renderSettings() {
  S.creds = await call("Credentials");
  S.settings = await call("GetSettings");
  const set = S.settings;
  const ver = await call("Version");
  const df = await call("DataFolder");
  const lk = await call("AppLock");
  const langs = await I18N.languages(api);
  const cur = S.settings.language || "";
  $("#main").innerHTML = `<div class="page" style="max-width:900px"><div class="head"><h1>Settings</h1></div>
    <section class="panel" style="margin-bottom:18px"><h2>Language</h2><div class="body">
      <div class="row" style="align-items:flex-end"><label class="f"><span>Interface language</span><select id="lang" onchange="setLanguage(this.value)" data-notr>
          <option value="" ${cur === "" ? "selected" : ""}>Automatic (${esc(langName(langs, I18N.code))})</option>
          ${langs.map((l) => `<option value="${esc(l.code)}" ${cur === l.code ? "selected" : ""}>${esc(l.name)}${l.status === "draft" ? " " + I18N.tr("· draft") : ""}${l.own ? " " + I18N.tr("· your file") : ""}</option>`).join("")}</select></label>
        <span style="flex:none;display:flex;gap:6px;margin-bottom:14px"><button class="btn" onclick="call('OpenTranslationsFolder')">Translation files</button></span></div>
      <p class="faint" style="font-size:12.5px;margin:-4px 0 0">Translations are made by people who use SwitchDesk. Missing a language, or found a wrong word? <a onclick="openURL('https://jaba0x.github.io/SwitchDesk/docs/translating/')">Help translate</a>. Switch output, configs and addresses always stay as the switch sends them.</p></div></section>
    <section class="panel" style="margin-bottom:18px"><h2>Credentials<span class="grow"></span><button class="btn small" onclick="editCred()">${ICON.plus}Add</button></h2>
      ${S.creds.length ? `<table><tbody>${S.creds.map((c) => `<tr><td>${esc(c.name)}${c.kind === "instanton" ? ' <span class="faint">Instant On account</span>' : ""}</td><td class="mono">${esc(c.username)}</td><td style="text-align:right"><button class="btn small" onclick="editCred('${c.id}')">Edit</button> <button class="btn small danger" onclick="delCred('${c.id}')">Remove</button></td></tr>`).join("")}</tbody></table>` : `<div class="empty">No credentials yet. They're added when you add your first switch.</div>`}
      <div class="faint" style="padding:10px 16px;font-size:12.5px">Passwords are encrypted on this computer and never shown again.</div></section>
    <section class="panel"><h2>Monitoring</h2><div class="body">
      <div class="row"><label class="f"><span>Status check</span><select id="cm" onchange="checkMethodHint()">
          <option value="port" ${set.checkMethod === "port" ? "selected" : ""}>SSH port</option>
          <option value="ping" ${set.checkMethod === "ping" ? "selected" : ""}>Ping (ICMP)</option>
          <option value="off" ${set.checkMethod === "off" ? "selected" : ""}>Off</option></select></label>
        <label class="f"><span>Check every (seconds)</span><input type="number" id="pi" min="5" value="${set.pingInterval}" ${set.checkMethod === "off" ? "disabled" : ""}></label></div>
      <p class="faint" id="cmhint" style="font-size:12.5px;margin:-6px 0 14px">${checkMethodText(set.checkMethod)}</p>
      <div class="row"><label class="f"><span>Read switches over SSH every (minutes, 0 = only on demand)</span><input type="number" id="ci" min="0" value="${set.collectInterval}"></label><span></span></div>
      <div class="row"><label class="f"><span>Switches read at the same time</span><input type="number" id="pa" min="1" max="16" value="${set.parallel}"></label>
        <label class="f"><span>Config copies kept per switch</span><input type="number" id="kb" min="1" value="${set.keepBackups}"></label></div>
      <div class="row"><label class="f"><span>Warn above CPU %</span><input type="number" id="cw" value="${set.cpuWarn}"></label>
        <label class="f"><span>Warn above memory %</span><input type="number" id="mw" value="${set.memWarn}"></label></div>
      <label class="check" style="margin-bottom:18px"><input type="checkbox" id="is" ${set.ignoreSnmp ? "checked" : ""}> Don't flag default SNMP communities</label>
      <div style="border-top:1px solid var(--line-soft);padding-top:14px;margin-bottom:16px"><b style="font-weight:600">Automatic firmware updates</b>
        <p class="faint" style="font-size:12.5px;margin:4px 0 10px">Uses the newest image in the firmware library for each switch. SwitchDesk has to be running for this.</p>
        <label class="check" style="margin-bottom:6px"><input type="radio" name="fa" value="off" ${set.firmwareAuto === "off" ? "checked" : ""}> Off</label>
        <label class="check" style="margin-bottom:6px"><input type="radio" name="fa" value="stage" ${set.firmwareAuto === "stage" ? "checked" : ""}> Copy new firmware to the spare bank, but don't reboot</label>
        <label class="check" style="margin-bottom:10px"><input type="radio" name="fa" value="install" ${set.firmwareAuto === "install" ? "checked" : ""}> Install: copy any time, reboot one switch at a time during the maintenance window</label>
        <div class="row" style="max-width:360px"><label class="f"><span>Window starts</span><input type="text" id="ws" class="mono" value="${esc(set.windowStart)}" placeholder="02:00"></label>
          <label class="f"><span>Window ends</span><input type="text" id="we" class="mono" value="${esc(set.windowEnd)}" placeholder="05:00"></label></div></div>
      <button class="btn primary" onclick="saveSettings()">Save settings</button></div></section>
    <section class="panel" style="margin-top:18px"><h2>App password<span class="grow"></span>${lk.enabled ? `<span class="okchip sev" style="font-weight:400">On</span>` : `<span class="faint" style="font-weight:400;font-size:13px">Off</span>`}</h2><div class="body">
      <p class="muted" style="font-size:13px;margin:0 0 12px">Optional. With a password, SwitchDesk asks for it when it starts, and the saved switch passwords can't be read without it, not even from the data folder. Without one, anyone who can use this computer account can open SwitchDesk.</p>
      ${lk.enabled ? `<div class="row" style="align-items:flex-end;max-width:560px"><label class="f"><span>Lock the window after</span><select id="la" onchange="saveLockAfter()">
          ${[0, 5, 15, 30, 60, 240].map((n) => `<option value="${n}" ${n === (lk.lockAfter || 0) ? "selected" : ""}>${n ? (n < 60 ? n + " minutes idle" : n / 60 + (n === 60 ? " hour" : " hours") + " idle") : "Never while running"}</option>`).join("")}</select></label>
          <span style="flex:none;display:flex;gap:6px;margin-bottom:14px"><button class="btn" onclick="lockNow()">Lock now</button></span></div>
        <div style="display:flex;gap:8px"><button class="btn" onclick="openAppPassword('change')">Change password…</button><button class="btn danger" onclick="openAppPassword('remove')">Turn off…</button></div>`
      : `<button class="btn primary" onclick="openAppPassword('set')">Set a password…</button>`}</div></section>
    <section class="panel" style="margin-top:18px"><h2>Data</h2><div class="body">
      <label class="f"><span>Data folder${df.custom ? "" : ' <span class="faint">(default)</span>'}</span>
        <div class="row" style="align-items:center"><input type="text" class="mono" readonly value="${esc(df.path)}"><span style="flex:none;display:flex;gap:6px">
        <button class="btn" onclick="pickDataFolder()">Change…</button><button class="btn" onclick="call('OpenDataFolder')">Open</button>
        ${df.custom ? `<button class="btn" onclick="resetDataFolder()">Use default</button>` : ""}</span></div></label>
      <p class="faint" style="font-size:12.5px;margin:-4px 0 18px">Holds your switches, encrypted credentials, saved configs and map layout. A folder on another disk or a synced drive works; the encryption key travels with it.</p>
      <div class="row" style="align-items:stretch">
        <div class="panel" style="padding:14px 16px;background:var(--panel-2)"><b style="font-weight:600">Export to another computer</b>
          <p class="muted" style="font-size:13px;margin:6px 0 12px">Saves everything to one .sdbak file. Passwords inside are protected by a passphrase you choose here and need again when importing.</p>
          <button class="btn primary" onclick="openExport()">Export…</button></div>
        <div class="panel" style="padding:14px 16px;background:var(--panel-2)"><b style="font-weight:600">Import from a file</b>
          <p class="muted" style="font-size:13px;margin:6px 0 12px">Add the switches from a .sdbak file to this computer, or replace everything here with its contents.</p>
          <button class="btn" onclick="openImport()">Import…</button></div>
      </div></div></section>
    <section class="panel about" style="margin-top:18px"><div class="body">
      <a class="about-logo" onclick="openURL('https://clihero.com')" title="clihero.com"><img src="clihero-logo-white.png" alt="clihero.com"></a>
      <div><b>SwitchDesk ${esc(ver)}</b>
        <div class="muted" style="font-size:13px;margin-top:2px">Built by Jaba Macharashvili · <a onclick="openURL('https://clihero.com')">clihero.com</a> · <a onclick="openURL('https://jaba0x.github.io/SwitchDesk/')">Documentation</a> · <a onclick="openURL('https://github.com/jaba0x/SwitchDesk')">GitHub</a></div>
        <div style="margin-top:10px;display:flex;gap:10px;align-items:center"><button class="btn small" onclick="checkUpdate(this)">${ICON.refresh}Check for updates</button><span id="updres" class="muted" style="font-size:13px"></span></div></div>
    </div></section></div>`;
}

// ---------- language ----------
function langName(list, code) { return (list.find((l) => l.code === code) || { name: "English" }).name; }

async function setLanguage(code) {
  try {
    await call("SaveSettings", { ...S.settings, language: code });
    S.settings = await call("GetSettings");
    await I18N.use(code, api);
    renderSide();
    await renderSettings();
  } catch (e) { toast(e.message, true); }
}

// ---------- optional app password (#13) ----------
function openAppPassword(mode) {
  const title = { set: "Set an app password", change: "Change the app password", remove: "Turn off the app password" }[mode];
  const m = modal(`<h2>${title}</h2><div class="body">
    ${mode !== "set" ? `<label class="f"><span>Current password</span><input type="password" id="ap0" autocomplete="current-password"></label>` : ""}
    ${mode !== "remove" ? `<div class="row"><label class="f"><span>New password</span><input type="password" id="ap1" autocomplete="new-password"></label>
      <label class="f"><span>Repeat it</span><input type="password" id="ap2" autocomplete="new-password"></label></div>
      <p class="faint" style="font-size:12.5px;margin:-4px 0 0">At least 8 characters. There's no way to recover it: if it's forgotten, the saved switch passwords have to be entered again.</p>`
    : `<p class="muted" style="margin:0">SwitchDesk will open without asking, and the key for the saved switch passwords goes back to being protected only by your computer account.</p>`}
    <div id="apmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn ${mode === "remove" ? "danger" : "primary"}" id="apgo">${mode === "remove" ? "Turn off" : "Save password"}</button></div>`);
  setTimeout(() => $("input", m)?.focus(), 30);
  $("#apgo", m).onclick = async () => {
    const err = (t) => ($("#apmsg", m).innerHTML = `<div class="msg err">${esc(t)}</div>`);
    const cur = $("#ap0", m)?.value || "";
    let next = "";
    if (mode !== "remove") {
      next = $("#ap1", m).value;
      if (next.length < 8) return err("Use at least 8 characters.");
      if (next !== $("#ap2", m).value) return err("The passwords don't match.");
    }
    try {
      await call("SetAppPassword", cur, next);
      closeModal();
      toast(mode === "remove" ? "App password turned off" : "App password saved");
      setupIdleLock();
      renderSettings();
    } catch (e) { err(e.message); }
  };
}

async function saveLockAfter() {
  try {
    await call("SaveSettings", { ...S.settings, lockAfter: +$("#la").value });
    S.settings = await call("GetSettings");
    setupIdleLock();
    toast("Saved");
  } catch (e) { toast(e.message, true); }
}

async function lockNow() {
  await call("LockNow");
  await showLock(await call("AppLock"));
}

// Shows the lock screen and resolves once the right password is entered.
function showLock(info) {
  return new Promise((resolve) => {
    closeModal();
    $("#lockscreen")?.remove();
    const v = document.createElement("div");
    v.id = "lockscreen";
    v.className = "lockscreen";
    v.innerHTML = `<form class="lockbox" autocomplete="off">
      <img src="clihero-logo-white.png" alt="" style="width:120px;opacity:.85;margin-bottom:6px">
      <h2>SwitchDesk is locked</h2>
      <p class="muted">${info.starting ? "Enter the app password to open your switches." : "Enter the app password to continue."}</p>
      <input type="password" id="lkp" placeholder="App password" autocomplete="current-password">
      <div id="lkmsg"></div>
      <button class="btn primary" id="lkgo" type="submit">Unlock</button>
      ${info.starting ? `<a class="faint" id="lkforgot" style="font-size:12.5px;margin-top:6px;cursor:pointer">Forgot the password?</a>` : ""}
    </form>`;
    document.body.appendChild(v);
    const inp = $("#lkp", v);
    setTimeout(() => inp.focus(), 30);
    $("form", v).onsubmit = async (e) => {
      e.preventDefault();
      const b = $("#lkgo", v);
      b.disabled = true; b.textContent = "Checking…";
      try {
        await call("Unlock", inp.value);
        v.remove();
        resolve();
      } catch (err) {
        $("#lkmsg", v).innerHTML = `<div class="msg err">${esc(err.message === "wrong password" ? "That's not the password." : err.message)}</div>`;
        inp.value = ""; inp.focus();
        b.disabled = false; b.textContent = "Unlock";
      }
    };
    const f = $("#lkforgot", v);
    if (f) f.onclick = () => {
      $("#lkmsg", v).innerHTML = `<div class="msg warnm" style="text-align:left">Without the password, the saved switch passwords can't be read. SwitchDesk can start over without it: your switches, settings, config copies and map stay, but every saved password is cleared and has to be entered again under Settings → Credentials.
        <div style="margin-top:10px"><button class="btn danger small" type="button" id="lkreset">Clear saved passwords and open</button></div></div>`;
      $("#lkreset", v).onclick = async () => {
        try { await call("ForgetAppPassword"); v.remove(); toast("App password removed. Enter the switch passwords again under Settings → Credentials.", true); resolve(); }
        catch (err) { toast(err.message, true); }
      };
    };
  });
}

// Locks the window after the idle time set in Settings.
let idleTimer = null;
function setupIdleLock() {
  clearTimeout(idleTimer);
  call("AppLock").then((info) => {
    S.lockAfter = info.enabled ? info.lockAfter : 0;
    bumpIdle();
  }).catch(() => {});
}
function bumpIdle() {
  clearTimeout(idleTimer);
  if (!S.lockAfter || $("#lockscreen")) return;
  idleTimer = setTimeout(() => lockNow().then(bumpIdle), S.lockAfter * 60000);
}
for (const ev of ["mousemove", "keydown", "mousedown", "wheel"]) document.addEventListener(ev, () => { if (S.lockAfter) bumpIdle(); }, { passive: true });

// ---------- data folder, export, import ----------
async function pickDataFolder() {
  let info;
  try { info = await call("PickDataFolder"); } catch (e) { toast(e.message, true); return; }
  if (!info) return;
  confirmFolder(info, false);
}

async function resetDataFolder() {
  confirmFolder(await call("DefaultFolderInfo"), true);
}

function confirmFolder(info, isDefault) {
  if (info.same) { toast("That's already the data folder"); return; }
  if (!info.writable) { toast("SwitchDesk can't write to " + info.path, true); return; }
  const where = isDefault ? "the default folder" : "this folder";
  const choices = info.hasData
    ? `<p>${isDefault ? "The default folder" : "This folder"} already has SwitchDesk data with ${plural(info.switches, "switch").replace("switchs", "switches")}.</p>
       <label class="check" style="margin-bottom:8px"><input type="radio" name="fm" value="use" checked> Use the data that's there</label>
       <label class="check"><input type="radio" name="fm" value="move"> Replace it with my current data</label>`
    : `<p>Your current data will be copied to ${where}.</p>
       <label class="check" style="margin-bottom:8px"><input type="radio" name="fm" value="move" checked> Move it, and remove it from the old folder</label>
       <label class="check"><input type="radio" name="fm" value="copy"> Copy it, and leave the old folder as it is</label>`;
  const m = modal(`<h2>Change data folder</h2><div class="body"><p class="mono" style="margin-top:0;user-select:text;word-break:break-all">${esc(info.path)}</p>${choices}<div id="fmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="fgo">Switch folder</button></div>`);
  $("#fgo", m).onclick = async () => {
    const mode = $("input[name=fm]:checked", m).value;
    const b = $("#fgo", m);
    b.disabled = true; b.textContent = "Switching…";
    try {
      await call("ChangeDataFolder", info.path, mode);
      closeModal();
      toast("Now using " + info.path);
      renderSettings();
    } catch (e) {
      $("#fmsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`;
      b.disabled = false; b.textContent = "Switch folder";
    }
  };
}

function openExport() {
  const m = modal(`<h2>Export to another computer</h2><div class="body">
    <div class="row"><label class="f"><span>Passphrase</span><input type="password" id="ep1" autocomplete="new-password"></label>
    <label class="f"><span>Repeat passphrase</span><input type="password" id="ep2" autocomplete="new-password"></label></div>
    <p class="faint" style="font-size:12.5px;margin:-4px 0 14px">At least 8 characters. Without it, the passwords in the file can't be read, so keep it somewhere safe.</p>
    <label class="check"><input type="checkbox" id="eh" checked> Include saved configs and last readings</label><div id="emsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="ego">Export…</button></div>`);
  $("#ego", m).onclick = async () => {
    const p1 = $("#ep1", m).value, p2 = $("#ep2", m).value;
    const err = (t) => ($("#emsg", m).innerHTML = `<div class="msg err">${esc(t)}</div>`);
    if (p1.length < 8) return err("Use at least 8 characters.");
    if (p1 !== p2) return err("The passphrases don't match.");
    try {
      const path = await call("ExportData", p1, $("#eh", m).checked);
      closeModal();
      toast("Exported to " + path);
    } catch (e) { if (e.message !== "cancelled") err(e.message); }
  };
}

async function openImport() {
  let path;
  try { path = await call("PickImportFile"); } catch (e) { toast(e.message, true); return; }
  if (!path) return;
  const m = modal(`<h2>Import from a file</h2><div class="body">
    <p class="mono" style="margin-top:0;user-select:text;word-break:break-all">${esc(path)}</p>
    <label class="f"><span>Passphrase used for the export</span><input type="password" id="ip" autocomplete="off"></label>
    <div id="iinfo"></div>
    <label class="check" style="margin-bottom:8px"><input type="radio" name="im" value="merge" checked> Add its switches to the ones here <span class="faint">(switches already here are skipped)</span></label>
    <label class="check"><input type="radio" name="im" value="replace"> Replace everything here with the file's contents</label>
    <div id="imsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="igo">Import</button></div>`);
  const err = (t) => ($("#imsg", m).innerHTML = `<div class="msg err">${esc(t)}</div>`);
  $("#ip", m).onchange = async () => {
    try {
      const i = await call("InspectImport", path, $("#ip", m).value);
      $("#iinfo", m).innerHTML = `<p class="muted" style="font-size:13px;margin:-4px 0 14px">${plural(i.switches, "switch").replace("switchs", "switches")}, exported ${new Date(i.createdAt).toLocaleString()}${i.history ? ", with saved configs" : ""}.</p>`;
      $("#imsg", m).innerHTML = "";
    } catch (e) { err(e.message); }
  };
  $("#igo", m).onclick = async () => {
    const mode = $("input[name=im]:checked", m).value;
    if (mode === "replace" && !(await askConfirm("Replace all switches, credentials and settings on this computer with the file's contents?", "Replace"))) return;
    try {
      const r = await call("ImportData", path, $("#ip", m).value, mode);
      closeModal();
      toast(`Imported ${plural(r.added, "switch").replace("switchs", "switches")}${r.skipped?.length ? `, skipped ${r.skipped.length} already here` : ""}`);
      location.hash = "#/dashboard";
    } catch (e) { err(e.message === "wrong passphrase" ? "That passphrase doesn't open this file." : e.message); }
  };
}

function checkMethodText(m) {
  if (m === "ping") return "Pings each switch. Opens no SSH connections, but ping (ICMP) has to reach the switches.";
  if (m === "off") return "No background checks. Online and offline only change when a switch is read, on the schedule below or with Collect now.";
  return "Opens a connection to each switch's SSH port. Switches log every check as an SSH connection.";
}

function checkMethodHint() {
  const m = $("#cm").value;
  $("#cmhint").textContent = checkMethodText(m);
  $("#pi").disabled = m === "off";
}

async function saveSettings() {
  try {
    await call("SaveSettings", { checkMethod: $("#cm").value, pingInterval: +$("#pi").value, collectInterval: +$("#ci").value, parallel: +$("#pa").value, keepBackups: +$("#kb").value, cpuWarn: +$("#cw").value, memWarn: +$("#mw").value, ignoreSnmp: $("#is").checked, firmwareAuto: $("input[name=fa]:checked").value, windowStart: $("#ws").value.trim(), windowEnd: $("#we").value.trim(), lockAfter: S.settings.lockAfter || 0, language: S.settings.language || "" });
    S.settings = await call("GetSettings");
    toast("Settings saved");
  } catch (e) { toast(e.message, true); }
}

function editCred(id) {
  const c = S.creds.find((x) => x.id === id) || {};
  const m = modal(`<h2>${id ? "Edit credentials" : "Add credentials"}</h2><div class="body">
    <label class="f"><span>Name</span><input type="text" id="cn" value="${esc(c.name || "")}" placeholder="Core switches"></label>
    <div class="row"><label class="f"><span>Username</span><input type="text" id="cu2" value="${esc(c.username || "")}"></label>
    <label class="f"><span>Password${id ? ' <span class="faint">(leave empty to keep)</span>' : ""}</span><input type="password" id="cp2"></label></div><div id="cmsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="csave">Save</button></div>`);
  $("#csave", m).onclick = async () => {
    try {
      await call("SaveCredential", { id: id || "", name: $("#cn", m).value.trim(), username: $("#cu2", m).value.trim(), password: $("#cp2", m).value, kind: c.kind || "" });
      closeModal();
      renderSettings();
    } catch (e) { $("#cmsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`; }
  };
}

async function delCred(id) {
  const c = S.creds.find((x) => x.id === id);
  if (!(await askConfirm(`Remove the credentials "${c?.name || ""}"? Switches still using them have to be moved to other credentials first.`, "Remove"))) return;
  try { await call("DeleteCredential", id); renderSettings(); } catch (e) { toast(e.message, true); }
}

// ---------- app updates ----------
function openURL(u) { call("OpenURL", u).catch(() => {}); }

function skippedVersion() {
  try { return localStorage.getItem("sd-skip-version") || ""; } catch { return ""; }
}

function showUpdate(u, force) {
  const bar = $("#updbar");
  if (!bar || !u?.available) return;
  if (!force && skippedVersion() === u.latest) return;
  S.update = u;
  bar.innerHTML = `<div class="updbar"><b>SwitchDesk ${esc(u.latest)} is available.</b><span>You have ${esc(u.current)}.</span><span class="grow"></span>
    <button class="ubtn" data-open-url="${esc(u.url)}">Download update</button>
    <button class="ulink" onclick="hideUpdate()">Later</button></div>`;
  $("#content").classList.add("hasupd");
}

function hideUpdate() {
  try { localStorage.setItem("sd-skip-version", S.update?.latest || ""); } catch {}
  $("#updbar").innerHTML = "";
  $("#content").classList.remove("hasupd");
}

async function checkUpdate(btn) {
  btn?.classList.add("spin");
  const res = $("#updres");
  try {
    const u = await call("CheckForUpdate");
    if (u.available) {
      showUpdate(u, true);
      if (res) res.innerHTML = `Version ${esc(u.latest)} is available.`;
    } else if (res) res.textContent = `You're on the latest version (${u.current}).`;
  } catch (e) { if (res) res.textContent = e.message; }
  btn?.classList.remove("spin");
}

// ---------- boot ----------
// showError puts an error on the page instead of leaving it blank, with the
// details to copy into a GitHub issue.
function showError(where, e) {
  console.error(where, e);
  const msg = `${where}: ${e?.message || e}${e?.stack ? "\n\n" + e.stack : ""}\n\nSwitchDesk ${S.version || ""} · ${navigator.userAgent}`;
  const box = document.createElement("div");
  box.className = "page";
  box.innerHTML = `<div class="panel" style="max-width:820px"><h2>Something went wrong</h2><div class="body">
    <p class="muted" style="margin-top:0">This page couldn't be shown. Please copy the details below into an issue at github.com/jaba0x/SwitchDesk/issues, so it can be fixed.</p>
    <pre class="code mono" style="white-space:pre-wrap;max-height:260px;overflow:auto"></pre>
    <button class="btn" data-copy>Copy details</button> <button class="btn" data-go="#/dashboard">Back to dashboard</button></div></div>`;
  $("pre", box).textContent = msg;
  $("[data-copy]", box).onclick = () => navigator.clipboard?.writeText(msg).then(() => toast("Copied"));
  const main = $("#main");
  main.innerHTML = "";
  main.appendChild(box);
}

window.addEventListener("unhandledrejection", (e) => toast("Error: " + (e.reason?.message || e.reason), true));
window.addEventListener("error", (e) => toast("Error: " + e.message, true));

async function boot() {
  try {
    await start();
  } catch (e) {
    try { renderSide(); } catch {}
    showError("Starting SwitchDesk", e);
  }
}

async function start() {
  // On a slow start (many switches, a busy computer) the backend can take a
  // few seconds to connect; wait up to 30 s and say so.
  for (let i = 0; i < 1000 && !window.go?.main?.App; i++) {
    if (i === 40) $("#main").innerHTML = `<div class="page"><div class="empty"><h3>Starting SwitchDesk…</h3></div></div>`;
    await new Promise((r) => setTimeout(r, 30));
  }
  api = window.go?.main?.App;
  if (!api) {
    $("#main").innerHTML = `<div class="page"><div class="empty"><h3>Can't reach the SwitchDesk backend</h3><p>Open this page through the SwitchDesk app.</p></div></div>`;
    return;
  }
  // the language first, so even the lock screen is in it
  S.settings = await call("GetSettings").catch(() => ({}));
  await I18N.use(S.settings.language || "", api).catch(() => {});
  // optional app password: nothing is loaded before it's entered
  const lock = await call("AppLock").catch(() => null);
  if (lock?.locked) await showLock(lock);
  S.version = await call("Version").catch(() => "");
  const plat = await call("Platform");
  if (plat === "darwin") document.documentElement.style.setProperty("--top", "22px");
  S.settings = await call("GetSettings");
  // never leave the window empty without a word if the first load is slow
  const slow = setTimeout(() => { $("#main").innerHTML = `<div class="page"><div class="empty"><h3>Loading the dashboard…</h3></div></div>`; }, 2000);
  S.dash = await call("GetDashboard").finally(() => clearTimeout(slow));
  window.runtime?.EventsOn?.("changed", () => refreshData().catch(() => {}));
  window.runtime?.EventsOn?.("firmware-download", (p) => { if (FW.dl) { FW.dl = p; drawDl(); } });
  window.runtime?.EventsOn?.("update-available", showUpdate);
  call("LastUpdateCheck").then((u) => u && showUpdate(u)).catch(() => {});
  const notice = await call("StartupNotice").catch(() => "");
  if (notice) toast(notice, true);
  // keep relative times fresh even when nothing changes
  setInterval(() => { if (!S.modalOpen && (S.route[0] || "dashboard") === "dashboard") renderDashboard(); }, 30000);
  setupIdleLock();
  route();
}
boot();
