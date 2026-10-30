"use strict";
/* Firmware: library, available updates, update jobs. Uses helpers from app.js. */


const FW = { images: [], jobs: [], dl: null, sel: new Set(), openLog: new Set() };

const shortVer = (v) => (v || "").replace(/^[A-Z]{2}\./, "");
const mb = (n) => (n >= 1 << 20 ? (n / (1 << 20)).toFixed(n > 100 << 20 ? 0 : 1) + " MB" : Math.max(1, Math.round(n / 1024)) + " KB");

// an update can be started when something newer exists and nothing is running
function canUpdate(v) {
  const j = v.firmware?.job;
  return !v.disabled && v.firmware?.available && !(j && (j.state === "running" || j.state === "queued"));
}

// small status cell used in the switch list and on the switch page
function fwCell(v) {
  const f = v.firmware || {};
  const j = f.job;
  if (j && (j.state === "running" || j.state === "queued")) {
    const pct = j.state === "running" && j.progress > 0 && j.progress < 1 ? ` ${Math.round(j.progress * 100)}%` : "";
    return `<span class="fwchip busy" title="${esc(j.step)}">${j.state === "queued" ? "queued" : "updating" + pct}</span>`;
  }
  if (f.available) {
    return `<span class="fwchip ${f.staged ? "staged" : "avail"}" title="${f.staged ? `${f.available} is in the spare bank and boots on the next restart` : `${f.available} is in your library`}">${f.staged ? "staged " : "↑ "}${esc(shortVer(f.available))}</span>`;
  }
  if (j && j.state === "failed") return `<span class="fwchip failed" title="${esc(j.error)}">update failed</span>`;
  return "";
}

// ---------- page ----------
async function renderFirmware(soft) {
  try {
    [FW.images, FW.jobs] = await Promise.all([call("FirmwareImages"), call("UpgradeJobs")]);
  } catch (e) {
    $("#main").innerHTML = `<div class="page"><div class="head"><h1>Firmware</h1></div><div class="panel"><div class="empty">${esc(e.message)}</div></div></div>`;
    return;
  }
  const set = S.settings || (S.settings = await call("GetSettings"));
  if (soft && $("#fw-updates")) {
    $("#fw-updates").innerHTML = updatesPanel();
    $("#fw-jobs").innerHTML = jobsPanel();
    $("#fw-lib").innerHTML = libraryTable();
    return;
  }
  const mode = { off: "Off", stage: "Copy new firmware to the spare bank automatically", install: `Install automatically between ${set.windowStart} and ${set.windowEnd}` }[set.firmwareAuto || "off"];
  $("#main").innerHTML = `<div class="page">
    <div class="head"><h1>Firmware</h1><span class="grow"></span>
      <span class="muted" style="font-size:13px">Automatic updates: <a data-go="#/settings">${esc(mode)}</a></span></div>
    <div id="fw-updates">${updatesPanel()}</div>
    <div id="fw-jobs">${jobsPanel()}</div>
    <section class="panel"><h2>Library<span class="grow"></span>
        <button class="btn small" onclick="call('OpenFirmwareFolder')">Open folder</button>
        <button class="btn small primary" onclick="addFirmwareFiles()">${ICON.plus}Add files</button></h2>
      <div class="body" style="padding-bottom:6px">
        <form class="row" style="align-items:center" onsubmit="downloadFirmware(event)">
          <input type="text" id="fw-url" placeholder="Paste a download link for a .swi file, e.g. from the HPE Networking Support Portal" autocomplete="off" spellcheck="false">
          <span style="flex:none;display:flex;gap:6px"><button class="btn" id="fw-dl">Download</button></span></form>
        <div id="fw-dlstate" style="margin-top:8px"></div>
        <p class="faint" style="font-size:12.5px;margin:8px 0 6px">HPE doesn't offer firmware downloads without signing in, so images come from you: add .swi files you downloaded, or paste a portal download link while it's still valid. Each switch is offered the newest image made for its platform.</p>
      </div>
      <div id="fw-lib">${libraryTable()}</div></section></div>`;
}

