// Headless check of pages/imagination-neurons.html: archetype picker, archetype table, a data test per archetype.
// node check-page.mjs [folder for screenshots]   (needs chromium)
import { spawn } from "node:child_process";
import { writeFileSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
const HERE = new URL(".", import.meta.url).pathname, OUT = process.argv[2] || tmpdir();
const chrome = spawn("chromium", ["--headless=new", "--remote-debugging-port=9337", `--user-data-dir=${mkdtempSync(tmpdir() + "/inprof")}`, "--window-size=1400,1600", "--no-first-run", "about:blank"], { stdio: "ignore" });
const sleep = ms => new Promise(r => setTimeout(r, ms));
let ws, id = 0; const waiting = new Map(); const errors = [];
try {
  let target; for (let i = 0; i < 40 && !target; i++) { await sleep(250); try { target = (await (await fetch("http://127.0.0.1:9337/json")).json()).find(t => t.type === "page"); } catch {} }
  ws = new WebSocket(target.webSocketDebuggerUrl); await new Promise(r => ws.onopen = r);
  ws.onmessage = e => { const m = JSON.parse(e.data); if (m.method === "Runtime.exceptionThrown") errors.push(m.params.exceptionDetails.exception?.description || m.params.exceptionDetails.text); if (waiting.has(m.id)) { waiting.get(m.id)(m); waiting.delete(m.id); } };
  const send = (method, params = {}) => new Promise(r => { const n = ++id; waiting.set(n, r); ws.send(JSON.stringify({ id: n, method, params })); });
  const js = async e => (await send("Runtime.evaluate", { expression: e, returnByValue: true, awaitPromise: true })).result.result.value;
  const shot = async name => writeFileSync(OUT + "/" + name, Buffer.from((await send("Page.captureScreenshot")).result.data, "base64"));
  await send("Page.enable"); await send("Runtime.enable");
  await send("Page.navigate", { url: "file://" + HERE + "pages/imagination-neurons.html" }); await sleep(2500);
  console.log("archetype options:", await js(`document.querySelector('#brain').options.length`), "| table rows:", await js(`document.querySelectorAll('#archTable tbody tr').length`));
  console.log("note:", await js(`document.querySelector('#archNote').innerText`));
  await js(`(()=>{const s=document.querySelector('#brain'); s.value='The Mirror Oracle'; s.dispatchEvent(new Event('change'));})()`); await sleep(8000);
  console.log("live:", await js(`document.querySelector('#readout').innerText.replace(/\\n/g,' ')`), "|", await js(`document.querySelector('#brainNote').innerText`));
  await shot("page-live.png");
  const waitDone = async () => { for (let i = 0; i < 160; i++) { await sleep(500); if (await js(`!document.querySelector('#runTest').disabled`)) return; } };
  for (const b of ["", "The Mirror Oracle", "The Sensory Mythweaver"]) {
    await js(`(()=>{document.querySelector('#runs').value='1'; document.querySelector('#runBrain').value=${JSON.stringify(b)}; document.querySelector('#runName').value=''; document.querySelector('#runTest').click();})()`);
    await sleep(300); await waitDone();
  }
  console.log(await js(`[...document.querySelectorAll('#compare tr')].slice(0,5).map(r=>r.innerText.replace(/\\s+/g,' ')).join('\\n')`));
  await js(`document.querySelector('#archetypes').scrollIntoView()`); await sleep(300); await shot("page-archetypes.png");
  await send("Emulation.setDeviceMetricsOverride", { width: 390, height: 900, deviceScaleFactor: 1, mobile: true }); await sleep(500);
  await js(`(()=>{const m=document.createElement('meta');m.name='viewport';m.content='width=device-width,initial-scale=1';document.head.appendChild(m);})()`); await sleep(500);
  console.log("phone page scroll width:", await js(`document.documentElement.scrollWidth`), "of", await js(`innerWidth`));
  console.log("errors:", errors.length ? errors : "none");
} finally { try { ws && ws.close(); } catch {} chrome.kill(); }
