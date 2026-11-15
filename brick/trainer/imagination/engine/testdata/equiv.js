// Runs one equivalence case on the JavaScript engine (../../sim.js) and prints its output; equiv_test.go runs the
// same case on the Go engine and compares the two byte for byte. Case on stdin as JSON:
//   { kind: "word" | "data" | "load", gap, data, mode, seed, brain, intuition, batch, ticks, questions, state }
const S = require("../../sim.js");
const fs = require("fs");
let input = "";
process.stdin.on("data", d => input += d);
process.stdin.on("end", () => {
  const c = JSON.parse(input);
  const seed = c.seed || 1;
  let net;
  if (c.kind === "word") net = new S.Network(S.wordSpec(c.gap), seed, null, c.brain || null, { intuition: !!c.intuition });
  else if (c.kind === "data") net = new S.Network(S.dataSpec(c.data, c.mode, seed), seed, null, c.brain || null, { intuition: !!c.intuition });
  else if (c.kind === "load") {
    net = new S.Network({ neurons: [] }, seed, null, c.brain || null, { intuition: true }).load(JSON.parse(fs.readFileSync(c.state, "utf8")));
    const spec = S.dataSpec(c.data || "", "raw", seed);
    net.encoded = c.omegas || {};
    net.sentences = spec.sentences;
    net.addWords([...new Set(spec.sentences.flat())]);
  }
  if (c.batch) net.batch = c.batch;
  var tick = () => { if (net.steps % S.C.TRAIN_EVERY === 0) net.train(); net.step(); };
  for (let k = 0; k < (c.ticks || 0); k++) tick();
  if (c.reload) {                                    // save, load into a new network, keep going
    const back = new S.Network({ neurons: [] }, seed + 1, null, c.brain || null, { intuition: true }).load(JSON.parse(JSON.stringify(net.save())));
    back.sentences = net.sentences; if (c.batch) back.batch = c.batch;
    net = back;
    for (let k = 0; k < c.reload; k++) tick();
  }
  const answers = S.parseQuestions(c.questions || "").map(q => {
    for (let j = 0; j < 30; j++) tick();
    const r = net.read(q.word, q.context);
    if (!r) return { word: q.word, found: false };
    const best = r.ranked[0], role = net.roleOf(best.s);
    return { word: q.word, found: true, verdict: r.verdict, clear: r.clear, meaning: r.meaning, sense: best.s.label,
             matched: best.matched, carried: r.carried, role: role && role.role, roleFrom: role && role.from,
             senses: r.ranked.map(x => ({ label: x.s.label, matched: x.matched, near: net.neighbours(x.s).slice(0, 3) })), profile: best.profile };
  });
  const cands = c.candidates ? net.candidates(c.candidates) : [];
  const extra = { warmth: net.warmth, orderPull: net.orderPull, steps: net.steps, t: net.t, log: net.log.map(l => l.text), stats: net.stats(),
                  bricks: (c.bricks || []).map(w => net.brickOf(w)),
                  readline: (c.readline ? net.readLine(c.readline) : []).map(({ word, result: r }) => {
                    if (!r) return { word, found: false };
                    const best = r.ranked[0], role = net.roleOf(best.s);
                    return { word, verdict: r.verdict, clear: r.clear, meaning: r.meaning, sense: best.s.label, matched: best.matched, carried: r.carried,
                             role: role && role.role, senses: r.ranked.map(x => ({ label: x.s.label, matched: x.matched, profile: x.profile })) };
                  }) };
  process.stdout.write(JSON.stringify(answers) + "\n" + JSON.stringify(cands) + "\n" + JSON.stringify(extra) + "\n" + JSON.stringify(net.save()) + "\n");
});
