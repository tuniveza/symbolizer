// brick trainer neurons: the imagination neurons, run by brick (trainer/neurons.py) with node.
// Input (stdin, JSON). Output (stdout, JSON).
//   { list: true }                                   the archetypes
//   { task: "learn", state, data, roles, verdicts, steps, candidates, seed, brain }
//        learn by itself from raw sentences (data), continuing the saved network in the file `state` (made if missing).
//        roles: {word: role} from brick's reader. verdicts: what brick's compiler said about the last candidates.
//        Returns new candidates (Ne's own sentences, for the compiler to check) and how it is doing.
//   { data, questions, mode, seed, steps, brain, state }
//        read each question's word in its context. With `state`, it reads with the saved network (after a short
//        refresher on data); without, it trains a fresh one on data first (the first version: groups given).
const S = require("./sim.js");
const fs = require("fs");
// --serve: stay running and take one job per line (one answer per line), so callers skip starting node each time.
// The network is still read from its file for every job, exactly as a fresh run would, so results are identical.
function handle(job, write) {
  if (job.list) { write(JSON.stringify(S.ARCHETYPES.map(a => ({ name: a.name, type: a.type, bias: a.bias, yours: a.yours, line: a.line })))); return; }
  const brain = job.brain ? S.findArchetype(job.brain) : null;
  if (job.brain && !brain) { write(JSON.stringify({ error: "no archetype called " + job.brain, names: S.ARCHETYPES.map(a => a.name) })); return; }
  const seed = job.seed || 1;
  const tick = net => { if (net.steps % S.C.TRAIN_EVERY === 0) net.train(); net.step(); };

  // A saved network, or a new one grown from the raw sentences alone (nothing given: no groups, no lines).
  function network(withState) {
    const saved = withState && job.state && fs.existsSync(job.state) ? JSON.parse(fs.readFileSync(job.state, "utf8")) : null;
    const spec = S.dataSpec(job.data || "", "raw", seed);
    const enc = job.omegas || {};                         // the encoder's starting frequencies (trainer/encoder.py)
    for (const n of spec.neurons) if (enc[n.label] != null && !n.fixed) n.omega = enc[n.label];
    const net = new S.Network(saved ? { neurons: [] } : spec, seed, null, brain, { intuition: true });
    net.encoded = enc;
    if (saved) net.load(saved);
    net.sentences = spec.sentences;
    net.addWords([...new Set(spec.sentences.flat())]);
    for (const [w, role] of Object.entries(job.roles || {})) for (const n of net.N) if (n.word === w) n.role = role;
    if (job.roles) net.slots(job.roles);
    return { net, fresh: !saved };
  }

  if (job.task === "learn") {
    const { net, fresh } = network(true);
    net.applyVerdicts(job.verdicts);
    net.batch = job.batch || 1;
    const t0 = Date.now();
    for (let k = 0; k < (job.steps || 1200); k++) tick(net);
    const candidates = net.candidates(job.candidates ?? 20);
    if (job.state) fs.writeFileSync(job.state, JSON.stringify(net.save()));
    const links = [...net.links.values()];
    write(JSON.stringify({ fresh, candidates, neurons: net.N.length, links: links.length,
      settled: links.filter(l => l.tent == null).length, proved: links.filter(l => l.proved).length,
      banned: (net.banned || new Set()).size, ni: +net.niRate().toFixed(3), bits: +net.bitRate().toFixed(3),
      phrases: (net.phrases || new Set()).size, slotted: net.N.filter(n => !n.role && net.roleOf(n) && net.roleOf(n).from === "its slot neuron").length, proposed: net.proposed, kept: net.confirmed, cut: net.rejected,
      split: [...new Set(net.N.filter(n => n.label.includes("·")).map(n => n.word))], seconds: Math.round(net.t), ms: Date.now() - t0 }));
    return;
  }

  // The brain's own prediction of the brick each word stands for (its bitstream neurons).
  if (job.task === "bricks") {
    const net = new S.Network({ neurons: [] }, seed, null, null, { intuition: true }).load(JSON.parse(fs.readFileSync(job.state, "utf8")));
    const out = {};
    for (const w of job.words || []) out[w] = net.brickOf(w);
    write(JSON.stringify(out));
    return;
  }

  // Read a whole line with the saved, learned network (no training first): one run for the line.
  if (job.task === "readline") {
    const net = new S.Network({ neurons: [] }, seed, null, brain, { intuition: true }).load(JSON.parse(fs.readFileSync(job.state, "utf8")));
    const answers = net.readLine(job.words || []).map(({ word, result: r }) => {
      if (!r) return { word, found: false };
      const best = r.ranked[0], role = net.roleOf(best.s);
      return { word, found: true, verdict: r.verdict, clear: r.clear, meaning: r.meaning, sense: best.s.label, matched: best.matched,
               carried: r.carried, role: role && role.role, senses: r.ranked.map(x => ({ label: x.s.label, matched: x.matched, near: net.neighbours(x.s).slice(0, 3) })) };
    });
    const missing = (job.words || []).filter(w => !answers.some(a => a.word === w)).map(word => ({ word, found: false }));
    write(JSON.stringify({ answers: answers.concat(missing), split: [], neurons: net.N.length, links: net.links.size, learned: true }));
    return;
  }

  // How precise is what it learned? groups: {name: [words]} (brick's own word groups, used only to score).
  if (job.task === "score") {
    const saved = JSON.parse(fs.readFileSync(job.state, "utf8"));
    const net = new S.Network({ neurons: [] }, seed, null, null, { intuition: true }).load(saved);
    const groupsOf = new Map();
    for (const [g, ws] of Object.entries(job.groups)) for (const w of ws) { if (!groupsOf.has(w)) groupsOf.set(w, new Set()); groupsOf.get(w).add(g); }
    const share = (a, b) => [...(groupsOf.get(a) || [])].some(g => (groupsOf.get(b) || new Set()).has(g));
    const gold = net.N.filter(n => groupsOf.has(n.word) && n.seen >= 3), pred = net.predIndex(), pairs = [];
    for (let i = 0; i < gold.length; i++) for (let j = i + 1; j < gold.length; j++) if (gold[i].word !== gold[j].word) pairs.push([net.alike(gold[i], gold[j], pred), gold[i].word, gold[j].word]);
    pairs.sort((x, y) => y[0] - x[0]);
    const at = k => { const top = pairs.slice(0, k); return top.length ? Math.round(100 * top.filter(p => share(p[1], p[2])).length / top.length) : null; };
    let placed = 0, alikePlaced = 0;
    for (const n of gold) { const vote = new Map(); for (const l of net.linksOf(n)) { if (l.tent != null || !(l.ne || l.proved)) continue; const o = net.get(l.a === n.id ? l.b : l.a);
      if (!o || o.word === n.word || !groupsOf.has(o.word)) continue; for (const g of groupsOf.get(o.word)) vote.set(g, (vote.get(g) || 0) + l.w); }
      const best = [...vote].sort((x, y) => y[1] - x[1])[0]; if (best && groupsOf.get(n.word).has(best[0])) alikePlaced++; }
    for (const n of gold) { const vote = new Map(); for (const l of net.linksOf(n)) { if (l.tent != null) continue; const o = net.get(l.a === n.id ? l.b : l.a);
      if (!o || o.word === n.word || !groupsOf.has(o.word)) continue; for (const g of groupsOf.get(o.word)) vote.set(g, (vote.get(g) || 0) + l.w); }
      const best = [...vote].sort((x, y) => y[1] - x[1])[0]; if (best && groupsOf.get(n.word).has(best[0])) placed++; }
    let chance = 0, cn = 0; for (let k = 0; k < Math.min(pairs.length, 4000); k++) { const p = pairs[Math.floor(Math.random() * pairs.length)]; cn++; if (share(p[1], p[2])) chance++; }
    write(JSON.stringify({ gold: gold.length, p20: at(20), p100: at(100), placed: Math.round(100 * placed / Math.max(1, gold.length)),
      placedAlike: Math.round(100 * alikePlaced / Math.max(1, gold.length)),
      chance: Math.round(100 * chance / Math.max(1, cn)), ni: (saved.stats || {}).ni || 0, top: pairs.slice(0, 12).map(p => p[1] + "~" + p[2]) }));
    return;
  }

  // Reading.
  let net;
  if (job.state) {
    ({ net } = network(true));
    for (let k = 0; k < (job.steps || 600); k++) tick(net);
  } else {
    net = new S.Network(S.dataSpec(job.data, job.mode || "groups", seed), seed, null, brain);
    for (let k = 0; k < (job.steps || 3000); k++) tick(net);
  }
  const answers = S.parseQuestions(job.questions).map(q => {
    for (let j = 0; j < 30; j++) tick(net);
    const r = net.read(q.word, q.context);
    if (!r) return { word: q.word, found: false };
    const best = r.ranked[0], role = net.roleOf(best.s);
    return { word: q.word, found: true, verdict: r.verdict, clear: r.clear, meaning: r.meaning, sense: best.s.label,
             matched: best.matched, carried: r.carried, role: role && role.role, roleFrom: role && role.from,
             senses: r.ranked.map(x => ({ label: x.s.label, matched: x.matched, near: net.neighbours(x.s).slice(0, 3) })), profile: best.profile };
  });
  const split = [...new Set(net.N.filter(n => n.label.includes("·")).map(n => n.word))];
  const archetype = brain ? { name: brain.name, type: brain.type, stack: brain.personality.stack, rules: net.d,
                              warmth: +net.warmth.toFixed(2), orderPull: +net.orderPull.toFixed(2) } : null;
  write(JSON.stringify({ archetype, answers, split, neurons: net.N.length, links: net.links.size, seconds: Math.round(net.t),
                                        learned: !!job.state, events: net.log.filter(l => /fission|fusion/.test(l.text)).map(l => l.text) }));
}

if (process.argv.includes("--serve")) {
  const rl = require("readline").createInterface({ input: process.stdin });
  rl.on("line", line => {
    if (!line.trim()) return;
    let out = "";
    try { handle(JSON.parse(line), s => { out += s; }); } catch (e) { out = JSON.stringify({ error: String(e && e.stack || e) }); }
    process.stdout.write(out.replace(/\n/g, " ") + "\n");
  });
} else {
  let input = "";
  process.stdin.on("data", d => input += d);
  process.stdin.on("end", () => handle(JSON.parse(input), s => process.stdout.write(s)));
}
