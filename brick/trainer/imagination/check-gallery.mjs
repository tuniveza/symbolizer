import { spawn } from "node:child_process";
import { writeFileSync } from "node:fs";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
const HERE = new URL(".", import.meta.url).pathname, OUT = process.argv[2] || tmpdir();
const chrome = spawn("chromium", ["--headless=new", "--remote-debugging-port=9336", `--user-data-dir=${mkdtempSync(tmpdir() + "/galprof")}`, "--window-size=1300,1100", "--no-first-run", "about:blank"], { stdio: "ignore" });
const sleep = ms => new Promise(r => setTimeout(r, ms));
let ws, id = 0; const waiting = new Map(); const errors = [];
try {
  let target; for (let i = 0; i < 40 && !target; i++) { await sleep(250); try { target = (await (await fetch("http://127.0.0.1:9336/json")).json()).find(t => t.type === "page"); } catch {} }
  ws = new WebSocket(target.webSocketDebuggerUrl); await new Promise(r => ws.onopen = r);
  ws.onmessage = e => { const m = JSON.parse(e.data); if (m.method === "Runtime.exceptionThrown") errors.push(m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text); if (waiting.has(m.id)) { waiting.get(m.id)(m); waiting.delete(m.id); } };
  const send = (method, params = {}) => new Promise(r => { const n = ++id; waiting.set(n, r); ws.send(JSON.stringify({ id: n, method, params })); });
  const js = async e => (await send("Runtime.evaluate", { expression: e, returnByValue: true })).result.result.value;
  await send("Page.enable"); await send("Runtime.enable");
  await send("Page.navigate", { url: "file://" + HERE + "pages/imagination-archetypes.html" }); await sleep(2500);
  console.log("cards:", await js(`document.querySelectorAll('.card').length`), "count:", await js(`document.querySelector('#count').textContent`));
  writeFileSync(OUT + "/gal1.png", Buffer.from((await send("Page.captureScreenshot")).result.data, "base64"));
  await js(`document.querySelector('.filters button[data-f=yours]').click()`); await sleep(200);
  console.log("yours:", await js(`document.querySelector('#count').textContent`));
  await js(`document.querySelector('.card:not([hidden])').click()`); await sleep(1500);
  console.log("measured:", await js(`[...document.querySelectorAll('#detail .rules')].pop().innerText.replace(/\\n/g,' | ')`));
  writeFileSync(OUT + "/gal2.png", Buffer.from((await send("Page.captureScreenshot")).result.data, "base64"));
  console.log("errors:", errors.length ? errors : "none");
} finally { try { ws && ws.close(); } catch {} chrome.kill(); }
