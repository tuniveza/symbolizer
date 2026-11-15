// brick trainer neurons: train the imagination neurons on data brick hands over, then read each asked word in its line.
// Input (stdin, JSON): { data, questions, mode, seed, steps, brain }. Output (stdout, JSON).
// brain: an archetype name ("mirror oracle"); { list: true } prints the archetypes instead.
const S = require("./sim.js");
let input = "";
process.stdin.on("data", d => input += d);
process.stdin.on("end", () => {
  const job = JSON.parse(input);
  if (job.list) { process.stdout.write(JSON.stringify(S.ARCHETYPES.map(a => ({ name: a.name, type: a.type, bias: a.bias, yours: a.yours, line: a.line })))); return; }
  const brain = job.brain ? S.findArchetype(job.brain) : null;
  if (job.brain && !brain) { process.stdout.write(JSON.stringify({ error: "no archetype called " + job.brain, names: S.ARCHETYPES.map(a => a.name) })); return; }
  const net = new S.Network(S.dataSpec(job.data, job.mode || "groups", job.seed || 1), job.seed || 1, null, brain);
  const steps = job.steps || 3000;
  for (let k = 0; k < steps; k++) { if (k % S.C.TRAIN_EVERY === 0) net.train(); net.step(); }
  const answers = S.parseQuestions(job.questions).map(q => {
    for (let j = 0; j < 30; j++) { if (net.steps % S.C.TRAIN_EVERY === 0) net.train(); net.step(); }
    const r = net.read(q.word, q.context);
    if (!r) return { word: q.word, found: false };
    const best = r.ranked[0];
    return { word: q.word, found: true, verdict: r.verdict, clear: r.clear, meaning: r.meaning, sense: best.s.label,
             matched: best.matched, carried: r.carried, senses: r.ranked.map(x => ({ label: x.s.label, matched: x.matched, near: net.neighbours(x.s).slice(0, 3) })),
             profile: best.profile };
  });
  const split = [...new Set(net.N.filter(n => n.label.includes("·")).map(n => n.word))];
  const archetype = brain ? { name: brain.name, type: brain.type, stack: brain.personality.stack, rules: net.d,
                              warmth: +net.warmth.toFixed(2), orderPull: +net.orderPull.toFixed(2) } : null;
  process.stdout.write(JSON.stringify({ archetype, answers, split, neurons: net.N.length, links: net.links.size, seconds: Math.round(net.t),
                                        events: net.log.filter(l => /fission|fusion/.test(l.text)).map(l => l.text) }));
});
