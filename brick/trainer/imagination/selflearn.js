// Does it learn by itself? Raw input only (no groups, no lines, random frequencies); groups only score the result.
// node selflearn.js [data.txt questions.txt]   (default: the sample data)
const S = require("./sim.js"), fs = require("fs");
const data = process.argv[2] ? fs.readFileSync(process.argv[2], "utf8") : S.SAMPLE_DATA;
const qs = S.parseQuestions(process.argv[3] ? fs.readFileSync(process.argv[3], "utf8") : process.argv[2] ? "" : S.SAMPLE_QUESTIONS);
const SEEDS = +(process.env.SEEDS || 10), STEPS = +(process.env.STEPS || 3000);
function purity(net) {           // of the settled lines between two words that both have a known group: same group?
  let same = 0, all = 0;
  for (const l of net.links.values()) { if (l.tent != null) continue; const a = net.get(l.a), b = net.get(l.b);
    if (a.truth && b.truth && a.word !== b.word) { all++; if (a.truth === b.truth) same++; } }
  return all ? same / all : 0;
}
function placed(net) {           // words whose settled neighbours mostly share the word's own group (it found its group)
  let right = 0, all = 0;
  for (const n of net.N) {
    if (!n.truth) continue;
    const vote = {};
    for (const l of net.linksOf(n)) { if (l.tent != null) continue; const o = net.get(l.a === n.id ? l.b : l.a);
      if (o.word !== n.word && o.truth) vote[o.truth] = (vote[o.truth] || 0) + l.w; }
    const best = Object.entries(vote).sort((a, b) => b[1] - a[1])[0];
    all++; if (best && best[0] === n.truth) right++;
  }
  return all ? right / all : 0;
}
function run(mode, opts, energies) {
  let ok = 0, judged = 0, early = 0, late = 0, pur = 0, pl = 0, splits = {}, conf = 0, prop = 0;
  for (let seed = 1; seed <= SEEDS; seed++) {
    const net = new S.Network(S.dataSpec(data, mode, seed), seed, energies, null, opts);
    for (let k = 0; k < STEPS; k++) { if (k % S.C.TRAIN_EVERY === 0) net.train(); net.step(); if (k === Math.floor(STEPS / 6)) early += net.niRate(); }
    late += net.niRate(); pur += purity(net); pl += placed(net); conf += net.confirmed; prop += net.proposed;
    for (const w of new Set(net.N.filter(n => n.label.includes("·")).map(n => n.word))) splits[w] = (splits[w] || 0) + 1;
    for (const q of qs) { for (let j = 0; j < 30; j++) { if (net.steps % 20 === 0) net.train(); net.step(); }
      const m = net.matches(net.read(q.word, q.context), q.want); if (m !== null) { judged++; if (m) ok++; } }
  }
  const pc = x => Math.round(100 * x);
  return { asExpected: judged ? pc(ok / judged) : null, placed: pc(pl / SEEDS), niEarly: pc(early / SEEDS), niLate: pc(late / SEEDS), purity: pc(pur / SEEDS),
           proposed: Math.round(prop / SEEDS), kept: Math.round(conf / SEEDS), splits };
}
const out = {
  "given groups, old loop": run("groups", {}),
  "raw input, old loop": run("raw", {}),
  "raw input, intuition": run("raw", { intuition: true }),
  "raw, intuition, Ni off": run("raw", { intuition: true }, { mechanical: 0, work: 0 }),
  "raw, intuition, Ne off": run("raw", { intuition: true }, { radiant: 0, oscillation: 0 }),
};
for (const [k, v] of Object.entries(out)) console.log(k.padEnd(26), JSON.stringify(v));
if (process.env.OUT) fs.writeFileSync(process.env.OUT, JSON.stringify(out, null, 1));
