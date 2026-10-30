// Collects the interface text into frontend/dist/i18n/en.json, the list of
// strings translators work from. Run it after changing interface text:
//
//   cd tools/i18n && npm install && npm run extract
//
// Strings are keyed by their English text. Parts that change at run time
// (numbers, names) become {0}, {1}… so "5 of 8 online" is "{0} of {1} online".
// Problem titles and details come from internal/core/issues.go, where the
// %d / %s / %v verbs become {0}, {1}… the same way.

import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import * as acorn from "acorn";
import * as walk from "acorn-walk";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const jsFiles = ["frontend/dist/app.js", "frontend/dist/firmware.js", "frontend/dist/map.js"];
// problem titles and details, firmware job states, Instant On notes
const goFiles = ["internal/core/issues.go", "internal/core/upgrade.go", "internal/instanton/snapshot.go", "internal/ionlocal/snapshot.go"];
// text that Go puts together at run time, written here as patterns
const extraFile = join(dirname(fileURLToPath(import.meta.url)), "extra.txt");

// calls whose string arguments are code, not text
const codeCalls = new Set(["call", "$", "$$", "querySelector", "querySelectorAll", "getElementById", "closest", "addEventListener",
  "removeEventListener", "setAttribute", "getAttribute", "removeAttribute", "setProperty", "EventsOn", "createElement", "add", "remove",
  "toggle", "contains", "matches", "startsWith", "endsWith", "includes", "split", "join", "replace", "replaceAll", "indexOf", "test",
  "RegExp", "fetch", "openURL", "toLocaleString", "toLocaleDateString", "toLocaleTimeString", "padStart", "localeCompare", "emit",
  "dbm", "has", "get", "set", "delete", "scrollIntoView", "trim"]);

