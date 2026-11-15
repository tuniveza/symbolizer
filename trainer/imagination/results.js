// Full measurement run for the page. Writes results.json and prints a summary.
const S = require("./sim.js");
const fs = require("fs");
const out = {};
const POS = [["crap", "clear", "remove"], ["crap", "wipe", "remove"], ["crap", "junk", "remove"], ["crap", "remove", "remove"],
             ["crap", "write", "show"], ["crap", "show", "show"], ["crap", "text", "show"], ["crap", "quote", "show"],
             ["test", "write", "show"], ["test", "text", "show"], ["test", "show", "show"],
             ["test", "check", "check"], ["test", "if", "check"], ["test", "compare", "check"], ["test", "equals", "check"]];
const NEG = [["crap", "put"], ["crap", "position"], ["crap", "window"], ["test", "window"], ["test", "wipe"], ["test", "place"]];

function tick(net) { if (net.steps % S.C.TRAIN_EVERY === 0) net.train(); net.step(); }
function trained(seed, o = {}) {
  const spec = S.wordSpec(o.gap ?? 1.2);
  if (o.randomFreq) { const r = S.rng(seed * 31); for (const n of spec.neurons) n.omega = 2 + r() * 4; }
  if (o.holdout) spec.sentences = spec.sentences.filter(x => !(x.includes("crap") && x.includes("quote")) && !(x.includes("test") && x.includes("compare")));
  if (o.third) spec.sentences.push(["put", "crap"], ["place", "crap"], ["crap", "position"]);
  const net = new S.Network(spec, seed, o.energies);
  for (let k = 0; k < (o.steps ?? 3000); k++) tick(net);
  return { net, spec };
}
function ask(net, word, ctx) { for (let i = 0; i < 30; i++) tick(net); return net.read(word, [ctx]); }
function score(o = {}, seeds = 10, pos = POS, neg = NEG) {
  let right = 0, wrong = 0, refused = 0, negOk = 0, negN = 0, splitCrap = 0, splitTest = 0, extra = 0, first = [];
  for (let seed = 1; seed <= seeds; seed++) {
    const { net } = trained(seed, o);
    const split = new Set(net.N.filter(n => n.label.includes("·")).map(n => n.word));
    if (split.has("crap")) splitCrap++; if (split.has("test")) splitTest++;
    extra += [...split].filter(w => w !== "crap" && w !== "test").length;
    const f = net.log.find(l => l.text.startsWith("nuclear fission")); if (f) first.push(f.t);
    for (const [w, c, want] of pos) { const r = ask(net, w, c); if (!r) { refused++; continue; } if (r.meaning === want) right++; else if (r.meaning) wrong++; else refused++; }
    for (const [w, c] of neg) { const r = ask(net, w, c); negN++; if (!r || r.verdict !== "accurate") negOk++; }
  }
  const n = seeds * pos.length;
  return { right: Math.round(100 * right / n), wrong: Math.round(100 * wrong / n), refused: Math.round(100 * refused / n),
           notRelevantOk: negN ? Math.round(100 * negOk / negN) : null, splitCrap, splitTest, wrongSplits: extra, seeds,
           firstSplit: first.length ? Math.round(first.reduce((a, b) => a + b, 0) / first.length) : null };
}
const line = (k, v) => console.log(k.padEnd(30), JSON.stringify(v));

out.base = score(); line("base", out.base);
out.unseen = score({ holdout: true }, 10, [["crap", "quote", "show"], ["test", "compare", "check"]], []); line("unseen", out.unseen);

// a third meaning: crap is also used with the place group
{
  let three = 0, senses = [];
  for (let seed = 1; seed <= 10; seed++) { const { net } = trained(seed, { third: true, steps: 5000 }); const k = net.N.filter(n => n.word === "crap").length; senses.push(k); if (k >= 3) three++; }
  out.third = { three, seeds: 10, senses }; line("third meaning", out.third);
}
// forgetting: stop feeding sentences; how long until a split sense has no lines left
{
  const times = [];
  for (let seed = 1; seed <= 10; seed++) {
    const { net } = trained(seed);
    const senses = net.N.filter(n => n.word === "test" || n.word === "crap");
    const t0 = net.t; let lost = null;
    for (let k = 0; k < 4000 && lost == null; k++) { net.step(); if (senses.some(s => net.N.includes(s) && net.linksOf(s).length === 0)) lost = net.t - t0; }
    times.push(lost == null ? null : Math.round(lost));
  }
  out.forget = { times }; line("forgetting (s)", out.forget);
}
out.gaps = {};
for (const gap of [1.6, 1.2, 0.9, 0.6, 0.4, 0.25]) { out.gaps[gap] = score({ gap }, 8); line("gap " + gap, out.gaps[gap]); }
out.random = score({ randomFreq: true }); line("random frequencies", out.random);
out.drives = {};
for (const e of S.ENERGIES) { out.drives[e.key] = score({ energies: { [e.key]: 0 } }, 6); line(e.key + " off", out.drives[e.key]); }
out.scale = {};
for (const n of [20, 100, 300, 1000]) {
  const net = new S.Network(S.fieldSpec(n, 5), 5);
  const k = n >= 300 ? 4 : 30, t0 = process.hrtime.bigint();
  for (let i = 0; i < k; i++) net.step();
  out.scale[n] = +(Number(process.hrtime.bigint() - t0) / 1e6 / k).toFixed(2); line("scale " + n, out.scale[n] + " ms/step");
}
fs.writeFileSync(__dirname + "/results.json", JSON.stringify(out, null, 1));
console.log("done");