function updatesPanel() {
  const vs = (S.dash?.switches || []).filter((v) => !v.disabled && v.firmware?.supported);
  const avail = vs.filter((v) => v.firmware.available);
  for (const id of FW.sel) if (!avail.find((v) => v.id === id)) FW.sel.delete(id);
  const upToDate = vs.filter((v) => !v.firmware.available && v.firmware.running).length;
  if (!avail.length) {
    return `<section class="panel" style="margin-bottom:18px"><h2>Updates</h2><div class="empty" style="padding:22px"><h3>${vs.length ? "Every switch runs the newest firmware in your library" : "No switches read yet"}</h3>
      <p>${vs.length ? `${upToDate} switch${upToDate === 1 ? "" : "es"} checked. Add newer images to the library below to see updates here.` : "Firmware shows up once switches have been collected."}</p></div></section>`;
  }
  const all = avail.every((v) => FW.sel.has(v.id));
  return `<section class="panel" style="margin-bottom:18px"><h2>Updates available<span class="sub muted" style="font-weight:400">${avail.length}</span><span class="grow"></span>
      ${FW.sel.size ? `<button class="btn small primary" onclick="openUpgradeDialog([...FW.sel])">${ICON.up}Update ${FW.sel.size} selected</button>` : `<span class="faint" style="font-weight:400;font-size:13px">Tick switches to update them</span>`}</h2>
    <div style="overflow-x:auto"><table class="swlist fwlist"><thead><tr><th class="cb nogo"><input type="checkbox" ${all ? "checked" : ""} onchange="fwSelAll(this.checked)"></th><th>Switch</th><th>Model</th><th>Running</th><th>Available</th><th>Primary</th><th>Secondary</th><th>Status</th></tr></thead><tbody>
    ${avail.map((v) => {
      const f = v.firmware, b = f.banks || {};
      const mark = (bank) => `${esc(b[bank] || "–")}${b.running === bank ? ' <span class="tag">running</span>' : ""}${b.default === bank && b.running !== bank ? ' <span class="tag">next boot</span>' : ""}`;
      return `<tr class="click${FW.sel.has(v.id) ? " picked" : ""}" data-go="#/switch/${v.id}"><td class="cb nogo"><input type="checkbox" ${FW.sel.has(v.id) ? "checked" : ""} onchange="fwSel('${v.id}', this.checked)"></td>
        <td title="${esc(v.host)}">${statusDot(v)} ${esc(v.name || v.summary?.hostname || v.host)}</td><td>${esc(v.summary?.model || "")}</td>
        <td class="mono">${esc(f.running)}</td><td class="mono ok">${esc(f.available)}</td><td class="mono">${mark("primary")}</td><td class="mono">${mark("secondary")}</td><td>${fwCell(v)}</td></tr>`;
    }).join("")}</tbody></table></div></section>`;
}

function fwSel(id, on) { on ? FW.sel.add(id) : FW.sel.delete(id); $("#fw-updates").innerHTML = updatesPanel(); }
function fwSelAll(on) {
  for (const v of S.dash.switches.filter((v) => !v.disabled && v.firmware?.available)) on ? FW.sel.add(v.id) : FW.sel.delete(v.id);
  $("#fw-updates").innerHTML = updatesPanel();
}

function jobsPanel() {
  if (!FW.jobs.length) return "";
  const label = { queued: ["Queued", "info"], running: ["Running", "info"], staged: ["Staged", "warning"], done: ["Done", "ok"], failed: ["Failed", "critical"], cancelled: ["Cancelled", ""] };
  return `<section class="panel" style="margin-bottom:18px"><h2>Update jobs<span class="grow"></span>
      ${FW.jobs.some((j) => !["queued", "running"].includes(j.state)) ? `<button class="btn small" onclick="call('ClearFinishedUpgrades').then(()=>renderFirmware(true))">Clear finished</button>` : ""}</h2>
    ${FW.jobs.map((j) => {
      const [txt, cls] = label[j.state] || [j.state, ""];
      const pct = j.state === "running" && j.progress > 0 && j.progress < 1 ? Math.round(j.progress * 100) : null;
      const open = FW.openLog.has(j.id);
      return `<div class="job">
        <div class="jobline"><span class="sev ${cls === "ok" ? "" : cls} ${cls === "ok" ? "okchip" : ""}">${txt}</span>
          <b>${esc(j.switchName)}</b><span class="mono muted">${esc(j.from)} → ${esc(j.to)}</span>
          <span class="muted">${j.reboot ? "with reboot" : "copy only"}${j.auto ? ", automatic" : ""}</span><span class="grow"></span>
          ${j.state === "queued" ? `<button class="btn small" onclick="cancelUpgrade('${j.id}')">Cancel</button>` : ""}
          <a onclick="toggleLog('${j.id}')" style="font-size:13px">${open ? "Hide log" : "Log"}</a></div>
        <div class="jobstep ${j.state === "failed" ? "bad" : "muted"}">${esc(j.state === "failed" ? j.error : j.step)}</div>
        ${pct !== null ? `<div class="bar" style="margin-top:6px;max-width:420px"><i style="width:${pct}%;background:var(--info)"></i></div>` : ""}
        ${open ? `<pre class="code mono" style="margin-top:8px;max-height:220px">${esc((j.log || []).join("\n") || "No entries yet.")}</pre>` : ""}</div>`;
    }).join("")}</section>`;
}

