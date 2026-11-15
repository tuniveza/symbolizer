// Measure every archetype on the same word test, so they can be compared. node archetypes.js [part parts] > part.json
// Same contexts as results.js. Neutral (no archetype) is measured too, as the baseline.
const S = require("./sim.js");
const POS = [["crap", "clear", "remove"], ["crap", "wipe", "remove"], ["crap", "junk", "remove"], ["crap", "remove", "remove"],
             ["crap", "write", "show"], ["crap", "show", "show"], ["crap", "text", "show"], ["crap", "quote", "show"],
             ["test", "write", "show"], ["test", "text", "show"], ["test", "show", "show"],
             ["test", "check", "check"], ["test", "if", "check"], ["test", "compare", "check"], ["test", "equals", "check"]];
const NEG = [["crap", "put"], ["crap", "position"], ["crap", "window"], ["test", "window"], ["test", "wipe"], ["test", "place"]];
const SEEDS = 6;
const tick = net => { if (net.steps % S.C.TRAIN_EVERY === 0) net.train(); net.step(); };
const ask = (net, w, c) => { for (let i = 0; i < 30; i++) tick(net); return net.read(w, [c]); };
function trained(brain, seed, spec = S.wordSpec(1.2), steps = 3000) {
  const net = new S.Network(spec, seed, null, brain);
  for (let k = 0; k < steps; k++) tick(net);
  return net;
}
function measure(brain) {
  let right = 0, wrong = 0, held = 0, negOk = 0, negN = 0, splits = 0, falseSplits = 0, first = [], forget = [], rare = 0, rareN = 0, unseen = 0, unseenN = 0, heat = 0, order = 0;
  for (let seed = 1; seed <= SEEDS; seed++) {
    const net = trained(brain, seed);
    const split = new Set(net.N.filter(n => n.label.includes("·")).map(n => n.word));
    splits += split.has("crap") + split.has("test");
    falseSplits += [...split].filter(w => w !== "crap" && w !== "test").length;
    const f = net.log.find(l => l.text.startsWith("nuclear fission")); if (f) first.push(f.t);
    heat += net.heatNow(); order += net.orderNow();
    for (const [w, c, want] of POS) { const r = ask(net, w, c); if (r && r.meaning === want) right++; else if (r && r.meaning) wrong++; else held++; }
    for (const [w, c] of NEG) { const r = ask(net, w, c); negN++; if (!r || r.verdict !== "accurate") negOk++; }
    // rare meaning: crap now comes up in 1 of every ~60 practice sentences for 150 s
    const rareNet = trained(brain, seed);
    const keep = rareNet.sentences.filter(x => x.includes("crap")).slice(0, 1);
    rareNet.sentences = rareNet.sentences.filter(x => !x.includes("crap")).flatMap(x => Array(6).fill(x)).concat(keep);
    for (let k = 0; k < 3000; k++) tick(rareNet);
    for (const [w, c, want] of POS.filter(p => p[0] === "crap")) { rareN++; const r = ask(rareNet, w, c); if (r && r.meaning === want) rare++; }
    // forgetting: no practice at all; seconds until a split sense has no lines left
    const quiet = trained(brain, seed), senses = quiet.N.filter(n => n.word === "test" || n.word === "crap"), t0 = quiet.t;
    let lost = null;
    for (let k = 0; k < 6000 && lost == null; k++) { quiet.step(); if (senses.some(s => quiet.N.includes(s) && !quiet.linksOf(s).length)) lost = quiet.t - t0; }
    forget.push(lost == null ? 300 : Math.round(lost));
    // contexts never trained
    const spec = S.wordSpec(1.2);
    spec.sentences = spec.sentences.filter(x => !(x.includes("crap") && x.includes("quote")) && !(x.includes("test") && x.includes("compare")));
    const held2 = trained(brain, seed, spec);
    for (const [w, c, want] of [["crap", "quote", "show"], ["test", "compare", "check"]]) { unseenN++; const r = ask(held2, w, c); if (r && r.meaning === want) unseen++; }
  }
  const pct = (a, b) => Math.round(100 * a / b), n = SEEDS * POS.length, avg = xs => xs.length ? Math.round(xs.reduce((a, b) => a + b, 0) / xs.length) : null;
  return { right: pct(right, n), wrong: pct(wrong, n), held: pct(held, n), notRelevant: pct(negOk, negN), split: pct(splits, 2 * SEEDS),
           falseSplits, firstSplit: avg(first), forget: avg(forget), rare: pct(rare, rareN), unseen: pct(unseen, unseenN),
           heat: +(heat / SEEDS).toFixed(2), order: +(order / SEEDS).toFixed(2) };
}
const all = [null, ...S.ARCHETYPES.map(a => a.name)];
const part = +(process.argv[2] || 0), parts = +(process.argv[3] || 1);
const out = {};
all.forEach((name, i) => { if (i % parts === part) { out[name || "(neutral)"] = measure(name); process.stderr.write("."); } });
process.stdout.write(JSON.stringify(out));
