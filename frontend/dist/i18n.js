// Interface translation. The pages are written in English; this file swaps
// English text for the chosen language as the page is drawn, so the rest of
// the code doesn't need to know about languages.
//
// A translation is a JSON file keyed by the English text (see i18n/en.json
// and docs/translating.md). {0}, {1}… stand for parts that change, such as
// numbers and switch names: "{0} of {1} online" → "{0} von {1} online".
// Anything without a translation stays in English.

const I18N = (() => {
  const base = new URL(".", document.currentScript?.src || location.href); // the folder this file is in
  let exact = new Map();
  let patterns = [];
  let cache = new Map();
  let code = "en";
  let observer = null;
  const misses = new Set(); // English text seen without a translation, for translators: I18N.missing()
  let known = new Set(); // every key and translation, so they aren't reported as missing

  // text inside these is switch data (configs, command output, addresses), never translated
  const SKIP = "pre, code, textarea, script, style, .mono, [data-notr]";
  const ATTRS = ["title", "placeholder", "aria-label"];

  function load(strings) {
    exact = new Map();
    patterns = [];
    cache = new Map();
    known = new Set();
    for (const [en, tr] of Object.entries(strings || {})) {
      known.add(en);
      if (tr) known.add(tr);
      if (typeof tr !== "string" || !tr.trim() || tr === en) continue;
      if (!/\{\d+\}/.test(en)) { exact.set(en, tr); continue; }
      const order = [];
      const src = en.split(/(\{\d+\})/).map((part) => {
        const m = /^\{(\d+)\}$/.exec(part);
        if (m) { order.push(+m[1]); return "(.*?)"; }
        return part.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      }).join("");
      patterns.push({ re: new RegExp("^" + src + "$", "s"), order, tr, weight: en.replace(/\{\d+\}/g, "").length });
    }
    // the most specific patterns first
    patterns.sort((a, b) => b.weight - a.weight);
  }

  function matchPattern(t) {
    for (const p of patterns) {
      const m = p.re.exec(t);
      if (!m) continue;
      const vals = {};
      p.order.forEach((n, i) => { vals[n] = m[i + 1]; });
      return p.tr.replace(/\{(\d+)\}/g, (_, n) => vals[n] ?? "");
    }
    return null;
  }

  function tr(text) {
    if (!text || (!exact.size && !patterns.length)) return null;
    const t = text.replace(/\s+/g, " ").trim();
    if (!t || !/[A-Za-z]/.test(t)) return null;
    let out = cache.get(t);
    if (out === undefined) {
      out = exact.get(t) ?? matchPattern(t);
      // a text glued together from several sentences: translate each one
      if (out === null && /[.!?]\s+\S/.test(t)) {
        const parts = t.split(/(?<=[.!?])\s+/);
        let hit = false;
        const done = parts.map((p) => {
          const v = exact.get(p) ?? matchPattern(p);
          if (v !== null && v !== undefined) { hit = true; return v; }
          return p;
        });
        if (hit) out = done.join(" ");
      }
      // a label with a count after it: "Critical 5", "Security (2)"
      if (out === null) {
        const m = /^(.*?[A-Za-z].*?)(\s+\(?\d+\)?)$/.exec(t);
        const head = m && (exact.get(m[1]) ?? matchPattern(m[1]));
        if (head) out = head + m[2];
      }
      if (out === null && misses.size < 5000 && !known.has(t)) misses.add(t);
      if (cache.size > 20000) cache.clear();
      cache.set(t, out);
    }
    if (out === null) return null;
    // keep the spacing around the text, which the layout relies on
    const lead = /^\s*/.exec(text)[0], trail = /\s*$/.exec(text)[0];
    return lead + out + trail;
  }

  function translateNode(root) {
    if (code === "en") return;
    if (root.nodeType === Node.TEXT_NODE) {
      if (root.parentElement && !root.parentElement.closest(SKIP)) {
        const v = tr(root.nodeValue);
        if (v !== null && v !== root.nodeValue) root.nodeValue = v;
      }
      return;
    }
    if (root.nodeType !== Node.ELEMENT_NODE || root.closest(SKIP)) return;
    const els = [root, ...root.querySelectorAll("[title],[placeholder],[aria-label]")];
    for (const el of els) {
      for (const a of ATTRS) {
        const v = el.getAttribute?.(a);
        if (v) { const t = tr(v); if (t !== null && t !== v) el.setAttribute(a, t); }
      }
    }
    const w = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: (n) => (n.parentElement?.closest(SKIP) ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
    });
    for (let n = w.nextNode(); n; n = w.nextNode()) {
      const v = tr(n.nodeValue);
      if (v !== null && v !== n.nodeValue) n.nodeValue = v;
    }
  }

  function watch() {
    observer?.disconnect();
    if (code === "en") return;
    observer = new MutationObserver((records) => {
      observer.disconnect(); // our own changes shouldn't come back as new work
      for (const r of records) {
        if (r.type === "childList") r.addedNodes.forEach(translateNode);
        else if (r.type === "characterData") translateNode(r.target);
        else if (r.type === "attributes") translateNode(r.target);
      }
      observer.observe(document.body, opts);
    });
    const opts = { subtree: true, childList: true, characterData: true, attributes: true, attributeFilter: ATTRS };
    translateNode(document.body);
    observer.observe(document.body, opts);
  }

  // The languages that come with SwitchDesk, plus any files in the data
  // folder's "translations" folder.
  async function languages(api) {
    let list = [];
    try { list = await (await fetch(new URL("i18n/languages.json", base))).json(); } catch {}
    const own = (await api?.UserTranslations?.().catch(() => [])) || [];
    for (const t of own) {
      const i = list.findIndex((l) => l.code === t.code);
      const entry = { code: t.code, name: t.language, own: true };
      if (i >= 0) list[i] = { ...list[i], ...entry }; else list.push(entry);
    }
    return list;
  }

  // "" picks the computer's language when SwitchDesk has it.
  async function use(want, api) {
    const list = await languages(api);
    let pick = want;
    if (!pick) {
      const sys = (navigator.languages || [navigator.language || "en"]).map((l) => l.toLowerCase());
      pick = sys.map((l) => list.find((x) => x.code === l) || list.find((x) => x.code === l.split("-")[0])).find(Boolean)?.code || "en";
    }
    let strings = null;
    if (pick !== "en") {
      const own = ((await api?.UserTranslations?.().catch(() => [])) || []).find((t) => t.code === pick);
      if (own) strings = own.strings;
      else {
        try { strings = (await (await fetch(new URL(`i18n/${pick}.json`, base))).json()).strings; } catch { strings = null; }
      }
      if (!strings) pick = "en";
    }
    code = pick;
    load(strings);
    document.documentElement.lang = code;
    watch();
    return code;
  }

  return { use, languages, tr: (s) => tr(s) ?? s, missing: () => [...misses].sort(), get code() { return code; } };
})();