function toggleLog(id) { FW.openLog.has(id) ? FW.openLog.delete(id) : FW.openLog.add(id); $("#fw-jobs").innerHTML = jobsPanel(); }

async function cancelUpgrade(id) {
  try { await call("CancelUpgrade", id); renderFirmware(true); } catch (e) { toast(e.message, true); }
}

function libraryTable() {
  if (!FW.images.length) return `<div class="empty" style="padding:22px">No images yet.</div>`;
  const used = {};
  for (const v of S.dash?.switches || []) if (v.firmware?.running) used[v.firmware.running.slice(0, 2)] = (used[v.firmware.running.slice(0, 2)] || 0) + 1;
  return `<table><thead><tr><th>File</th><th>Version</th><th>For</th><th class="num">Size</th><th>Added</th><th>From</th><th></th></tr></thead><tbody>
    ${FW.images.map((i) => `<tr><td class="mono">${esc(i.file)}</td><td class="mono">${esc(i.version.text)}</td>
      <td class="muted">${used[i.version.branch] ? `${used[i.version.branch]} of your switches` : `<span class="faint">none of your switches</span>`}</td>
      <td class="num">${mb(i.size)}</td><td class="faint">${ago(i.addedAt)}</td><td class="faint">${esc(i.source === "file" ? "added file" : i.source)}</td>
      <td style="text-align:right"><button class="ib danger" title="Delete" data-fw-delete="${esc(i.file)}">${ICON.trash}</button></td></tr>`).join("")}
    </tbody></table>`;
}

async function addFirmwareFiles() {
  try {
    const added = await call("AddFirmwareFiles");
    if (added?.length) toast(`Added ${added.map((i) => i.version.text).join(", ")}`);
  } catch (e) { toast(e.message, true); }
  S.dash = await call("GetDashboard");
  renderFirmware(true);
}

async function downloadFirmware(e) {
  e.preventDefault();
  const url = $("#fw-url").value.trim();
  if (!url) return;
  const btn = $("#fw-dl");
  if (FW.dl) { call("CancelDownload"); return; }
  FW.dl = { done: 0, total: -1 };
  btn.textContent = "Cancel";
  drawDl();
  try {
    const img = await call("DownloadFirmware", url);
    toast(`Downloaded ${img.version.text}`);
    $("#fw-url").value = "";
  } catch (err) {
    if (err.message !== "cancelled") toast(err.message, true);
  }
  FW.dl = null;
  if ($("#fw-dl")) $("#fw-dl").textContent = "Download";
  drawDl();
  S.dash = await call("GetDashboard");
  renderFirmware(true);
}

function drawDl() {
  const el = $("#fw-dlstate");
  if (!el) return;
  const d = FW.dl;
  if (!d) { el.innerHTML = ""; return; }
  const pct = d.total > 0 ? Math.round((d.done / d.total) * 100) : null;
  el.innerHTML = `<div class="muted" style="font-size:13px;margin-bottom:4px">Downloading… ${mb(d.done)}${d.total > 0 ? ` of ${mb(d.total)}` : ""}</div>
    <div class="bar" style="max-width:420px"><i style="width:${pct ?? 100}%;background:var(--info);${pct === null ? "opacity:.4" : ""}"></i></div>`;
}


// file names go through a data attribute, never into inline JavaScript
document.addEventListener("click", (e) => {
  const b = e.target.closest("[data-fw-delete]");
  if (b) deleteFirmware(b.dataset.fwDelete);
});

async function deleteFirmware(file) {
  if (!(await askConfirm(`Delete ${file} from the library?`))) return;
  try { await call("DeleteFirmware", file); } catch (e) { toast(e.message, true); }
  S.dash = await call("GetDashboard");
  renderFirmware(true);
}

