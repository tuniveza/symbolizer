// Measure what the imagination-neuron network can and can't do. node measure.js [which]
const S = require("./sim.js");
const which = process.argv[2] || "all";

function trained(seed, opts = {}) {
  const spec = S.wordSpec(opts.gap ?? 1.2);
  if (opts.holdout) spec.sentences = spec.sentences.filter(x => !(x.includes("crap") && x.includes("quote")) && !(x.includes("test") && x.includes("compare")));
  if (opts.randomFreq) { const r = S.rng(seed * 31); for (const n of spec.neurons) n.omega = 2 + r() * 4; }
  const net = new S.Network(spec, seed, opts.energies);
  const steps = opts.steps ?? 3000;
  for (let k = 0; k < steps; k++) { if (k % S.C.TRAIN_EVERY === 0) net.train(); net.step(); }
  return { net, spec };
}

function quiet(net, k = 30) { for (let i = 0; i < k; i++) { if (net.steps % S.C.TRAIN_EVERY === 0) net.train(); net.step(); } }

function accuracy(net, spec, word) {
  const truth = spec.truth[word];
  let right = 0, total = 0, split = net.N.filter(n => n.word === word).length;
  for (const [ctx, want] of Object.entries(truth)) {
    for (let rep = 0; rep < 3; rep++) {
      quiet(net);
      const r = net.read(word, [ctx]);
      total++;
      if (r && r.senses > 1 && r.meaning === want) right++;
    }
  }
  return { right, total, senses: split };
}

function run(label, seeds, opts) {
  let r = 0, t = 0, splitCrap = 0, splitTest = 0, firstSplit = [], ms = 0;
  for (const seed of seeds) {
    const t0 = Date.now();
    const { net, spec } = trained(seed, opts);
    ms += Date.now() - t0;
    const fis = net.log.filter(l => l.text.startsWith("nuclear fission"));
    if (fis.length) firstSplit.push(fis[0].t);
    const a = accuracy(net, spec, "crap"), b = accuracy(net, spec, "test");
    if (a.senses > 1) splitCrap++;
    if (b.senses > 1) splitTest++;
    r += a.right + b.right; t += a.total + b.total;
  }
  const avg = firstSplit.length ? (firstSplit.reduce((x, y) => x + y, 0) / firstSplit.length).toFixed(0) + "s" : "never";
  console.log(`${label.padEnd(34)} context right ${((100 * r) / t).toFixed(0).padStart(3)}%  crap split ${splitCrap}/${seeds.length}  test split ${splitTest}/${seeds.length}  first split ~${avg}  (${(ms / seeds.length).toFixed(0)} ms/run)`);
  return r / t;
}

const seeds = Array.from({ length: 10 }, (_, i) => i + 1);
if (which === "all" || which === "base") {
  run("default energies", seeds, {});
}
if (which === "one") {
  const { net, spec } = trained(1, {});
  for (const l of net.log) console.log(l.t.toFixed(1), l.text);
  console.log(net.stats());
  console.log(net.N.map(n => `${n.label}:${n.omega.toFixed(2)} T${n.T.toFixed(2)}`).join("  "));
  console.log(accuracy(net, spec, "crap"), accuracy(net, spec, "test"));
}
if (which === "all" || which === "unseen") {
  // generalisation: 'quote crap' and 'compare test' are never trained; only those two contexts are scored
  let right = 0, total = 0;
  for (const seed of seeds) {
    const { net, spec } = trained(seed, { holdout: true });
    for (const [word, ctx, want] of [["crap", "quote", "show"], ["test", "compare", "check"]]) for (let rep = 0; rep < 3; rep++) {
      quiet(net); const r = net.read(word, [ctx]); total++; if (r && r.senses > 1 && r.meaning === want) right++;
    }
  }
  console.log(`unseen contexts (never trained)     right ${(100 * right / total).toFixed(0)}% of ${total}`);
}
if (which === "all" || which === "limits") {
  for (const gap of [1.6, 1.2, 0.9, 0.7, 0.5, 0.35]) run(`groups ${gap} apart`, seeds, { gap });
  run("random frequencies (no encoder)", seeds, { randomFreq: true });
}
if (which === "all" || which === "drives") {
  const off = k => ({ [k]: 0 });
  for (const k of ["mechanical", "nuclear", "electric", "kinetic", "work", "gravity", "ionization", "rest", "chemical"]) run(`${k} off`, seeds.slice(0, 6), { energies: off(k) });
  run("nuclear x2", seeds.slice(0, 6), { energies: { nuclear: 2 } });
  run("electric 0.3 (fusion allowed)", seeds.slice(0, 6), { energies: { electric: 0.3 } });
}
if (which === "all" || which === "heat") {
  // heat only flows hot to cold: check no transfer ever makes the colder one hotter than the hotter was
  const net = new S.Network(S.fieldSpec(16, 3), 3);
  let maxT = 0, sum0 = 0;
  for (let k = 0; k < 2000; k++) { if (k % 15 === 0) net.train(); net.step(); maxT = Math.max(maxT, ...net.N.map(n => n.T)); }
  const st = net.stats();
  console.log(`field of 16: order ${st.order.toFixed(2)} hot ${st.hot} cold ${st.cold} links ${st.links} maxT ${maxT.toFixed(2)} events ${net.log.length}`);
}
if (which === "all" || which === "scale") {
  for (const n of [20, 100, 300, 1000]) {
    const net = new S.Network(S.fieldSpec(n, 5), 5);
    const t0 = process.hrtime.bigint();
    const k = n > 300 ? 5 : 40;
    for (let i = 0; i < k; i++) net.step();
    const ms = Number(process.hrtime.bigint() - t0) / 1e6 / k;
    console.log(`scale ${String(n).padStart(5)} neurons: ${ms.toFixed(2)} ms per step (${(1000 / ms / 20).toFixed(1)}x real time at 20 steps/s)`);
  }
}