const keys = new Map(); // key -> first place it was seen
const add = (s, where, text) => {
  s = s.replace(/&nbsp;/g, " ").replace(/&amp;/g, "&").replace(/&#39;/g, "'").replace(/&quot;/g, '"').replace(/&[lg]t;/g, "");
  s = s.replace(/\s+/g, " ").trim();
  if (/^(#|__)/.test(s)) return; // element ids, internal values
  if (/^(use strict|var\(|rgba?\(|[a-z-]+:[^\s]|[A-Za-z]+\(.*\)$)/.test(s)) return; // code and css
  if (/^(show|vlan|write|aaa|interface|ip|radius-server|web-management) [a-z]/.test(s)) return; // switch commands stay as they are
  {
    // number the placeholders from {0} in the order they appear
    let n = 0;
    const map = {};
    s = s.replace(/\{(\d+)\}/g, (_, i) => `{${i in map ? map[i] : (map[i] = n++)}}`);
  }
  if (s.includes('">')) s = s.slice(s.lastIndexOf('">') + 2).trim(); // tail of a tag split by ${}
  if (!/[A-Za-z]{2}/.test(s)) return;
  if (/sans-serif|monospace/.test(s)) return;
  if (/\d+(\.\d+)?px |\b(data-[a-z-]+|onclick|style|title|class)="|^\.[\w-]+\[/.test(s)) return; // css and markup
  if (/^[{}0-9\s.,:;·–—()%/+×-]*$/.test(s)) return;
  if (/^(https?:|#\/|mailto:)/.test(s)) return;
  if (/^[a-z][a-z0-9-]*$/.test(s) && !text && !/^(now|on|off|up|down|collecting|disabled|online|offline|none|never|tagged|untagged|yes|no|or|and|of)$/.test(s)) return; // class names, ids, keys
  if (/^[A-Za-z_$][\w$]*(\.[\w$]+)+$/.test(s)) return; // a.b.c
  if (/[a-z][A-Z]/.test(s) && !/\s/.test(s)) return; // camelCase identifiers
  if (/^[\w-]+\s*[:=]/.test(s) && /;|px|var\(|rgba?\(/.test(s)) return; // inline css
  if (/^[.#]?[\w-]+([ >.#][\w-]+)*$/.test(s) && !/\s/.test(s) && /[.#]/.test(s)) return; // selectors
  if (/^\s*<|\/>|=>|&&|\|\||\bfunction\b/.test(s)) return;
  if (!keys.has(s)) keys.set(s, where);
};

// Splits a piece of markup into its text and its title / placeholder / aria-label values.
function fromMarkup(s, where) {
  for (const m of s.matchAll(/\b(?:title|placeholder|aria-label|alt)="([^"\u0002]*)"/g)) add(m[1], where);
  // a string can start inside a tag (after a ${} in an attribute) or end inside one
  if (/^[^<>]*["'][^<>]*>/.test(s) || /^[^<>"']*>/.test(s)) s = s.replace(/^[^<>]*>/, "");
  s = s.replace(/<[^>]*$/, "");
  const hadTags = /<[a-z][^>]*>/i.test(s); // pieces between tags are page text, even single lowercase words
  const text = s.replace(/<[^>]*>/g, "\u0000");
  for (const piece of text.split(/[\u0000\u0002]/)) {
    // renumber the placeholders inside this piece from {0}
    let n = 0;
    const map = {};
    add(piece.replace(/\{(\d+)\}/g, (_, i) => `{${i in map ? map[i] : (map[i] = n++)}}`).replace(/&nbsp;/g, " ").replace(/&amp;/g, "&").replace(/&#39;/g, "'").replace(/&quot;/g, '"'), where, hadTags);
  }
}

for (const f of jsFiles) {
  const src = readFileSync(join(root, f), "utf8");
  const ast = acorn.parse(src, { ecmaVersion: "latest", locations: true });
  walk.fullAncestor(ast, (node, _state, ancestors) => {
    const parent = ancestors[ancestors.length - 2];
    const where = `${f}:${node.loc.start.line}`;
    if (node.type === "Literal" && typeof node.value === "string") {
      if (!parent) return;
      if (parent.type === "Property" && parent.key === node) return;
      if (parent.type === "MemberExpression" && parent.property === node) return;
      if (parent.type === "BinaryExpression" && ["===", "!==", "==", "!=", "in"].includes(parent.operator)) return;
      if (parent.type === "SwitchCase") return;
      if (parent.type === "CallExpression" && parent.arguments[0] === node) {
        const c = parent.callee;
        const name = c.type === "Identifier" ? c.name : c.type === "MemberExpression" && !c.computed ? c.property.name : "";
        if (codeCalls.has(name)) return;
      }
      if (parent.type === "CallExpression" && parent.arguments.includes(node)) {
        const c = parent.callee;
        const name = c.type === "Identifier" ? c.name : c.type === "MemberExpression" && !c.computed ? c.property.name : "";
        if (["call", "setProperty", "setAttribute", "addEventListener", "replace", "replaceAll", "split", "join", "padStart"].includes(name)) return;
      }
      fromMarkup(node.value, where);
    } else if (node.type === "TemplateLiteral") {
      if (parent?.type === "TaggedTemplateExpression") return;
      let s = "";
      node.quasis.forEach((q, i) => {
        s += q.value.cooked ?? q.value.raw;
        if (i < node.expressions.length) {
          const e = node.expressions[i];
          // a nested template or a markup-producing call stands for markup, not text
          s += e.type === "TemplateLiteral" || e.type === "ConditionalExpression" || e.type === "LogicalExpression" || (e.type === "CallExpression" && !["esc", "plural", "String", "fmtBytes", "ago"].includes(e.callee.name)) ? "\u0001" : `{${i}}`;
        }
      });
      // a nested template splits the text the same way markup does
      fromMarkup(s.replace(/\u0001/g, "\u0002"), where);
    }
  });
}

// Problem texts from the Go rules: string literals with fmt verbs.
for (const f of goFiles) {
  const src = readFileSync(join(root, f), "utf8");
  src.split("\n").forEach((line, i) => {
    if (/^\s*\/\//.test(line)) return;
    for (const m of line.matchAll(/"((?:[^"\\]|\\.)*)"/g)) {
      let n = 0;
      let s = m[1].replace(/\\"/g, '"').replace(/%(\.\d+)?[dsvf]/g, () => `{${n++}}`);
      if (s.includes("%%")) s = s.replace(/%%/g, "%");
      if (/^Jan 2|15:04/.test(s)) continue; // Go time layouts
      // details are often glued together from pieces; the page translates
      // them sentence by sentence, so the sentences are keys too
      s = s.replace(/^[.,;:]\s*/, "").trim();
      const parts = [s, ...s.split(/(?<=[.!?])\s+/)];
      for (const p of new Set(parts)) {
        if (/\s/.test(p) && /^[A-Z0-9{]/.test(p)) add(p, `${f}:${i + 1}`);
        else if (/^[A-Z][a-z]+( [A-Za-z0-9-]+)*$/.test(p)) add(p, `${f}:${i + 1}`); // categories such as "Spanning tree"
      }
    }
  });
}

if (existsSync(extraFile)) {
  for (const line of readFileSync(extraFile, "utf8").split("\n")) if (line.trim() && !line.startsWith("#")) add(line, "tools/i18n/extra.txt", true);
}

const out = join(root, "frontend/dist/i18n/en.json");
const old = existsSync(out) ? JSON.parse(readFileSync(out, "utf8")) : {};
const sorted = [...keys.keys()].sort((a, b) => a.localeCompare(b, "en"));
const doc = {
  language: "English",
  code: "en",
  note: "Template for translators. Copy this file to <code>.json, fill in each value, and see docs/translating.md. Keys are the English text; {0}, {1}… stand for numbers and names and must stay in the translation.",
  strings: Object.fromEntries(sorted.map((k) => [k, ""])),
};
writeFileSync(out, JSON.stringify(doc, null, 2) + "\n");
const before = Object.keys(old.strings || {}).length;
console.log(`${sorted.length} strings (${sorted.length - before >= 0 ? "+" : ""}${sorted.length - before}) written to frontend/dist/i18n/en.json`);
