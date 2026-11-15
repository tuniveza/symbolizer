// results.js's scale test: a vocabulary-like network (~5 lines a word, a band that grows with size).
// node scale.js N STEPS -> line 1: ms per step; line 2: JSON.stringify(net.save()) after the steps.
const S = require("../../sim.js");
const n = +process.argv[2], k = +process.argv[3];
const r = S.rng(9), neurons = [], links = [];
for (let i = 0; i < n; i++) neurons.push({ label: "n" + i, omega: 2 + r() * Math.max(4, n / 25), x: r(), y: r() });
for (let i = 0; i < n; i++) for (let q = 0; q < 5; q++) { const j = Math.floor(r() * n); if (j !== i) links.push(["n" + i, "n" + j, 0.4]); }
const net = new S.Network({ neurons, links, sentences: [] }, 5);
const t0 = process.hrtime.bigint();
for (let i = 0; i < k; i++) net.step();
const ms = Number(process.hrtime.bigint() - t0) / 1e6 / k;
process.stdout.write(ms + "\n" + JSON.stringify(net.save()) + "\n");