// ---------- update dialog ----------
function openUpgradeDialog(ids) {
  const vs = ids.map((id) => S.dash.switches.find((v) => v.id === id)).filter(Boolean);
  const ok = vs.filter((v) => v.firmware?.available && !v.disabled);
  const skipped = vs.length - ok.length;
  if (!ok.length) { toast("None of these switches has a newer image in the library", true); return; }
  const m = modal(`<h2>Update firmware</h2><div class="body">
    <table style="margin-bottom:14px"><tbody>${ok.map((v) => `<tr><td>${esc(v.name || v.summary?.hostname || v.host)}</td><td class="mono muted">${esc(v.firmware.running)}</td><td class="mono ok">→ ${esc(v.firmware.available)}</td></tr>`).join("")}</tbody></table>
    ${skipped ? `<p class="faint" style="font-size:13px;margin-top:-6px">${skipped} selected switch${skipped === 1 ? " has" : "es have"} nothing newer and will be skipped.</p>` : ""}
    <p class="muted" style="font-size:13px;margin-top:0">The image goes into each switch's spare bank. The running version stays in the other bank, so you can boot back to it.</p>
    <label class="check" style="margin-bottom:8px"><input type="radio" name="rb" value="1" checked> Copy, then save the config and reboot <span class="faint">(one switch at a time)</span></label>
    <label class="check"><input type="radio" name="rb" value="0"> Copy only; the new version runs after the next reboot</label>
    <p class="faint" style="font-size:12.5px;margin:14px 0 0">SwitchDesk uploads the image to each switch (SSH on AOS-S, HTTPS on AOS-CX), so keep it open until the jobs finish. TFTP is only used if a switch refuses the upload.</p>
    <div id="umsg"></div></div>
    <div class="foot"><span class="grow"></span><button class="btn" data-close>Cancel</button><button class="btn primary" id="ugo">${ICON.up}Start update</button></div>`, true);
  $("#ugo", m).onclick = async () => {
    const reboot = $("input[name=rb]:checked", m).value === "1";
    try {
      const jobs = await call("StartUpgrade", ok.map((v) => v.id), "", reboot);
      closeModal();
      FW.sel.clear();
      toast(`Started ${jobs.length} update${jobs.length === 1 ? "" : "s"}`);
      location.hash = "#/firmware";
    } catch (e) { $("#umsg", m).innerHTML = `<div class="msg err">${esc(e.message)}</div>`; }
  };
}

// firmware card on the switch page
function firmwareCard(d) {
  const f = d.firmware || {}, b = f.banks || {};
  if (!f.supported) return `<section class="panel"><h2>Firmware</h2><div class="body muted" style="font-size:13px">${d.web ? "Instant On firmware is updated from the switch's web page (Maintenance → Firmware)." : d.summary?.family === "Instant On" ? "Instant On firmware is updated from the Instant On app or portal." : "Firmware details appear once the switch has been read."}</div></section>`;
  const row = (bank) => `<dt>${bank[0].toUpperCase() + bank.slice(1)}</dt><dd class="mono">${esc(b[bank] || "–")}${b.running === bank ? ' <span class="tag">running</span>' : ""}${b.default === bank && b.running !== bank ? ' <span class="tag">next boot</span>' : ""}</dd>`;
  const j = f.job;
  return `<section class="panel"><h2>Firmware<span class="grow"></span>${canUpdate(d) ? `<button class="btn small primary" onclick="openUpgradeDialog(['${d.id}'])">${ICON.up}${f.staged ? "Install now" : "Update"}</button>` : ""}</h2><div class="body"><dl class="kv">
    <dt>Running</dt><dd class="mono">${esc(f.running || "?")}</dd>${b.known ? row("primary") + row("secondary") : ""}
    <dt>Available</dt><dd>${f.available ? `<span class="mono ok">${esc(f.available)}</span>${f.staged ? ' <span class="tag">staged</span>' : ""}` : `<span class="faint">Up to date with your library</span>`}</dd>
    ${j ? `<dt>Last update</dt><dd>${fwCell(d) || esc(j.state)} <span class="faint">${esc(j.state === "failed" ? j.error : j.step)}</span> <a data-go="#/firmware" style="font-size:13px">Details</a></dd>` : ""}
  </dl></div></section>`;
}
