// manager gui — the page. Data comes from /api/data (written by manager index); buttons go to /api/do.
"use strict";

const TOKEN = document.querySelector('meta[name="manager-token"]').content;
const $ = (s) => document.querySelector(s);
const HOME_AN = "/an/";
const state = { data: null, group: "All", q: "", sort: localGet("sort") || "changed", open: null, mi: 0 };

function localGet(k) { try { return localStorage.getItem("manager." + k); } catch { return null; } }
function localSet(k, v) { try { localStorage.setItem("manager." + k, v); } catch { /* private window */ } }
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
const enc = encodeURIComponent;

// ---- media urls ---------------------------------------------------------------------------
const thumbUrl = (m) => m.thumb ? `/t/${m.thumb}` : `/f?p=${enc(m.thumbSrc || m.src)}`;
const fullUrl = (m) => m.kind === "video"
  ? (m.play ? `/t/${m.play}` : `/f?p=${enc(m.src)}`)
  : (m.big ? `/t/${m.big}` : m.src ? `/f?p=${enc(m.src)}` : `/t/${m.thumb}`);
const previewUrl = (m) => m.preview ? `/t/${m.preview}` : (m.kind === "video" && m.from === "organizer" ? `/f?p=${enc(m.src)}` : null);

// ---- little helpers -----------------------------------------------------------------------
function ago(t) {
  if (!t) return "";
  const s = Date.now() / 1000 - t;
  if (s < 90) return "just now";
  const m = s / 60, h = m / 60, d = h / 24;
  if (m < 60) return `${Math.round(m)} min ago`;
  if (h < 36) return `${Math.round(h)} h ago`;
  if (d < 45) return `${Math.round(d)} days ago`;
  if (d < 600) return `${Math.round(d / 30)} months ago`;
  return `${(d / 365).toFixed(1)} years ago`;
}
function bytes(n) {
  const u = ["B", "KB", "MB", "GB"]; let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(n < 10 && i ? 1 : 0)} ${u[i]}`;
}
const shortPath = (p) => p.includes(HOME_AN) ? p.slice(p.indexOf(HOME_AN) + HOME_AN.length) : p.replace(/^\/home\/[^/]+/, "~");
const groupOf = (p) => (p.trail || "").split(" › ")[0] || "Other";
function code(p) { // the two-letter path, for cards with no picture
  const s = shortPath(p.path).split("/");
  return s.length > 3 ? `${s[0]}/…/${s[s.length - 1]}` : s.join("/");
}

let toastTimer;
function toast(msg, bad = false) {
  const t = $("#toast");
  t.textContent = msg; t.className = "toast" + (bad ? " bad" : ""); t.hidden = false;
  clearTimeout(toastTimer); toastTimer = setTimeout(() => { t.hidden = true; }, 3200);
}

async function post(url, body) {
  const r = await fetch(url, { method: "POST", headers: { "Content-Type": "application/json", "X-Manager-Token": TOKEN }, body: JSON.stringify(body) });
  return r.json();
}

// ---- rendering ----------------------------------------------------------------------------
function cardHtml(p, i) {
  const first = p.media && p.media[0];
  const vids = (p.media || []).filter((m) => m.kind === "video").length;
  const imgs = (p.media || []).length - vids;
  let cover;
  if (first) {
    cover = `<img src="${esc(thumbUrl(first))}" alt="" loading="lazy" decoding="async" width="720" height="450">`;
    if (first.tone) cover = `<div class="tone" style="background:${esc(first.tone)}"></div>` + cover;
  } else {
    cover = `<div class="blank"><span class="code">${esc(code(p))}</span><span class="kind">${esc(p.type)} · no pictures yet</span></div>`;
  }
  const badges = [vids ? `<span class="badge">▶ ${vids} video${vids > 1 ? "s" : ""}</span>` : "", imgs > 1 ? `<span class="badge">${imgs} pictures</span>` : ""].join("");
  const when = p.modified ? ago(p.modified) : "";
  return `<button class="card" data-i="${i}">
    <div class="cover">${cover}<div class="badges">${badges}</div></div>
    <div class="body">
      <h3>${esc(p.name)}</h3>
      ${p.about ? `<p class="about">${esc(p.about)}</p>` : ""}
      <div class="meta"><span class="type">${esc(p.type)}</span>${when ? `<span>${esc(when)}</span>` : ""}<span class="trail" title="${esc(shortPath(p.path))}">${esc(p.trail)}</span></div>
    </div></button>`;
}

function sorted(list) {
  const recent = state.data.recent || [];
  const by = {
    changed: (a, b) => (b.modified || 0) - (a.modified || 0),
    used: (a, b) => recent.indexOf(b.path) - recent.indexOf(a.path) || (b.modified || 0) - (a.modified || 0),
    name: (a, b) => a.name.localeCompare(b.name),
    folder: (a, b) => a.path.localeCompare(b.path),
  }[state.sort];
  return [...list].sort(by);
}

function matches(p) {
  if (state.group !== "All" && groupOf(p) !== state.group) return false;
  if (!state.q) return true;
  const hay = [p.name, p.about, p.what, p.path, shortPath(p.path), p.trail, p.type, (p.langs || []).join(" ")].join(" ").toLowerCase();
  return state.q.toLowerCase().split(/\s+/).every((w) => hay.includes(w));
}

function render() {
  const all = state.data.projects;
  const list = sorted(all.filter(matches));
  let html = "", lastHead = null;
  for (const p of list) {
    if (state.sort === "folder") {
      const head = p.trail || "~";
      if (head !== lastHead) { html += `<h2 class="section-head">${esc(head)}</h2>`; lastHead = head; }
    }
    html += cardHtml(p, all.indexOf(p));
  }
  $("#grid").innerHTML = html;
  $("#empty").hidden = list.length > 0;
}

function renderGroups() {
  const counts = new Map([["All", state.data.projects.length]]);
  for (const p of state.data.projects) counts.set(groupOf(p), (counts.get(groupOf(p)) || 0) + 1);
  $("#groups").innerHTML = [...counts].map(([g, n]) =>
    `<button aria-pressed="${g === state.group}" data-g="${esc(g)}">${esc(g)}<b>${n}</b></button>`).join("");
}

function renderSummary() {
  const d = state.data, n = d.projects.length;
  const withMedia = d.projects.filter((p) => p.media && p.media.length).length;
  $("#summary").textContent = `${n} projects · ${withMedia} with pictures`;
  const gen = d.generated;
  const next = gen ? new Date((gen + 3 * 86400) * 1000) : null;
  $("#foot").innerHTML = gen
    ? `Pictures and facts gathered ${esc(ago(gen))}${d.took ? ` (took ${d.took}s)` : ""} · refreshed every 3 days in the background, next around ${esc(next.toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short" }))} · <button id="foot-refresh">refresh now</button> · names come from <code>~/an/ca/sy/manager/names.tsv</code>`
    : "No pictures gathered yet.";
}

// ---- hover previews (only while the pointer is on a card) --------------------------------
let hoverVideo = null;
function stopHover() { if (hoverVideo) { hoverVideo.remove(); hoverVideo = null; } }
$("#grid").addEventListener("pointerover", (e) => {
  const card = e.target.closest(".card");
  if (!card || (hoverVideo && card.contains(hoverVideo))) return;
  stopHover();
  const p = state.data.projects[+card.dataset.i];
  const m = (p.media || []).find((x) => previewUrl(x));
  if (!m) return;
  const v = document.createElement("video");
  Object.assign(v, { src: previewUrl(m), muted: true, loop: true, playsInline: true, autoplay: true, preload: "auto" });
  v.style.cssText = "position:absolute;inset:0";
  card.querySelector(".cover").appendChild(v);
  hoverVideo = v;
});
$("#grid").addEventListener("pointerleave", stopHover);
$("#grid").addEventListener("pointerout", (e) => { if (hoverVideo && !e.relatedTarget?.closest?.(".card")?.contains(hoverVideo)) stopHover(); });

// ---- detail -------------------------------------------------------------------------------
function show(i) {
  const p = state.data.projects[i];
  state.open = p; state.mi = 0;
  $("#d-trail").textContent = p.trail;
  $("#d-name").textContent = p.name;
  $("#d-about").textContent = p.what || p.about || "";
  $("#d-path").textContent = shortPath(p.path).startsWith("~") ? shortPath(p.path) : "~/an/" + shortPath(p.path);
  // what each folder in the path means
  const parts = shortPath(p.path).replace(/^~\//, "").split("/");
  const names = [...(p.trail ? p.trail.split(" › ") : []), p.name];
  $("#d-crumbs").innerHTML = parts.map((s, k) => `<li><b>${esc(s)}</b>${esc(names[k] && names[k] !== s ? names[k] : "")}</li>`).join("");
  const facts = [
    ["Type", p.type],
    ["Opens in", p.editor],
    ["Changed", p.modified ? `${ago(p.modified)} · ${new Date(p.modified * 1000).toLocaleDateString()}` : ""],
    ["Last commit", p.git ? `${p.git.subject} (${ago(p.git.time)})` : ""],
    ["Languages", (p.langs || []).join(", ")],
    ["Files", p.files ? `${p.files.toLocaleString()} · ${bytes(p.bytes || 0)}` : ""],
    ["Status", p.status],
  ].filter(([, v]) => v);
  $("#d-facts").innerHTML = facts.map(([k, v]) => `<dt>${esc(k)}</dt><dd>${esc(v)}</dd>`).join("");
  $("#d-actions").innerHTML = [
    `<button class="primary" data-a="run">▶ Run</button>`,
    `<button data-a="watch" ${p.reload ? "" : "disabled"} title="rebuild + rerun on every save">⟳ Watch</button>`,
    `<button data-a="build">Build</button>`,
    `<button data-a="open">Open in ${esc(p.editor || "editor")}</button>`,
    `<button data-a="folder">Folder</button>`,
  ].join("");
  $("#d-strip").innerHTML = (p.media || []).length > 1 ? p.media.map((m, k) =>
    `<button data-m="${k}" aria-current="${k === 0}" title="${esc(m.caption)}"><img src="${esc(thumbUrl(m))}" alt="" loading="lazy">${m.kind === "video" ? '<span class="play">▶</span>' : ""}</button>`).join("") : "";
  showMedia(0);
  if (!$("#detail").open) $("#detail").showModal();
}

function showMedia(k) {
  const p = state.open, m = (p.media || [])[k];
  state.mi = k;
  const view = $("#d-view");
  if (!m) {
    view.style.background = "";
    view.innerHTML = `<div class="blank"><span class="code">${esc(code(p))}</span><span class="kind">no pictures or videos found in this project yet<br>add a screenshot (named screenshot…, or in a screenshots/ folder) and it shows up at the next refresh</span></div>`;
    return;
  }
  view.style.background = m.tone ? `radial-gradient(closest-side, ${m.tone}, #0f0d0c)` : "";
  view.innerHTML = m.kind === "video"
    ? `<video src="${esc(fullUrl(m))}" poster="${esc(thumbUrl(m))}" controls autoplay muted loop playsinline preload="metadata"></video>`
    : `<img src="${esc(fullUrl(m))}" alt="${esc(m.caption)}">`;
  if (m.caption) view.insertAdjacentHTML("beforeend", `<span class="caption">${esc(m.caption)}</span>`);
  for (const b of document.querySelectorAll("#d-strip button")) b.setAttribute("aria-current", String(+b.dataset.m === k));
}

$("#d-strip").addEventListener("click", (e) => { const b = e.target.closest("button"); if (b) showMedia(+b.dataset.m); });
$("#d-close").addEventListener("click", () => $("#detail").close());
$("#detail").addEventListener("click", (e) => { if (e.target === $("#detail")) $("#detail").close(); });
$("#detail").addEventListener("close", () => { $("#d-view").innerHTML = ""; state.open = null; });
$("#d-copy").addEventListener("click", async () => {
  try { await navigator.clipboard.writeText(state.open.path); toast("path copied"); } catch { toast("couldn't copy", true); }
});
$("#d-actions").addEventListener("click", async (e) => {
  const b = e.target.closest("button[data-a]");
  if (!b || b.disabled) return;
  const a = b.dataset.a, p = state.open;
  const said = { run: `running ${p.name} in a terminal`, watch: `watching ${p.name} in a terminal`, build: `building ${p.name} in a terminal`,
    open: `opening ${p.name} in ${p.editor}`, folder: "opening the folder" }[a];
  try {
    const r = await post("/api/do", { path: p.path, action: a });
    r.ok ? toast(said) : toast(r.error || "that didn't work", true);
  } catch { toast("manager gui isn't answering", true); }
});

// ---- events -------------------------------------------------------------------------------
$("#grid").addEventListener("click", (e) => { const c = e.target.closest(".card"); if (c) show(+c.dataset.i); });
$("#groups").addEventListener("click", (e) => {
  const b = e.target.closest("button"); if (!b) return;
  state.group = b.dataset.g; renderGroups(); render();
});
$("#q").addEventListener("input", (e) => { state.q = e.target.value.trim(); render(); });
$("#sort").value = state.sort;
$("#sort").addEventListener("change", (e) => { state.sort = e.target.value; localSet("sort", state.sort); render(); });
document.addEventListener("keydown", (e) => {
  if (e.key === "/" && document.activeElement !== $("#q") && !$("#detail").open) { e.preventDefault(); $("#q").focus(); }
  if ($("#detail").open && state.open && (e.key === "ArrowRight" || e.key === "ArrowLeft")) {
    const n = (state.open.media || []).length; if (n < 2) return;
    showMedia((state.mi + (e.key === "ArrowRight" ? 1 : n - 1)) % n);
  }
});
document.addEventListener("click", (e) => { if (e.target.id === "foot-refresh") refresh(); });
$("#refresh").addEventListener("click", (e) => refresh(e.shiftKey));

// ---- refresh + progress -------------------------------------------------------------------
let polling = false;
async function refresh(force = false) {
  await post("/api/refresh", { force });
  toast(force ? "remaking every picture…" : "looking for new pictures and videos…");
  watchProgress();
}
async function watchProgress() {
  if (polling) return; polling = true;
  $("#refresh").classList.add("busy");
  let seen = false;
  for (let tries = 0; tries < 900; tries++) {
    const st = await (await fetch("/api/state")).json().catch(() => ({}));
    if (st.running) {
      seen = true;
      $("#progress").hidden = false;
      $("#bar").style.width = st.total ? `${(100 * st.done / st.total).toFixed(0)}%` : "4%";
      $("#progress-text").textContent = st.total ? `gathering pictures · ${st.done}/${st.total} · ${st.current || ""}` : "finding projects…";
    } else if (seen || tries > 6) {
      break;
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  $("#progress").hidden = true; $("#refresh").classList.remove("busy"); polling = false;
  await load();
}

async function load() {
  const d = await (await fetch("/api/data")).json();
  state.data = d;
  if (!d.projects.length && (d.state || {}).running !== false) { watchProgress(); }
  if (state.group !== "All" && !d.projects.some((p) => groupOf(p) === state.group)) state.group = "All";
  renderGroups(); render(); renderSummary();
  if ((d.state || {}).running) watchProgress();
  openFromHash();
}

// #p=<id> opens that project (handy for links and for checking the page)
function openFromHash() {
  const id = new URLSearchParams(location.hash.slice(1)).get("p");
  const i = id ? state.data.projects.findIndex((p) => p.id === id) : -1;
  if (i >= 0 && state.open?.id !== id) show(i);
}
window.addEventListener("hashchange", openFromHash);

// the server closes a few minutes after the window goes; tell it we're still here
setInterval(() => fetch("/api/alive").catch(() => {}), 20000);
load();
