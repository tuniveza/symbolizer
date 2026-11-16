// Imagination neurons: a small simulation of the owner's spec.
// Every neuron oscillates; impacts relay only between neurons in sync; absorbed impact is temperature;
// hot neurons exchange (hot -> cold), cold neurons refine; radiant lights hidden neurons; the imagination
// layer redesigns lines (links, splits, merges, retunes), steered by 8 drives x 2 energies = 16 energies.
(function (root) {
  "use strict";

  const ENERGIES = [
    { key: "mechanical", name: "Mechanical", drive: "Internal abstraction", side: "internal", fn: "Ni", does: "absorbs incoming impact; what it absorbs becomes temperature" },
    { key: "heat", name: "Heat", drive: "Internal abstraction", side: "external", fn: "Fe", does: "a hot neuron passes absorbed impact along its links to colder neurons" },
    { key: "nuclear", name: "Nuclear", drive: "External abstraction", side: "internal", fn: "Ti", does: "a cold neuron holding two meanings splits in two (fission); identical twins merge (fusion)" },
    { key: "radiant", name: "Radiant", drive: "External abstraction", side: "external", fn: "", does: "a hot neuron lights up hidden neurons at matching frequencies, with no link needed" },
    { key: "rest", name: "Rest", drive: "Synchronicity", side: "internal", fn: "", does: "self temperature: each neuron settles back to a steady rhythm and its resting heat" },
    { key: "thermal", name: "Thermal", drive: "Synchronicity", side: "external", fn: "Se", does: "neurons in sync exchange temperature, hot to cold" },
    { key: "gravity", name: "Gravity", drive: "Order", side: "internal", fn: "", does: "pulls the frequencies of linked, synced neurons together into clusters" },
    { key: "ionization", name: "Ionization", drive: "Order", side: "external", fn: "", does: "pushes half-matching neurons apart, so clusters stay cleanly separate" },
    { key: "potential", name: "Potential", drive: "Speed", side: "internal", fn: "", does: "stored heat makes a neuron readier to relay (a lower sync bar)" },
    { key: "kinetic", name: "Kinetic", drive: "Speed", side: "external", fn: "Te", does: "how strongly an impact travels on to the next neuron" },
    { key: "chemical", name: "Chemical", drive: "Maintenance", side: "internal", fn: "", does: "holds each neuron near its own original frequency" },
    { key: "elastic", name: "Elastic", drive: "Maintenance", side: "external", fn: "", does: "links spring back toward their original strength" },
    { key: "electric", name: "Electric", drive: "Individuality", side: "internal", fn: "", does: "keeps a minimum frequency gap, so two neurons never fully merge" },
    { key: "magnetic", name: "Magnetic", drive: "Individuality", side: "external", fn: "", does: "a neuron's field pulls on nearby frequencies even without a link" },
    { key: "work", name: "Mechanical work", drive: "Refinement", side: "internal", fn: "", does: "a cold neuron retunes itself toward the neurons whose impacts reach it" },
    { key: "oscillation", name: "Mechanical oscillation", drive: "Refinement", side: "external", fn: "", does: "a refined neuron's sharper rhythm spreads to the neurons in sync with it" },
  ];

  // How each of the 16 energies is read as evidence that a neuron belongs with a context (see read()).
  const PROOF = {
    mechanical: "impact it absorbed from the context",
    heat: "heat it took in from hot context neurons along its lines",
    nuclear: "how often it was used together with the context (meaning bound at its core)",
    radiant: "light it received from the context",
    rest: "how closely its own rhythm locked onto the context's",
    thermal: "temperature it exchanged with the context",
    gravity: "strength of its lines to the context",
    ionization: "how cleanly it belongs with the context rather than with anything else",
    potential: "activation it built up from the context's impacts",
    kinetic: "impacts it received from the context",
    chemical: "how close its own original frequency is to the context's",
    elastic: "the settled strength its lines to the context spring back to",
    electric: "its own charge, in step with the context",
    magnetic: "how much its field overlaps the context's",
    work: "past impacts from the context it refined itself toward",
    oscillation: "sharp rhythm it shares with the context",
  };

  // Constants (simulation units: time in seconds, frequency in radians per second)
  const C = {
    dt: 0.05, SIGMA: 0.6, SYNC: 0.45, T_HOT: 1.0, T_COLD: 0.5, T_REST: 0.2,
    K: 1.6, RELAY: 1.2, DECAY: 1.6, ABSORB: 0.5, REST: 0.25, HEAT: 0.8, THERMAL: 0.6, RADIANT: 0.6,
    GRAVITY: 0.15, ION: 0.12, GAP: 0.18, ELECTRIC: 0.6, CHEM: 0.04, ELASTIC: 0.02, FIELD: 0.08,
    WORK: 0.35, OSC: 0.5, SHARE: 0.25, SPLIT_GAP: 0.4, MEAN_GAP: 1.0, CO_MIN: 0.15, CO_LINK: 0.1, MIN_GROUP: 2, NOISE: 0.6, LIT_DECAY: 0.4, INBOX_DECAY: 0.015, FANOUT: 2, IMAGINE_EVERY: 20, TRAIN_EVERY: 20,
  };

  function rng(seed) {                       // mulberry32: same seed, same run
    let a = seed >>> 0;
    return function () {
      a |= 0; a = (a + 0x6d2b79f5) | 0;
      let t = Math.imul(a ^ (a >>> 15), 1 | a);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  const pairKey = (a, b) => a < b ? a + "|" + b : b + "|" + a;
  // Special neurons (raw input): "=write" bitstream (a piece of brick's output), "@place" slot (a job in a line),
  // "#2" number (one scale: near frequency = near amount). Order and flip words are ordinary words with a duty.
  const ORDER = new Set(["then", "after", "before", "next", "first", "finally", "until", "while", "when", "once", "afterwards", "later"]);
  const FLIP = new Set(["not", "don't", "dont", "never", "unless", "no", "without", "stop", "nor"]);
  const special = w => /^[=@#]/.test(w);
  function numberValue(w) { const m = /^#~?(\d+(?:\.\d+)?)$/.exec(w); return m ? +m[1] : null; }
  function numberOmega(v) { return 0.4 + 1.3 * Math.log10(1 + v) / Math.log10(10001); }   // 0..10000 -> 0.4..1.7

  function defaults() { const e = {}; for (const x of ENERGIES) e[x.key] = 1; return e; }

  class Network {
    // brain: an archetype (object or name). Its personality sets the 16 energies and steers heat and order;
    // its decisions set the rules (how strict, how soon to split, how long to keep lines, ...). No brain = neutral.
    // opts.intuition: learn by itself (the owner's loop: Se takes in, Si recalls, Ni predicts, Ne proposes; Ti, Te,
    // Fi and Fe decide what stays). Off = the first version, where meaning groups and lines were given.
    constructor(spec, seed = 1, energies = null, brain = null, opts = {}) {
      this.intuition = !!opts.intuition;
      this.ni = []; this.proposed = 0; this.confirmed = 0; this.rejected = 0; this.lastSentence = null;
      this.rand = rng(seed);
      this.brain = brain ? (typeof brain === "string" ? findArchetype(brain) : brain) : null;
      if (brain && !this.brain) throw new Error("no archetype called " + brain);
      const lean = this.brain ? this.brain.personality.energies : defaults();
      this.e = {};
      for (const x of ENERGIES) this.e[x.key] = lean[x.key] * ((energies || {})[x.key] ?? 1);   // "x off" still means off
      this.base = Object.assign({}, this.e);
      this.d = Object.assign({}, NEUTRAL, this.brain ? this.brain.decisions : {});
      this.warmth = 1; this.orderPull = 1;
      this.t = 0; this.steps = 0; this.nextId = 0; this.log = [];
      this.N = []; this.links = new Map();
      this.groups = spec.groups || {};
      for (const n of spec.neurons) this.add(n);
      for (const [a, b, w] of spec.links || []) this.connect(this.byLabel(a), this.byLabel(b), w);
      this.sentences = spec.sentences || [];
    }

    add(n) {
      const r = this.rand;
      const neuron = {
        id: this.nextId++, label: n.label, word: n.word || n.label, group: n.group || null,
        omega: n.omega, omega0: n.omega, theta: r() * Math.PI * 2, amp: 0.6, E: 0, T: C.T_REST, lit: 0,
        inbox: new Map(), co: new Map(), next: new Map(), out: new Map(), fixed: !!n.fixed, truth: n.truth || null, seen: n.seen || 0, stepIn: new Map(), stepHeat: new Map(), stepThermal: new Map(), stepLit: new Map(), litBy: null, refined: 0, x: n.x ?? r(), y: n.y ?? r(), relayed: 0, hidden: !!n.hidden,
      };
      this.N.push(neuron);
      this.predCache = null; this.rarity = null;                        // a new neuron: the cached tables are out of date
      return neuron;
    }

    byLabel(label) { return this.N.find(n => n.label === label); }
    key(a, b) { return a.id < b.id ? a.id * 2097152 + b.id : b.id * 2097152 + a.id; }   // a number: faster than text
    link(a, b) { return this.links.get(this.key(a, b)); }
    connect(a, b, w = 0.5) {
      if (!a || !b || a === b) return;
      this.links.set(this.key(a, b), { a: a.id, b: b.id, w, w0: w, carried: 0, idle: 0 }); this.linkVersion = (this.linkVersion || 0) + 1;
    }
    linksOf(n) {
      if (!this.linkIndex || this.linkIndexSize !== this.links.size || this.linkIndexVersion !== this.linkVersion) {
        this.linkIndex = new Map();
        for (const l of this.links.values()) for (const id of [l.a, l.b]) { let m = this.linkIndex.get(id); if (!m) this.linkIndex.set(id, m = []); m.push(l); }
        this.linkIndexSize = this.links.size; this.linkIndexVersion = this.linkVersion;
      }
      return (this.linkIndex.get(n.id) || []).filter(l => this.links.get(this.key(this.get(l.a) || { id: l.a }, this.get(l.b) || { id: l.b })) === l);
    }
    get(id) {
      if (!this.byId || this.byIdSize !== this.N.length || this.byIdStale) { this.byId = new Map(this.N.map(n => [n.id, n])); this.byIdSize = this.N.length; this.byIdStale = false; }
      return this.byId.get(id);
    }

    say(text) { this.log.push({ t: this.t, text }); if (this.log.length > 200) this.log.shift(); }

    stimulate(words, strength = 1) {
      const set = new Set(words), hit = this.N.filter(n => set.has(n.word));
      for (const n of hit) n.E += strength;
      // used together: the imagination layer notes which neurons were set off by the same moment of input.
      // A word with several senses gets the context only on the sense that fits it best (its contextual association).
      const chosen = [];
      for (const word of set) {
        const senses = hit.filter(n => n.word === word);
        if (senses.length < 2) { chosen.push(...senses); continue; }
        const others = hit.filter(n => n.word !== word);
        const fit = sn => others.reduce((t, o) => { const l = this.link(sn, o); return t + (l ? l.w : 0) + this.sync(sn, o).fsim; }, 0);
        chosen.push(senses.reduce((best, sn) => fit(sn) > fit(best) ? sn : best));
      }
      for (const a of chosen) for (const b of chosen) if (a.word !== b.word) a.co.set(b.id, (a.co.get(b.id) || 0) + strength);
    }

    sync(a, b) {
      const df = a.omega - b.omega, fsim = Math.exp(-((df / C.SIGMA) ** 2));
      return { df, fsim, s: fsim * (1 + Math.cos(b.theta - a.theta)) / 2 };
    }

    step() {
      const N = this.N, n = N.length, e = this.e, dt = C.dt, r = this.rand;
      const dTheta = new Float64Array(n), inc = new Float64Array(n), dOmega = new Float64Array(n);
      for (const x of N) { x.stepIn = new Map(); x.stepHeat = new Map(); x.stepThermal = new Map(); x.stepLit = new Map(); }   // what arrives this step, by source
      const bar = x => C.SYNC - 0.12 * e.potential * Math.min(x.T, 1.5) / 1.5;   // potential: heat lowers the bar

      // Pairs to work on: every pair while small; once big, only pairs close enough in frequency to touch (beyond
      // 1.8 sigma, sync under 4%) plus every linked pair, which gives the same physics without checking all n x n pairs.
      let pairs = null;
      if (n > 120) {
        pairs = [];
        const order = [...N.keys()].sort((p, q) => N[p].omega - N[q].omega), done = new Set(), reach = 1.8 * C.SIGMA;   // beyond this frequency similarity is under 4%
        for (let u = 0; u < n; u++) for (let v = u + 1; v < n && N[order[v]].omega - N[order[u]].omega < reach; v++) {
          const p = Math.min(order[u], order[v]), q = Math.max(order[u], order[v]); pairs.push(p, q); done.add(p * n + q); }
        const index = new Map(N.map((x, k) => [x.id, k]));
        for (const l of this.links.values()) { const p0 = index.get(l.a), q0 = index.get(l.b); if (p0 == null || q0 == null) continue;
          const p = Math.min(p0, q0), q = Math.max(p0, q0); if (!done.has(p * n + q)) { pairs.push(p, q); done.add(p * n + q); } }
      }
      const total = pairs ? pairs.length / 2 : n * (n - 1) / 2;
      // fan-out: what a neuron sends out is its own activation shared among the neurons it reaches; past C.FANOUT
      // (the sum of coupling x sync over its pairs) each one gets a share, so a neuron with many lines can't multiply
      // activation (and the heat it turns into) without bound.
      const fan = new Float64Array(n);
      for (let pi = 0, i = 0, j = 0; pi < total; pi++) {
        if (pairs) { i = pairs[2 * pi]; j = pairs[2 * pi + 1]; } else { j++; if (pi === 0) { i = 0; j = 1; } if (j >= n) { i++; j = i + 1; } }
        const a = N[i], b = N[j], { fsim, s } = this.sync(a, b), l = this.link(a, b);
        const c = (l ? l.w : 0) + C.FIELD * e.magnetic * fsim;
        if (c > 0) { fan[i] += c * s; fan[j] += c * s; }
      }
      const share = new Float64Array(n);
      for (let i = 0; i < n; i++) share[i] = fan[i] > C.FANOUT ? C.FANOUT / fan[i] : 1;
      for (let pi = 0, i = 0, j = 0; pi < total; pi++) {
        if (pairs) { i = pairs[2 * pi]; j = pairs[2 * pi + 1]; } else { j++; if (pi === 0) { i = 0; j = 1; } if (j >= n) { i++; j = i + 1; } }
        const a = N[i];
        {
          const b = N[j];
          const { df, fsim, s } = this.sync(a, b);
          const l = this.link(a, b), w = l ? l.w : 0;
          const c = w + C.FIELD * e.magnetic * fsim;                         // magnetic: a field beyond links
          if (c > 0) {
            const pull = C.K * c * fsim * Math.sin(b.theta - a.theta);        // falling into rhythm
            dTheta[i] += pull; dTheta[j] -= pull;
          }
          // the sync rule: an impact relays only when the two are in sync enough
          if (c > 0 && s > bar(b) && a.E > 0.01) {
            const imp = C.RELAY * e.kinetic * c * a.amp * a.E * s * dt * share[i];
            inc[j] += imp; b.inbox.set(a.id, (b.inbox.get(a.id) || 0) + imp); b.stepIn.set(a.id, imp); a.relayed += imp;
            if (l) l.carried += imp;
          }
          if (c > 0 && s > bar(a) && b.E > 0.01) {
            const imp = C.RELAY * e.kinetic * c * b.amp * b.E * s * dt * share[j];
            inc[i] += imp; a.inbox.set(b.id, (a.inbox.get(b.id) || 0) + imp); a.stepIn.set(b.id, imp); b.relayed += imp;
            if (l) l.carried += imp;
          }
          // order: gravity pulls linked synced neurons together, ionization pushes half-matches apart
          if (l && s > C.SYNC) {
            const g = e.gravity * C.GRAVITY * (b.omega - a.omega) * s;
            dOmega[i] += g; dOmega[j] -= g;
          } else if (!l && fsim > 0.12 && fsim < 0.5) {                  // only neurons with no line between them
            const p = e.ionization * C.ION * Math.sign(df || 1) * fsim;
            dOmega[i] += p; dOmega[j] -= p;
          }
          // individuality: electric keeps a minimum gap
          if (Math.abs(df) < C.GAP) {
            const p = e.electric * C.ELECTRIC * Math.sign(df || (r() - 0.5)) * (C.GAP - Math.abs(df)) / C.GAP;
            dOmega[i] += p; dOmega[j] -= p;
          }
          // thermal: temperature exchange between neurons in sync, hot to cold
          if (s > C.SYNC) {
            const f = e.thermal * C.THERMAL * s * (a.T - b.T) * dt;
            a.T -= f; b.T += f;
            if (f > 0) b.stepThermal.set(a.id, f); else if (f < 0) a.stepThermal.set(b.id, -f);
          }
          // heat: a neuron hot enough passes absorbed impact along its links, hot to cold
          if (l) {
            const hot = a.T >= b.T ? a : b, cold = hot === a ? b : a;
            if (hot.T > C.T_HOT) {
              const f = e.heat * C.HEAT * l.w * (hot.T - cold.T) * dt;
              hot.T -= f; cold.T += f; cold.stepHeat.set(hot.id, f);
            }
          }
          // radiant: a hot neuron lights up others at matching frequencies, no link needed
          if (a.T > C.T_HOT) { const add = e.radiant * C.RADIANT * (a.T - C.T_HOT) * fsim * dt; if (add > 0.002 && b.lit < 1) { b.lit += add; b.litBy = a.id; } if (add > 0) b.stepLit.set(a.id, add); }
          if (b.T > C.T_HOT) { const add = e.radiant * C.RADIANT * (b.T - C.T_HOT) * fsim * dt; if (add > 0.002 && a.lit < 1) { a.lit += add; a.litBy = b.id; } if (add > 0) a.stepLit.set(b.id, add); }
          // mechanical oscillation: a refined neuron's sharper rhythm spreads to synced, linked neighbours
          if (l && s > C.SYNC) {
            const f = e.oscillation * C.OSC * s * (a.amp - b.amp) * dt;
            a.amp -= f; b.amp += f;
          }
        }
      }

      for (let i = 0; i < n; i++) {
        const a = N[i];
        const noise = C.NOISE / (1 + 2 * e.rest);                            // rest: a steadier self rhythm
        a.theta += (a.omega + dTheta[i]) * dt + noise * Math.sqrt(dt) * (r() - 0.5);
        a.E += inc[i] - a.E * C.DECAY * dt;
        a.T += e.mechanical * C.ABSORB * inc[i];                             // mechanical: absorbing impact
        a.T += (C.T_REST - a.T) * e.rest * C.REST * dt;                      // rest: self temperature
        if (a.T > C.T_HOT) a.amp -= 0.15 * (a.T - C.T_HOT) * dt;              // hot: the rhythm blurs
        // cold: the neuron refines its own abstraction (mechanical work toward its useful inputs)
        a.refined = 0;
        if (a.T < C.T_COLD && a.inbox.size) {
          const split = this.spread(a);
          if (split && !split.bimodal && split.total > 0.05) {
            dOmega[i] += e.work * C.WORK * (split.mean - a.omega);
            a.amp += e.work * 0.2 * (1 - a.amp) * dt;
            a.refined = 1;
          }
        }
        dOmega[i] += e.chemical * C.CHEM * (a.omega0 - a.omega);              // chemical: hold your own frequency
        a.omega += dOmega[i] * dt;
        if (a.fixed) a.omega = a.omega0;                                    // bitstream, slot and number neurons hold still
        a.amp = Math.min(1, Math.max(0.15, a.amp));
        a.T = Math.max(0, a.T);
        a.lit = Math.max(0, Math.min(1, a.lit * (1 - C.LIT_DECAY * dt)));
        for (const m of [a.inbox, a.co]) for (const [k, v] of m) { const nv = v * (1 - C.INBOX_DECAY * dt); if (nv < 1e-4) m.delete(k); else m.set(k, nv); }
      }
      for (const l of this.links.values()) {                                  // elastic: links spring back
        l.w += this.e.elastic * C.ELASTIC * (l.w0 - l.w) * dt;
        l.idle = l.carried > 0.001 ? 0 : l.idle + dt;
        l.carried *= 0.9;
      }
      this.t += dt; this.steps++;
      if (this.steps % C.IMAGINE_EVERY === 0) { this.steer(); this.imagine(); }
    }

    // The personality, reacting: it measures how warm and how ordered the network is and nudges the energies toward
    // its own target (warmer: more absorbing, less resting; more order: more gravity, less holding to old frequencies).
    heatNow() { return this.N.reduce((t, n) => t + n.T, 0) / Math.max(1, this.N.length); }
    orderNow() { let s = 0, c = 0; for (const l of this.links.values()) { const a = this.get(l.a), b = this.get(l.b); if (a && b) { s += this.sync(a, b).s; c++; } } return c ? s / c : 0; }
    steer() {
      if (!this.brain || !this.d.react) return;
      const tg = this.brain.personality.target, k = this.d.react, clamp = v => Math.min(2, Math.max(0.5, v));
      this.warmth = clamp(this.warmth + k * (tg.heat - this.heatNow()));
      this.orderPull = clamp(this.orderPull + 4 * k * (tg.order - this.orderNow()));
      const e = this.e, b = this.base;
      if (b.mechanical) e.mechanical = b.mechanical * this.warmth;
      if (b.rest) e.rest = b.rest / this.warmth;
      if (b.gravity) e.gravity = b.gravity * this.orderPull;
      if (b.chemical) e.chemical = b.chemical / this.orderPull;
    }

    // Where a neuron's impacts come from: the frequencies of its sources, weighted, and whether they form two groups.
    spread(a, from = "inbox") {
      const items = [];
      for (const [id, v] of a[from]) { const s = this.get(id); if (s && s.word !== a.word) items.push({ s, v }); }
      if (!items.length) return null;
      items.sort((x, y) => x.s.omega - y.s.omega);
      const total = items.reduce((t, x) => t + x.v, 0);
      const mean = items.reduce((t, x) => t + x.v * x.s.omega, 0) / total;
      let best = null;                                                       // the widest gap that leaves weight on both sides
      let left = 0;
      for (let k = 0; k < items.length - 1; k++) {
        left += items[k].v;
        const gap = items[k + 1].s.omega - items[k].s.omega, share = Math.min(left, total - left) / total;
        if (share > C.SHARE && gap > C.SPLIT_GAP * C.SIGMA && (!best || gap * share > best.gap * best.share)) best = { k, gap, share };
      }
      if (!best) return { total, mean, bimodal: false };
      const lo = items.slice(0, best.k + 1), hi = items.slice(best.k + 1);
      const m = xs => xs.reduce((t, x) => t + x.v * x.s.omega, 0) / xs.reduce((t, x) => t + x.v, 0);
      const loMean = m(lo), hiMean = m(hi);
      if (hiMean - loMean < C.MEAN_GAP * C.SIGMA) return { total, mean, bimodal: false };   // two sides, but not two meanings
      return { total, mean, bimodal: true, lo, hi, loMean, hiMean, share: best.share, gap: best.gap };
    }

    // Contexts: the neurons used at the same moment as this one, grouped by whether they are also used with
    // each other. Two groups that never meet are two contexts, so two meanings. Frequencies play no part.
    contexts(a) {
      const partners = [];
      let top = 0;
      for (const [id, v] of a.co) { const s = this.get(id); if (s && s.word !== a.word) { partners.push({ s, v }); top = Math.max(top, v); } }
      const strong = partners.filter(p => p.v > C.CO_MIN * top);
      if (strong.length < 2) return null;
      const seen = new Set(), groups = [];
      for (const p of strong) {
        if (seen.has(p.s.id)) continue;
        const group = [], todo = [p];
        seen.add(p.s.id);
        while (todo.length) {
          const x = todo.pop(); group.push(x);
          for (const q of strong) if (!seen.has(q.s.id)) {
            const together = Math.max(x.s.co.get(q.s.id) || 0, q.s.co.get(x.s.id) || 0), line = this.link(x.s, q.s);
            if (together > C.CO_LINK * Math.min(x.v, q.v) || (line && line.w > 0.2)) { seen.add(q.s.id); todo.push(q); }
          }
        }
        groups.push(group);
      }
      const total = strong.reduce((t, x) => t + x.v, 0);
      const weight = g => g.reduce((t, x) => t + x.v, 0);
      groups.sort((x, y) => weight(y) - weight(x));
      const big = groups.filter(g => g.length >= C.MIN_GROUP);                  // a context needs at least two members
      groups.length = 0; groups.push(...big);
      if (groups.length < 2 || weight(groups[1]) / total < C.SHARE) return { total, bimodal: false, groups: groups.length };
      const m = xs => xs.reduce((t, x) => t + x.v * x.s.omega, 0) / weight(xs);
      let [lo, hi] = groups;
      if (m(lo) > m(hi)) [lo, hi] = [hi, lo];
      return { total, bimodal: true, lo, hi, loMean: m(lo), hiMean: m(hi), share: weight(groups[1]) / total, groups: groups.length };
    }

    // The imagination layer: redesigns lines, steered by the drives.
    imagine() {
      const e = this.e;
      if (this.intuition) this.decide();
      // nuclear fission: a cold neuron whose impacts come from two separate groups splits into two
      for (const a of [...this.N]) {
        if (a.T >= C.T_COLD || a.fixed) continue;
        const sp = this.contexts(a);                                         // the layer sees use, not only impacts
        if (!sp || !sp.bimodal || sp.total < 0.05) continue;
        const score = e.nuclear * sp.share * 2 * (0.5 + 0.5 * e.electric) - 0.25 * e.chemical;
        if (score > this.d.split) this.fission(a, sp);
      }
      // nuclear fusion: two neurons with nearly the same frequency and the same neighbours merge (electric resists)
      if (e.electric < 0.5) {
        for (let i = 0; i < this.N.length; i++) for (let j = i + 1; j < this.N.length; j++) {
          const a = this.N[i], b = this.N[j];
          if (a.word === b.word && Math.abs(a.omega - b.omega) < 0.12 && e.nuclear * (1 - e.electric) > 0.5) { this.fusion(a, b); return; }
        }
      }
      // lines: links that carry impact grow (order); links idle a long time fade and go (refinement)
      for (const [k, l] of this.links) {
        if (l.carried > 0.01) l.w = Math.min(1, l.w + 0.02 * e.gravity);
        if (l.idle > this.d.patience / Math.max(0.2, e.work)) { l.w -= 0.05; if (l.w <= 0.03) { this.links.delete(k); const a = this.get(l.a), b = this.get(l.b); this.say(`refinement: the unused line ${a.label}—${b.label} faded away`); } }
      }
      // radiant + imagination: a hidden, unlinked neuron that was lit up gets retuned and joined, regardless of sync
      for (const a of this.N) {
        if (a.lit < this.d.radiant || a.litBy == null || this.linksOf(a).length) continue;
        const src = this.get(a.litBy);
        if (!src || e.radiant <= 0) continue;
        a.omega0 = a.omega = a.omega + 0.6 * (src.omega - a.omega);
        this.connect(a, src, 0.3);
        a.hidden = false;
        this.say(`radiant: hidden ${a.label} was lit up by ${src.label}; the imagination layer tuned it closer and joined them`);
      }
    }

    // ---------- intuition: learning by itself ----------
    fnPower(fn) { const [a, b] = FUNCTIONS[fn]; return (this.e[a] + this.e[b]) / 2; }

    // The sense of a word that fits the rest of the moment best (its contextual association).
    senseIn(word, words) {
      const senses = this.N.filter(n => n.word === word);
      if (senses.length < 2) return senses[0] || null;
      const others = this.N.filter(n => n.word !== word && words.includes(n.word));
      const fit = sn => others.reduce((t, o) => { const l = this.link(sn, o); return t + (l ? l.w : 0) + this.sync(sn, o).fsim; }, 0);
      return senses.reduce((best, sn) => fit(sn) > fit(best) ? sn : best);
    }

    // Ni predicts each next word from the words before it. A wrong guess becomes absorbed impact (heat); a right guess
    // lets a cold neuron refine. Each earlier word then expects what really came a little more (more after a surprise).
    // Te: words that keep predicting each other get a line. Ne: a surprise lights up proposals.
    intuit(words) {
      const seq = words.filter(w => !/^[=@]/.test(w)).map(w => this.senseIn(w, words)).filter(Boolean);
      for (const x of seq) x.seen++;
      let start = 0;                                                    // order neurons: a new step starts after them
      const ni = this.fnPower("Ni"), ne = this.fnPower("Ne"), te = this.fnPower("Te"), R = 0.5;
      for (let i = 1; i < seq.length; i++) {
        const x = seq[i], score = new Map();
        if (ORDER.has(seq[i - 1].word)) start = i - 1;                 // Ni still predicts what follows "then" ...
        let total = 0;
        for (let j = i - 1, r = 1; j >= start && r > 0.1; j--, r *= R) for (const [id, v] of seq[j].next) { score.set(id, (score.get(id) || 0) + r * v); total += r * v; }
        let best = null, top = 0;
        for (const [id, v] of score) if (v > top) { top = v; best = id; }
        const p = total > 0 ? (score.get(x.id) || 0) / (total + 0.2) : 0;   // 0.2 keeps room for the unexpected
        this.ni.push(best === x.id ? 1 : 0); if (this.ni.length > 300) this.ni.shift();
        const surprise = 1 - p, eta = 0.25 * ni;
        for (let j = i - 1, r = 1; j >= start && r > 0.1; j--, r *= R) {   // ... but not across it from the step before
          const c = seq[j], k = eta * r;
          if (c.word === x.word) continue;
          for (const [id, v] of c.next) if (id !== x.id) { const nv = v * (1 - 0.1 * k); if (nv < 1e-3) c.next.delete(id); else c.next.set(id, nv); }
          const old = c.next.get(x.id) || 0, nv = old + k * surprise * (1 - old);
          c.next.set(x.id, nv);
          if (nv > 0.35 / Math.max(0.3, te) && !this.link(c, x)) { this.connect(c, x, 0.3); }   // Te: it works, keep a line
        }
        x.T += C.ABSORB * this.e.mechanical * surprise * 0.4;                                    // Ni: a wrong guess is heat
        if (p > 0.5 && x.T < C.T_COLD) x.amp = Math.min(1, x.amp + 0.05 * this.e.work);          // Ni: a right guess refines
        if (surprise > 0.6 && this.rand() < 0.5 * ne) this.propose(x, i ? seq[i - 1] : null);
      }
      this.predCache = null; this.rarity = null;
    }

    // Who comes before each neuron, and how strongly (built from every neuron's predictions).
    predIndex() {
      if (this.predCache) return this.predCache;
      const pred = new Map();
      for (const a of this.N) for (const [id, v] of a.next) { let m = pred.get(id); if (!m) pred.set(id, m = new Map()); m.set(a.id, v); }
      return this.predCache = pred;
    }
    // How alike two neurons are used: what comes before them and what they predict, compared (cosine).
    // A context neuron used everywhere (like "in") says little, so each one counts less the more it is used.
    alike(a, b, pred = this.predIndex()) {
      const pa = pred.get(a.id) || new Map(), pb = pred.get(b.id) || new Map();
      const rare = this.rarity || (this.rarity = this.rarityOf());
      // a neuron's own size (sum of squares) only depends on itself, so within one pass (propose, decide,
      // candidates: nothing changes meanwhile) it is worked out once, adding in the same order as before
      const norm = (x, px) => {
        const memo = this._norm ? this._norm.get(x.id) : undefined;   // no pass running: work it out
        if (memo !== undefined) return memo;
        let t = 0;
        for (const [k, v] of x.next) { const q = v * (rare.get(k) || 1); t += q * q; }
        for (const [k, v] of px) { const q = v * (rare.get(k) || 1); t += q * q; }
        if (this._norm) this._norm.set(x.id, t);
        return t;
      };
      let dot = 0;
      for (const [k, v] of a.next) if (b.next.has(k) && k !== a.id && k !== b.id) dot += v * (rare.get(k) || 1) * b.next.get(k) * (rare.get(k) || 1);
      for (const [k, v] of pa) if (pb.has(k) && k !== a.id && k !== b.id) dot += v * (rare.get(k) || 1) * pb.get(k) * (rare.get(k) || 1);
      const na = norm(a, pa), nb = norm(b, pb);
      return na && nb ? dot / Math.sqrt(na * nb) : 0;
    }
    rarityOf() {
      const total = this.N.reduce((t, n) => t + n.seen, 0) || 1, out = new Map();
      for (const n of this.N) out.set(n.id, Math.log(1 + total / (1 + n.seen)) / Math.log(1 + total));
      return out;
    }

    // Ne proposes: lines to neurons used in the same places as this one (it could stand in for them).
    propose(x) {
      const pred = this.predIndex(), cands = [];
      if (x.seen < 3 || special(x.word)) return;
      this._norm = new Map();                                           // sizes worked out once in this pass                        // Ti: too little evidence to say anything yet
      this.banned = this.banned || new Set();
      const near = new Set();                                            // only words that share a neighbour can be alike
      for (const k of x.next.keys()) for (const p of (pred.get(k) || new Map()).keys()) near.add(p);
      for (const p of (pred.get(x.id) || new Map()).keys()) { const q = this.get(p); if (q) for (const k of q.next.keys()) near.add(k); }
      for (const id of near) {
        const y = this.get(id);
        if (!y || y === x || y.word === x.word || y.seen < 3 || special(y.word) || this.link(x, y) || this.banned.has(pairKey(x.word, y.word))) continue;
        const sim = this.alike(x, y, pred);
        if (sim > 0.25) cands.push([y, sim]);
      }
      this._norm = null;
      cands.sort((a, b) => b[1] - a[1]);
      for (const [y] of cands.slice(0, 2)) { this.connect(x, y, 0.15); this.link(x, y).tent = this.t; this.proposed++; }
    }

    // Ti, Te, Fi and Fe decide what stays. Fi: two neurons must be alike enough to share a line (each keeps itself).
    // Te: a proposal that holds is strengthened until it's a real line; one that doesn't is cut. Fe: linked neurons
    // align their frequencies, so meanings settle into bands by use alone. (Ti splits and merges in imagine().)
    decide() {
      const pred = this.predIndex(), te = this.fnPower("Te"), fe = this.fnPower("Fe"), fi = this.fnPower("Fi");
      this._norm = new Map();
      for (const [k, l] of this.links) {
        const a = this.get(l.a), b = this.get(l.b);
        if (!a || !b) continue;
        if (l.tent != null && this.t - l.tent > 8) {
          if (this.alike(a, b, pred) > 0.35 * fi) {
            l.w = Math.min(1, l.w + 0.08 * te);
            if (l.w >= 0.3) { l.tent = null; l.w0 = l.w; l.ne = true; this.confirmed++; this.say(`Ne proposed, Te kept: ${a.label}—${b.label} (used in the same places)`); }
          } else { l.w -= 0.05 * Math.max(0.2, te); if (l.w <= 0.03) { this.links.delete(k); this.rejected++; continue; } }
        }
        if (l.tent == null) {
          const pull = fe * 0.02 * l.w * (b.omega - a.omega);
          if (a.fixed && b.fixed) continue;
          if (a.fixed) b.omega -= 2 * pull; else if (b.fixed) a.omega += 2 * pull; else { a.omega += pull; b.omega -= pull; }
        }
      }
      this._norm = null;
      for (const n of this.N) if (!n.fixed) n.omega0 += 0.05 * (n.omega - n.omega0);   // what it learned becomes its own frequency
      this.fusePhrases(pred);
      if (this.steps % (C.IMAGINE_EVERY * 10) === 0) this.assignSlots();
    }
    // ---------- bitstream neurons ----------
    // From every sentence brick built, each word learns which pieces of output ("=write", "=wait") it makes:
    // a delta rule, like Ni's guesses. Before learning, Ni guesses the output from the words alone, and that guess is
    // scored (bitRate): the brain's own measure of how precisely it turns English into bits. A word right after a flip
    // neuron ("don't clear") learns nothing from it. Te gives a word a line to an output it reliably makes, and Fe
    // then pulls it toward that output's fixed frequency: words that make the same bits end up tuned alike.
    learnBits(words) {
      const want = new Set(words.out), outs = words.out.map(o => this.N.find(n => n.word === o)).filter(Boolean);
      if (!outs.length) return;
      const seq = words.map(w => this.senseIn(w, words)).filter(x => x && !special(x.word));
      const flipped = new Set();
      seq.forEach((x, i) => { if (FLIP.has(x.word) && seq[i + 1]) flipped.add(seq[i + 1]); });
      const guess = this.guessBits(seq.filter(x => !flipped.has(x)));
      if (guess.length) { this.bits = this.bits || []; this.bits.push(want.has(guess[0].o) ? 1 : 0); if (this.bits.length > 300) this.bits.shift(); }
      const eta = 0.2 * this.fnPower("Ni"), te = this.fnPower("Te");
      for (const x of seq) {
        if (flipped.has(x)) continue;
        for (const [id, v] of x.out) if (!outs.some(o => o.id === id)) { const nv = v * (1 - 0.15 * eta); if (nv < 1e-3) x.out.delete(id); else x.out.set(id, nv); }
        for (const o of outs) {
          const nv = (x.out.get(o.id) || 0) + eta * (1 - (x.out.get(o.id) || 0));
          x.out.set(o.id, nv);
          if (nv > 0.5 / Math.max(0.3, te) && x.seen >= 3 && !this.link(x, o)) { this.connect(x, o, 0.4); this.link(x, o).bits = true; }
        }
      }
    }
    // Which outputs the words would make: each word's learned outputs, or for a word with none, its settled lines'.
    guessBits(seq) {
      const score = new Map();
      for (const x of seq) {
        let m = x.out;
        if (!m.size) { m = new Map(); for (const l of this.linksOf(x)) { if (l.tent != null) continue; const o = this.get(l.a === x.id ? l.b : l.a);
          if (o && !special(o.word)) for (const [id, v] of o.out) m.set(id, (m.get(id) || 0) + v * l.w); } }
        for (const [id, v] of m) score.set(id, (score.get(id) || 0) + v);
      }
      const total = [...score.values()].reduce((a, b) => a + b, 0);
      return [...score].map(([id, v]) => ({ o: this.get(id)?.word, share: total ? v / total : 0 })).filter(x => x.o).sort((a, b) => b.share - a.share);
    }
    // The brain's own prediction of the brick a word stands for: its strongest output, and how sure (share).
    brickOf(word) {
      const n = this.N.filter(x => x.word === word).sort((a, b) => b.seen - a.seen)[0];
      if (!n) return null;
      const g = this.guessBits([n]);
      return g.length ? { output: g[0].o, share: +g[0].share.toFixed(3), second: g[1] ? g[1].o : null } : null;
    }
    bitRate() { return this.bits && this.bits.length ? this.bits.reduce((a, b) => a + b, 0) / this.bits.length : 0; }

    // ---------- slot neurons ----------
    // One neuron per job a word can have in a line ("@do", "@place", "@colour" ...). Words brick reads are lined to
    // their slot; a word brick doesn't know gets lined to the slot its settled lines clearly vote for (2+ votes, 60%+).
    slots(roles) {
      const band = Math.max(4, this.N.length / 25), kinds = [...new Set(Object.values(roles || {}))].sort();
      kinds.forEach((r, k) => { const w = "@" + r; if (!this.N.some(n => n.word === w)) this.add({ label: w, omega: 2 + band * (k + 0.5) / kinds.length, x: 0.05 + 0.9 * (k + 0.5) / kinds.length, y: 0.97, fixed: true }); });
      for (const n of this.N) { if (!n.role || special(n.word)) continue; const slot = this.N.find(x => x.word === "@" + n.role);
        if (slot && !this.link(n, slot)) { this.connect(n, slot, 0.3); this.link(n, slot).proved = "role"; } }
    }
    assignSlots() {
      for (const n of this.N) {
        if (n.role || special(n.word) || n.seen < 3 || this.linksOf(n).some(l => { const o = this.get(l.a === n.id ? l.b : l.a); return o && o.word[0] === "@"; })) continue;
        const vote = new Map(); let total = 0;
        for (const l of this.linksOf(n)) { if (l.tent != null) continue; const o = this.get(l.a === n.id ? l.b : l.a);
          if (o && o.role && !special(o.word)) { vote.set(o.role, (vote.get(o.role) || 0) + 1); total++; } }
        const best = [...vote].sort((a, b) => b[1] - a[1])[0];
        if (best && best[1] >= 2 && best[1] / total >= 0.6) { const slot = this.N.find(x => x.word === "@" + best[0]); if (slot) this.connect(n, slot, 0.2); }
      }
    }

    // ---------- phrase neurons ----------
    // Ti fuses: when Ni sees one word almost always come right after another (it predicts it, and nothing else
    // usually comes before it), the two become one phrase neuron ("top right", then "top right corner"). A phrase
    // neuron is used like a word: it has its own frequency, lines, senses and role, so it can fill a slot.
    fusePhrases(pred = this.predIndex()) {
      this.phrases = this.phrases || new Set();
      const ti = this.fnPower("Ti"), cap = Math.max(20, Math.round(this.N.length * 0.3));
      if (this.phrases.size >= cap || ti <= 0) return;
      const made = [];
      for (const c of this.N) {
        if (c.seen < 5 || c.word.split(" ").length >= 4 || special(c.word) || ORDER.has(c.word)) continue;
        for (const [id, v] of c.next) {
          const x = this.get(id);
          if (!x || x.seen < 5 || v < 0.6 / Math.min(1.5, ti) || x.word === c.word || special(x.word) || ORDER.has(x.word)) continue;
          const word = c.word + " " + x.word;
          if (word.split(" ").length > 4 || this.phrases.has(word)) continue;
          const before = pred.get(x.id) || new Map(); let total = 0; for (const [, u] of before) total += u;
          if (total && v / total > 0.5) made.push([c, x, word]);
        }
      }
      for (const [c, x, word] of made.slice(0, 5)) {
        if (this.phrases.size >= cap) break;
        const p = this.add({ label: word, word, omega: (c.omega + x.omega) / 2, x: (c.x + x.x) / 2, y: (c.y + x.y) / 2 + 0.03 });
        p.parts = [c.word, x.word]; p.phrase = true;
        this.connect(p, c, 0.5); this.connect(p, x, 0.5);
        this.phrases.add(word);
        this.say(`Ti fused "${word}" into one phrase neuron (Ni nearly always sees ${x.word} right after ${c.word})`);
      }
    }
    // A sentence with its phrases joined, longest first: "at the top right corner" -> at, the, "top right corner".
    chunk(words) {
      if (!this.phrases || !this.phrases.size) return words;
      const out = [];
      for (let i = 0; i < words.length;) {
        let took = 1;
        for (let k = Math.min(4, words.length - i); k > 1; k--) if (this.phrases.has(words.slice(i, i + k).join(" "))) { took = k; break; }
        out.push(words.slice(i, i + took).join(" ")); i += took;
      }
      return out;
    }

    niRate() { return this.ni.length ? this.ni.reduce((a, b) => a + b, 0) / this.ni.length : 0; }

    // Save and load everything it has learned (by label, so it survives in a file).
    save() {
      const lab = id => this.get(id)?.label;
      const map = m => Object.fromEntries([...m].filter(([id]) => lab(id) != null).map(([id, v]) => [lab(id), +v.toFixed(4)]));
      return { version: 1, t: this.t, steps: this.steps, senseCount: this.senseCount || {}, intuition: this.intuition,
        neurons: this.N.map(n => ({ label: n.label, word: n.word, group: n.group, truth: n.truth, omega: +n.omega.toFixed(4), omega0: +n.omega0.toFixed(4),
          T: +n.T.toFixed(3), amp: +n.amp.toFixed(3), seen: n.seen, role: n.role || null, x: +n.x.toFixed(3), y: +n.y.toFixed(3), next: map(n.next), co: map(n.co), out: map(n.out), fixed: n.fixed || undefined })),
        links: [...this.links.values()].map(l => [lab(l.a), lab(l.b), +l.w.toFixed(4), +l.w0.toFixed(4), l.tent == null ? null : l.tent, l.ne ? 1 : 0, l.proved || 0]),
        banned: [...(this.banned || [])], checked: [...(this.checked || [])], phrases: [...(this.phrases || [])],
        stats: { proposed: this.proposed, confirmed: this.confirmed, rejected: this.rejected, ni: +this.niRate().toFixed(3), bits: +this.bitRate().toFixed(3) } };
    }
    load(state) {
      this.N = []; this.links = new Map(); this.nextId = 0;
      for (const n of state.neurons) { const x = this.add(n); Object.assign(x, { omega0: n.omega0, T: n.T, amp: n.amp, group: n.group, truth: n.truth, seen: n.seen || 0, role: n.role || null }); }
      const byLabel = new Map(this.N.map(n => [n.label, n]));
      state.neurons.forEach((n, i) => { const x = this.N[i];
        for (const [l, v] of Object.entries(n.next || {})) if (byLabel.has(l)) x.next.set(byLabel.get(l).id, v);
        for (const [l, v] of Object.entries(n.co || {})) if (byLabel.has(l)) x.co.set(byLabel.get(l).id, v);
        for (const [l, v] of Object.entries(n.out || {})) if (byLabel.has(l)) x.out.set(byLabel.get(l).id, v);
        x.fixed = !!n.fixed; });
      this.t = state.t || 0;                                             // before the lines: their ages count from it
      // a tentative line keeps the time it was proposed (files from before kept 0: count those as made at the saved
      // time, not at time 0). A line from a neuron to itself is skipped.
      for (const [a, b, w, w0, tent, ne, proved] of state.links) { const A = byLabel.get(a), B = byLabel.get(b); if (!A || !B || A === B) continue; this.connect(A, B, w0); const l = this.link(A, B); l.w = w; if (tent != null) l.tent = tent > 0 ? tent : this.t; if (ne) l.ne = true; if (proved) l.proved = proved === 1 ? "role" : proved; }
      this.banned = new Set(state.banned || []); this.checked = new Set(state.checked || []); this.phrases = new Set(state.phrases || []);
      for (const n of this.N) if (this.phrases.has(n.word)) { n.phrase = true; n.parts = n.word.split(" "); }
      this.senseCount = state.senseCount || {}; this.t = state.t || 0; this.steps = state.steps || 0;
      Object.assign(this, state.stats ? { proposed: state.stats.proposed, confirmed: state.stats.confirmed, rejected: state.stats.rejected } : {});
      this.predCache = null;
      return this;
    }
    // ---------- the reality check (brick's compiler, done outside the engine) ----------
    // Ne's own sentences: a known sentence with one word swapped for a word Ne thinks is used the same way. Each one
    // tests one line between two words. Lines already checked are not asked about again.
    candidates(k = 20) {
      const pred = this.predIndex(), out = [], asked = new Set();
      this.checked = this.checked || new Set();
      const lines = [...this.links.values()].map(l => ({ l, a: this.get(l.a), b: this.get(l.b) }))
        .filter(x => x.a && x.b && x.a.word !== x.b.word && !special(x.a.word) && !special(x.b.word) && !this.checked.has(pairKey(x.a.word, x.b.word)) && (x.l.tent != null || x.l.ne))
        .map(x => ({ ...x, sim: this.alike(x.a, x.b, pred) })).sort((p, q) => q.sim - p.sim);
      for (const { a, b, sim } of lines) {
        if (out.length >= k) break;
        const key = pairKey(a.word, b.word);
        if (asked.has(key)) continue;
        const [from, to] = this.rand() < 0.5 ? [a, b] : [b, a];
        const hosts = this.sentences.map(st => this.chunk(st)).filter(st => st.includes(from.word) && !st.includes(to.word));
        if (!hosts.length) continue;
        const host = hosts[Math.floor(this.rand() * hosts.length)];
        asked.add(key);
        out.push({ a: from.word, b: to.word, alike: +sim.toFixed(2), original: host.join(" "), swapped: host.map(w => w === from.word ? to.word : w).join(" ") });
      }
      return out;
    }
    // What the check said. ok: the line becomes a settled line (and the swapped sentence becomes practice);
    // false: the line is cut and never proposed again; null: no answer (stays tentative, not asked again for now).
    applyVerdicts(verdicts) {
      this.checked = this.checked || new Set(); this.banned = this.banned || new Set();
      for (const v of verdicts || []) {
        if (v.a === v.b) continue;                                       // a word and itself: nothing to check
        const key = pairKey(v.a, v.b);
        this.checked.add(key);
        if (v.ok == null) continue;
        for (const x of this.N.filter(n => n.word === v.a)) for (const y of this.N.filter(n => n.word === v.b)) {
          const l = this.link(x, y);
          const level = v.level || "role", w = level === "meaning" ? 0.7 : 0.5;   // same meaning binds harder than same role
          if (v.ok) { if (l) { l.w = l.w0 = Math.max(l.w, w); l.tent = null; l.proved = level; } else { this.connect(x, y, w); this.link(x, y).proved = level; } }
          else if (l) this.links.delete(this.key(x, y));
        }
        if (v.ok === false) this.banned.add(key);
        this.say(v.ok ? `the check agreed: ${v.a}—${v.b} (${v.why || "same role"})` : `the compiler disagreed: ${v.a}—${v.b} cut (${v.why || "different role"})`);
      }
      this.predCache = null;
    }
    // Roles (what a word does in a line: do, thing, place, colour, time, ...). Words brick reads come with their role
    // from brick's reader; a word it doesn't know gets the role its settled lines vote for.
    roleOf(n) {
      if (n.role) return { role: n.role, from: "brick's reader" };
      const slot = this.linksOf(n).map(l => this.get(l.a === n.id ? l.b : l.a)).find(o => o && o.word[0] === "@");
      if (slot) return { role: slot.word.slice(1), from: "its slot neuron" };
      const vote = {};
      for (const l of this.linksOf(n)) { if (l.tent != null) continue; const o = this.get(l.a === n.id ? l.b : l.a); if (o && o.role && o.word !== n.word) vote[o.role] = (vote[o.role] || 0) + l.w * (l.proved ? 2 : 1); }
      const best = Object.entries(vote).sort((a, b) => b[1] - a[1])[0];
      return best ? { role: best[0], from: "learned from its lines" } : null;
    }

    // New words from new input join as fresh neurons (random frequency, no lines): it learns them by itself.
    addWords(words) {
      const have = new Set(this.N.map(n => n.word));
      const band = Math.max(4, this.N.length / 25);
      for (const w of words) if (!have.has(w)) { have.add(w); const v = numberValue(w), out = w[0] === "=";
        const enc = this.encoded && this.encoded[w];        // the encoder's frequency, when it has one
        this.add(v != null ? { label: w, omega: numberOmega(v), fixed: true } : { label: w, omega: enc != null ? enc : 2 + this.rand() * band, fixed: out }); }
    }

    fission(a, sp) {
      const base = a.word;
      this.senseCount = this.senseCount || {};
      const mk = (part, mean, side) => {
        const suffix = this.senseCount[base] = (this.senseCount[base] || 0) + 1;
        const child = this.add({ label: `${base}·${suffix}`, word: base, omega: mean, x: a.x + side * 0.05, y: a.y + 0.05 });
        child.T = a.T; child.amp = a.amp; child.theta = a.theta; child.parentLabel = a.label; child.truth = a.truth;
        child.next = new Map(a.next);
        for (const x of this.N) if (x.next.has(a.id)) x.next.set(child.id, x.next.get(a.id));
        for (const { s } of part) { const old = this.link(a, s); this.connect(child, s, old ? old.w : 0.3); }
        return child;
      };
      const c1 = mk(sp.lo, sp.loMean, -1), c2 = mk(sp.hi, sp.hiMean, 1);
      this.predCache = null; this.rarity = null;                        // the children took over the parent's next maps
      // keep any other links of the parent on the closer child
      for (const l of this.linksOf(a)) {
        const other = this.get(l.a === a.id ? l.b : l.a);
        if (this.link(c1, other) || this.link(c2, other)) continue;
        const near = Math.abs(other.omega - c1.omega) < Math.abs(other.omega - c2.omega) ? c1 : c2;
        this.connect(near, other, l.w);
      }
      this.remove(a);
      const names = xs => xs.slice().sort((p, q) => q.v - p.v).slice(0, 3).map(x => x.s.label).join(", ");
      this.say(`nuclear fission: ${a.label} held two meanings, so it split into ${c1.label} (near ${names(sp.lo)}) and ${c2.label} (near ${names(sp.hi)})`);
    }

    fusion(a, b) {
      a.omega = a.omega0 = (a.omega + b.omega) / 2;
      for (const l of this.linksOf(b)) { const other = this.get(l.a === b.id ? l.b : l.a); if (other !== a && !this.link(a, other)) this.connect(a, other, l.w); }
      this.remove(b);
      this.say(`nuclear fusion: ${b.label} merged into ${a.label} (electric was too weak to keep them apart)`);
    }

    remove(a) {
      for (const [k, l] of this.links) if (l.a === a.id || l.b === a.id) this.links.delete(k);
      this.N = this.N.filter(x => x !== a); this.byIdStale = true;
      for (const x of this.N) { x.inbox.delete(a.id); x.co.delete(a.id); x.next.delete(a.id); }
      this.predCache = null; this.rarity = null;                        // gone, and every next map changed
    }

    // One training moment: a random sentence from the spec, felt as impacts on its words.
    // replay (a decision): some practice goes to a sentence holding the line closest to fading, like sleep.
    train() {
      if (!this.sentences.length) return null;
      let s = null;
      if (this.d.replay && this.rand() < this.d.replay) {
        let worst = null;
        for (const l of this.links.values()) if (l.idle > this.d.patience * 0.25 && (!worst || l.idle > worst.idle)) worst = l;
        if (worst) {
          const a = this.get(worst.a), b = this.get(worst.b);
          const both = this.sentences.filter(x => x.includes(a.word) && x.includes(b.word));
          const one = both.length ? both : this.sentences.filter(x => x.includes(a.word) || x.includes(b.word));
          if (one.length) s = one[Math.floor(this.rand() * one.length)];
        }
      }
      if (!s && this.intuition && this.lastSentence && this.rand() < 0.2 * this.fnPower("Si")) {   // Si: recall a similar past moment
        const like = this.sentences.filter(x => x !== this.lastSentence && x.some(w => this.lastSentence.includes(w)));
        if (like.length) s = like[Math.floor(this.rand() * like.length)];
      }
      s = s || this.sentences[Math.floor(this.rand() * this.sentences.length)];
      const out = s.out;
      if (this.intuition) { s = this.chunk(s); if (out) s.out = out; }                     // phrase neurons take their words
      this.stimulate(s, this.intuition ? 1.2 * Math.min(1.5, this.fnPower("Se")) : 1.2);   // Se: take it in
      if (this.intuition) {
        this.intuit(s);
        if (s.out) this.learnBits(s);
        // Ni and Ne can read faster than the physics runs: with batch > 1 they also read that many more sentences
        // per training moment (no impacts, only guessing and proposing), so big inputs get read in time.
        for (let k = 1; k < (this.batch || 1); k++) {
          const raw = this.sentences[Math.floor(this.rand() * this.sentences.length)], c = this.chunk(raw);
          if (raw.out) c.out = raw.out;
          this.intuit(c); if (c.out) this.learnBits(c);
        }
      }
      this.lastSentence = s;
      return s;
    }

    // Which sense of a word wins in a context, and whether it's relevant at all. For each of the 16 energies, the
    // context's energy of that kind is shared out among every neuron it reaches: a neuron's percentage is its share.
    // A sense matches on an energy when it gets more than a fair share (100% / the neurons reached). The meaning is
    // accurate when it matches on more than half the energies that carried anything; partly at a quarter; else not relevant.
    read(word, context, steps = 40) {
      const senses = this.N.filter(x => x.word === word);
      if (!senses.length) return null;
      const ctx = new Set(context), keys = Object.keys(PROOF);
      const sums = new Map();
      const add = (x, k, v) => { if (!(v > 0)) return; let t = sums.get(x.id); if (!t) sums.set(x.id, t = {}); t[k] = (t[k] || 0) + v; };
      const isCtx = n => ctx.has(n.word);
      for (let k = 0; k < steps; k++) {
        if (k % 8 === 0) { this.stimulate(context, 1.0); this.stimulate([word], 0.4); }
        this.step();
        const cx = this.N.filter(isCtx);
        if (!cx.length) continue;
        const ctxOmega0 = cx.reduce((t, n) => t + n.omega0, 0) / cx.length, dt = C.dt;
        // big networks: only neurons the context can reach (near it in frequency, linked, or sent something this step)
        const pool = this.N.length <= 120 ? this.N : this.N.filter(r => !isCtx(r) && (senses.includes(r) || r.stepIn.size || r.stepLit.size ||
          cx.some(o => Math.abs(o.omega - r.omega) < 1.8 * C.SIGMA || this.link(r, o))));
        // how close every pair in the pool is (frequency similarity), worked out once per step instead of once per
        // pair per neuron: the same formula and the same adding order, so the results are bit for bit the same
        const ctxIds = new Set(cx.map(n => n.id)), P = pool.length;
        if (!this.readBuf || this.readBuf.length < P * P) this.readBuf = new Float64Array(P * P);
        const F = this.readBuf, skip = new Uint8Array(P);
        for (let i = 0; i < P; i++) {
          skip[i] = ctxIds.has(pool[i].id) ? 1 : 0;
          const oi = pool[i].omega;
          for (let j = i + 1; j < P; j++) { const v = Math.exp(-(((oi - pool[j].omega) / C.SIGMA) ** 2)); F[i * P + j] = v; F[j * P + i] = v; }
        }
        for (let ri = 0; ri < P; ri++) {
          const r = pool[ri];
          if (skip[ri]) continue;
          const take = m => { let v = 0; for (const [id, a] of m) { if (ctxIds.has(id)) v += a; } return v; };
          const imp = take(r.stepIn), allIn = [...r.stepIn.values()].reduce((t, v) => t + v, 0);
          add(r, "kinetic", imp); add(r, "mechanical", imp * C.ABSORB);
          add(r, "heat", take(r.stepHeat)); add(r, "thermal", take(r.stepThermal)); add(r, "radiant", take(r.stepLit));
          add(r, "potential", allIn > 0 ? r.E * imp / allIn * dt : 0);
          add(r, "work", take(r.inbox) * dt);
          add(r, "chemical", Math.exp(-(((r.omega0 - ctxOmega0) / C.SIGMA) ** 2)) * dt);
          let fsAll = 0;
          for (let j = 0; j < P; j++) if (j !== ri && !skip[j]) fsAll += F[ri * P + j];
          for (const o of cx) {
            const sy = this.sync(r, o), l = this.link(r, o);
            add(r, "rest", sy.s * dt); add(r, "magnetic", sy.fsim * dt);
            add(r, "gravity", (l ? l.w : 0) * dt); add(r, "elastic", (l ? l.w0 : 0) * dt);
            add(r, "nuclear", (r.co.get(o.id) || 0) * dt);
            add(r, "electric", r.E * sy.s * dt);
            add(r, "oscillation", sy.s * Math.min(r.amp, o.amp) * dt);
            add(r, "ionization", sy.fsim / (sy.fsim + fsAll + 1e-9) * dt);
          }
        }
      }
      return this.verdictFrom(senses, sums, keys);
    }

    // The 16-percentage verdict from what each neuron took in from the context (read and readLine share it).
    verdictFrom(senses, sums, keys) {
      const alive = senses.filter(x => this.N.includes(x));
      if (!alive.length) return null;
      const profile = new Map(alive.map(x => [x.id, {}])), matched = new Map(alive.map(x => [x.id, 0]));
      let carried = 0;
      for (const k of keys) {
        if (this.e[k] <= 0) continue;
        let total = 0, reached = 0;
        for (const [, t] of sums) if (t[k] > 0) { total += t[k]; reached++; }
        if (total <= 1e-12) continue;
        carried++;
        const fair = 100 / reached;
        for (const x of alive) {
          const pct = 100 * ((sums.get(x.id) || {})[k] || 0) / total;
          profile.get(x.id)[k] = pct;
          if (pct > fair) matched.set(x.id, matched.get(x.id) + 1);
        }
      }
      const avg = x => { const v = Object.values(profile.get(x.id)); return v.length ? v.reduce((a, b) => a + b, 0) / v.length : 0; };
      const ranked = alive.map(x => ({ s: x, matched: matched.get(x.id), pct: avg(x), profile: profile.get(x.id) }))
        .sort((a, b) => b.matched - a.matched || b.pct - a.pct);
      const best = ranked[0];
      const bar = this.d.strict;                                         // the archetype's strictness (neutral 0.5)
      const verdict = !carried ? "not relevant" : best.matched > carried * bar ? "accurate" : best.matched >= carried * bar / 2 ? "partly" : "not relevant";
      const clear = ranked.length < 2 || best.matched > ranked[1].matched;
      return { winner: best.s, senses: ranked.length, ranked, carried, verdict, clear,
               meaning: verdict === "accurate" && clear ? this.meaningOf(best.s) : null };
    }

    // Every word of a line read in one run: the whole line is felt together, and each word's senses are judged
    // only by what reached them from the other words (its context), with the same 16 energies and the same
    // verdict as read(). One simulation for the line instead of one per word.
    readLine(words, steps = 40) {
      words = [...new Set(words)].filter(w => this.N.some(n => n.word === w));
      const keys = Object.keys(PROOF), W = words.length, wordOf = new Map(words.map((w, i) => [w, i])), dt = C.dt;
      const sumsW = words.map(() => new Map());
      const add = (m, id, k, v) => { if (!(v > 0)) return; let t = m.get(id); if (!t) m.set(id, t = {}); t[k] = (t[k] || 0) + v; };
      const PAIR = ["rest", "magnetic", "gravity", "elastic", "nuclear", "electric", "oscillation"];
      for (let step = 0; step < steps; step++) {
        if (step % 8 === 0) this.stimulate(words, 1.0);
        this.step();
        const cx = this.N.filter(n => wordOf.has(n.word));
        if (W < 2 || !cx.length) continue;
        const cxWord = cx.map(n => wordOf.get(n.word)), idWord = new Map(cx.map((n, i) => [n.id, cxWord[i]]));
        const pool = this.N.filter(r => !wordOf.has(r.word) && (this.N.length <= 120 || r.stepIn.size || r.stepLit.size ||
          cx.some(o => Math.abs(o.omega - r.omega) < 1.8 * C.SIGMA || this.link(r, o))));
        const all = pool.concat(cx), A = all.length, P = pool.length;
        const fs = (a, b) => Math.exp(-(((a.omega - b.omega) / C.SIGMA) ** 2));
        // closeness to everything outside the line: only neurons within 5.3 sigma count (further ones add < 1e-12)
        const sorted = pool.map((n, j) => [n.omega, j]).sort((a, b) => a[0] - b[0]), reach = 5.3 * C.SIGMA;
        const sortedOm = sorted.map(x => x[0]);
        const lower = x => { let lo = 0, hi = sortedOm.length; while (lo < hi) { const mid = (lo + hi) >> 1; if (sortedOm[mid] < x) lo = mid + 1; else hi = mid; } return lo; };
        // per neuron: what came from each word, kept apart so any one word can be left out
        const per = all.map((r, ri) => {
          const byWord = (m, scale = 1) => { const v = new Float64Array(W); for (const [id, a] of m) { const i = idWord.get(id); if (i !== undefined) v[i] += a * scale; } return v; };
          const pair = PAIR.map(() => new Float64Array(W)), fsimO = new Float64Array(cx.length);
          cx.forEach((o, oi) => {
            if (o === r) return;
            const sy = this.sync(r, o), l = this.link(r, o), i = cxWord[oi];
            fsimO[oi] = sy.fsim;
            pair[0][i] += sy.s * dt; pair[1][i] += sy.fsim * dt; pair[2][i] += (l ? l.w : 0) * dt; pair[3][i] += (l ? l.w0 : 0) * dt;
            pair[4][i] += (r.co.get(o.id) || 0) * dt; pair[5][i] += r.E * sy.s * dt; pair[6][i] += sy.s * Math.min(r.amp, o.amp) * dt;
          });
          let base = 0;                                                  // closeness to every neuron outside the line
          for (let q = lower(r.omega - reach); q < P && sortedOm[q] <= r.omega + reach; q++) { const j = sorted[q][1]; if (j !== ri) base += fs(r, pool[j]); }
          const tot = v => { let t = 0; for (let i = 0; i < W; i++) t += v[i]; return t; };
          const imp = byWord(r.stepIn), heat = byWord(r.stepHeat), thermal = byWord(r.stepThermal), lit = byWord(r.stepLit), work = byWord(r.inbox, dt);
          return { imp, heat, thermal, lit, work, allIn: [...r.stepIn.values()].reduce((t, v) => t + v, 0), pair, fsimO, base,
                   T: { imp: tot(imp), heat: tot(heat), thermal: tot(thermal), lit: tot(lit), work: tot(work), pair: pair.map(tot) } };
        });
        const but = (t, v, w) => Math.max(0, t - v[w]);                 // everything but this word's own share
        const lineIdx = new Map(cx.map((n, i) => [n.id, i]));
        for (let w = 0; w < W; w++) {
          const ctxN = cx.filter((n, i) => cxWord[i] !== w);
          if (!ctxN.length) continue;
          const mean0 = ctxN.reduce((t, n) => t + n.omega0, 0) / ctxN.length, own = cx.filter((n, i) => cxWord[i] === w);
          for (let ri = 0; ri < A; ri++) {
            const r = all[ri];
            if (ri >= P && cxWord[ri - P] !== w) continue;               // judged: outside the line, or this word's own senses
            const p = per[ri], m = sumsW[w], imp = but(p.T.imp, p.imp, w);
            add(m, r.id, "kinetic", imp); add(m, r.id, "mechanical", imp * C.ABSORB);
            add(m, r.id, "heat", but(p.T.heat, p.heat, w)); add(m, r.id, "thermal", but(p.T.thermal, p.thermal, w)); add(m, r.id, "radiant", but(p.T.lit, p.lit, w));
            add(m, r.id, "potential", p.allIn > 0 ? r.E * imp / p.allIn * dt : 0);
            add(m, r.id, "work", but(p.T.work, p.work, w));
            add(m, r.id, "chemical", Math.exp(-(((r.omega0 - mean0) / C.SIGMA) ** 2)) * dt);
            for (let ki = 0; ki < PAIR.length; ki++) add(m, r.id, PAIR[ki], but(p.T.pair[ki], p.pair[ki], w));
            let fsAll = p.base;                                          // this word's own senses count as outside too
            for (const s of own) if (s !== r) fsAll += fs(r, s);
            let ion = 0;
            cx.forEach((o, oi) => { if (cxWord[oi] !== w && o !== r) ion += p.fsimO[oi] / (p.fsimO[oi] + fsAll + 1e-9) * dt; });
            add(m, r.id, "ionization", ion);
          }
        }
      }
      return words.map((w, i) => ({ word: w, result: this.verdictFrom(this.N.filter(x => x.word === w), sumsW[i], keys) }));
    }

    fromContext(s, ctx) { let t = 0; for (const [id, v] of (s.stepIn || new Map())) { const src = this.get(id); if (src && (ctx.has(src.word) || this.isNear(src, ctx))) t += v; } return t; }
    isNear(src, ctx) { for (const l of this.linksOf(src)) { const o = this.get(l.a === src.id ? l.b : l.a); if (o && ctx.has(o.word)) return true; } return false; }

    // The group a neuron's linked neighbours mostly belong to (for display and scoring only; the dynamics never read it).
    meaningOf(n) {
      if (n.group) return n.group;                                       // a word that never split keeps its own group
      const count = {};                                                  // raw input: named by its neighbours' groups
      for (const l of this.linksOf(n)) { const o = this.get(l.a === n.id ? l.b : l.a); const g = o && (o.group || o.truth); if (g && o.word !== n.word && l.tent == null) count[g] = (count[g] || 0) + l.w; }
      const best = Object.entries(count).sort((a, b) => b[1] - a[1])[0];
      if (best) return best[0];
      const near = this.neighbours(n).slice(0, 2);                     // no groups given: name it by its strongest lines
      return near.length ? "near " + near.join(", ") : null;
    }

    neighbours(n) {
      return this.linksOf(n).map(l => ({ o: this.get(l.a === n.id ? l.b : l.a), w: l.w })).filter(x => x.o && x.o.word !== n.word)
        .sort((a, b) => b.w - a.w).map(x => x.o.word);
    }

    // Does an answer match what the person expected? expected: a group name, a word the sense should be linked to,
    // or "not relevant".
    matches(r, want) {
      if (!want) return null;
      if (!r) return false;
      if (/^(not relevant|none|no)$/.test(want)) return r.verdict !== "accurate";
      if (r.verdict !== "accurate" || !r.clear) return false;
      if (this.N.some(x => (x.group || x.truth) === want)) return this.meaningOf(r.winner) === want;
      return this.neighbours(r.winner).slice(0, 4).includes(want);
    }

    // ---------- a local sub-network: the part of the brain one moment touches, run on its own ----------
    // The neurons of `words` (every sense), plus the neurons most strongly coupled to them, out to `hops` hops and at
    // most `cap` neurons in all. Coupling is what step() itself uses: line weight plus the magnetic field of near
    // frequencies (a proved "used alike" line counts extra, since speaking reads those lines), and Ni's predictions
    // both ways. `also`: words whose neurons are kept in too, without their neighbours (chatter, read only for heat).
    // The result is a real Network with the same neuron ids, state (frequency, rhythm, heat, light, predictions,
    // what was used together, outputs), lines, clock and phrases, and the archetype's brain (opts.brain, default this
    // network's). Its dynamics are this network's own; neurons left outside count as they are now: in how warm and
    // how ordered the personality feels the whole network to be (steer), and in Ni's tables (predIndex, rarity).
    // This network is only read, never changed.
    local(words, opts = {}) {
      const brain = "brain" in opts ? (typeof opts.brain === "string" ? findArchetype(opts.brain) : opts.brain) : this.brain;
      const lean = brain ? brain.personality.energies : this.e;           // the field as the speaking brain feels it
      const cap = Math.max(1, opts.cap || 200), hops = opts.hops ?? 2, mag = C.FIELD * lean.magnetic, reach = 1.8 * C.SIGMA;
      const want = new Set(words.map(w => String(w).toLowerCase())), also = new Set((opts.also || []).map(w => String(w).toLowerCase()));
      const seeds = this.N.filter(n => want.has(n.word)), extra = this.N.filter(n => also.has(n.word) && !want.has(n.word));
      const pred = this.predIndex(), byOmega = this.N.slice().sort((a, b) => a.omega - b.omega);
      const inside = new Set([...seeds, ...extra].map(n => n.id)), score = new Map();
      let frontier = seeds.map(n => [n, 1]);
      for (let h = 0; h < hops && frontier.length; h++) {
        const found = new Map();
        const offer = (o, v) => { if (o && !inside.has(o.id) && v > (found.get(o.id) || 0)) found.set(o.id, v); };
        for (const [x, sx] of frontier) {
          for (const l of this.linksOf(x)) { const o = this.get(l.a === x.id ? l.b : l.a); if (o) offer(o, sx * (l.w + mag * this.sync(x, o).fsim + (l.proved ? 1 : 0))); }
          for (const [id, v] of x.next) offer(this.get(id), sx * v);
          for (const [id, v] of pred.get(x.id) || []) offer(this.get(id), sx * v);
          let lo = 0, hi = byOmega.length;                                 // magnetic: near frequencies, no line needed
          while (lo < hi) { const m = (lo + hi) >> 1; if (byOmega[m].omega < x.omega - reach) lo = m + 1; else hi = m; }
          for (let k = lo; k < byOmega.length && byOmega[k].omega < x.omega + reach; k++) if (byOmega[k] !== x) offer(byOmega[k], sx * mag * this.sync(x, byOmega[k]).fsim);
        }
        for (const [id, v] of found) if (v > (score.get(id) || 0)) score.set(id, v);
        frontier = [...found].sort((p, q) => q[1] - p[1]).slice(0, cap).map(([id, v]) => [this.get(id), v]);
      }
      for (const [id] of [...score].sort((p, q) => q[1] - p[1])) { if (inside.size >= cap) break; inside.add(id); }
      const sub = new Network({ neurons: [] }, opts.seed ?? 1, null, brain, { intuition: this.intuition });
      for (const n of this.N) {                                          // in this network's order: step() works in order
        if (!inside.has(n.id)) continue;
        const x = sub.add({ label: n.label, word: n.word, group: n.group, truth: n.truth, omega: n.omega, fixed: n.fixed, seen: n.seen, hidden: n.hidden, x: n.x, y: n.y });
        Object.assign(x, { id: n.id, omega0: n.omega0, amp: n.amp, E: n.E, T: n.T, lit: n.lit, litBy: inside.has(n.litBy) ? n.litBy : null, role: n.role || null, refined: n.refined });
        if (n.phrase) { x.phrase = true; x.parts = n.parts; }
        x.next = new Map(n.next); x.out = new Map(n.out);
        for (const [k, v] of n.co) if (inside.has(k)) x.co.set(k, v);
        for (const [k, v] of n.inbox) if (inside.has(k)) x.inbox.set(k, v);
      }
      sub.nextId = this.nextId; sub.byIdStale = true;
      // how ordered the lines left outside are (all lines, worked out once while this network stays as it is, minus
      // the lines that touch the inside); lines from inside to a neuron left outside are worked out as it runs
      const stamp = this.links.size + "|" + this.linkVersion + "|" + this.N.length + "|" + this.steps;
      if (!this._order || this._order.stamp !== stamp) {
        let s = 0, c = 0;
        for (const l of this.links.values()) { const a = this.get(l.a), b = this.get(l.b); if (a && b) { s += this.sync(a, b).s; c++; } }
        this._order = { stamp, s, c };
      }
      let outLinks = this._order.c, outOrder = this._order.s;
      const edge = [];
      for (const l of this.links.values()) {                             // in this network's order
        const ia = inside.has(l.a), ib = inside.has(l.b);
        if (!ia && !ib) continue;
        const a = this.get(l.a), b = this.get(l.b);
        if (!a || !b) continue;
        outOrder -= this.sync(a, b).s; outLinks--;
        if (ia && ib) sub.links.set(sub.key(a, b), Object.assign({}, l)); else edge.push(ia ? [l.a, b] : [l.b, a]);
      }
      sub.linkVersion = (sub.linkVersion || 0) + 1;
      Object.assign(sub, { t: this.t, steps: this.steps, phrases: new Set(this.phrases || []), banned: new Set(this.banned || []),
        checked: new Set(this.checked || []), senseCount: Object.assign({}, this.senseCount || {}), sentences: this.sentences, encoded: this.encoded });
      let outT = 0, outN = 0;
      for (const n of this.N) if (!inside.has(n.id)) { outT += n.T; outN++; }
      const full = this;
      sub.heatNow = function () { return (outT + this.N.reduce((t, n) => t + n.T, 0)) / Math.max(1, outN + this.N.length); };
      sub.orderNow = function () {
        let s = outOrder, c = outLinks;
        for (const l of this.links.values()) { const a = this.get(l.a), b = this.get(l.b); if (a && b) { s += this.sync(a, b).s; c++; } }
        for (const [id, o] of edge) { const a = this.get(id); if (a) { s += this.sync(a, o).s; c++; } }
        return c ? s / c : 0;
      };
      sub.predIndex = () => full.predIndex();
      sub.rarityOf = () => full.rarity || (full.rarity = full.rarityOf());
      return sub;
    }

    // Overall readings for the page.
    // ---------- speaking: the network says a meaning in its own words (the archetype's way) ----------
    // pieces: the meaning in pieces (["write"], ["hello there"], ["in", "red"], ...), each word {text, keep}: kept
    // words carry the meaning (words to show, colours, numbers, places, names, files) and are always said as they are.
    // The meaning's words are stimulated and the network runs its own dynamics. Then:
    //   order (Ni): each next piece is the one the network predicts after the last word said, weighed by how hot it is;
    //     a judging archetype (strict) keeps closer to the order it was given;
    //   swaps (Ne, radiant): a free word is said as a word it is used alike with, when that one ran hotter;
    //   economy (Te, Se): a free small word that stayed cold is left out;
    //   chatter (Fe, Fi): the hottest chatter neuron may be said before or after, more for warm (extraverted) types;
    //   slips: heat makes the odd letter swap in a free word.
    //   It runs on a local sub-network (see local()): the stimulated neurons and the neurons most strongly coupled to
    //   them, with this archetype's brain, so a meaning takes a fraction of a second. opts.local = false: run it on
    //   this whole network instead (slow, and it changes this network). opts.cap: how many neurons (default 200).
    speak(pieces, opts = {}) {
      if (opts.local !== false) {
        const brain = "brain" in opts ? opts.brain : this.brain;
        const sub = this.local(pieces.flat().map(t => t.text), { cap: opts.cap, hops: opts.hops, also: opts.chatter, seed: opts.seed, brain });
        return sub.speak(pieces, Object.assign({}, opts, { local: false }));
      }
      const e = this.e, r = this.rand, steps = opts.steps || 30, small = new Set(opts.small || []), chatter = opts.chatter || [];
      const byWord = new Map();
      for (const n of this.N) if (!n.hidden) { const k = n.word; if (!byWord.has(k) || n.T > byWord.get(k).T) byWord.set(k, n); }
      const neuron = w => byWord.get(String(w).toLowerCase()) || null;
      const words = pieces.flat().map(t => t.text.toLowerCase());
      this.stimulate(words, 1.5);
      for (let k = 0; k < steps; k++) this.step();
      for (const n of this.N) if (!n.hidden) { const k = n.word; if (!byWord.has(k) || n.T > byWord.get(k).T) byWord.set(k, n); }
      const heat = w => { const n = neuron(w); return n ? n.T + 0.5 * n.lit : 0.2; };
      const radiant = this.fnPower("Ne"), te = this.fnPower("Te"), fe = this.fnPower("Fe"), fi = this.fnPower("Fi");
      const warm = this.brain ? this.brain.personality.target.heat : 0.85;
      const said = t => {
        if (t.keep) return t.text;
        const n = neuron(t.text);
        if (!n) return t.text;
        let best = null, bestT = n.T;
        for (const l of this.linksOf(n)) {
          const o = this.get(l.a === n.id ? l.b : l.a);
          if (!o || o.word === n.word || o.hidden || /[=@#·]/.test(o.word) || o.word.includes(" ")) continue;
          const alike = l.kind === "role" || l.proved;
          if (alike && o.T * (0.6 + 0.4 * radiant) * l.w > bestT) { best = o; bestT = o.T * l.w; }
        }
        let w = best && !small.has(t.text.toLowerCase()) ? best.word : t.text;
        if (w.length > 3 && r() < 0.03 * Math.min(1.5, n.T) * warm) { const i = 1 + Math.floor(r() * (w.length - 2)); w = w.slice(0, i - 1) + w[i] + w[i - 1] + w.slice(i + 1); }
        return w;
      };
      const kept = piece => piece.filter(t => t.keep || !small.has(t.text.toLowerCase()) || heat(t.text) > C.T_COLD * (0.6 + 0.6 * te));
      const left = pieces.map((p, i) => i), out = [];
      let last = null;
      const strict = this.d.strict || 0.5;
      while (left.length) {
        let pick = left[0], top = -1e9;
        for (const i of left) {
          const first = pieces[i][0], n = neuron(first.text);
          const ni = last && n ? (last.next.get(n.id) || 0) : 0;
          const s = heat(first.text) + 1.5 * ni + (i === left[0] ? 2.5 * strict : 0) + 0.6 * (r() - 0.5) * warm;
          if (s > top) { top = s; pick = i; }
        }
        left.splice(left.indexOf(pick), 1);
        const words = kept(pieces[pick]).map(said);
        if (words.length) out.push(words.join(" "));
        const lw = pieces[pick][pieces[pick].length - 1];
        last = neuron(lw.text) || last;
      }
      if (chatter.length && r() < 0.15 + 0.25 * (fe + fi) / 2 * warm) {
        const hot = chatter.map(w => [w, heat(w) + 0.3 * r()]).sort((a, b) => b[1] - a[1])[0][0];
        if (r() < 0.6) out.unshift(hot); else out.push(hot);
      }
      return out.join(" ");
    }

    stats() {
      const N = this.N;
      let re = 0, im = 0; for (const n of N) { re += Math.cos(n.theta); im += Math.sin(n.theta); }
      const order = Math.hypot(re, im) / Math.max(1, N.length);
      const heat = N.reduce((t, n) => t + n.T, 0);
      const hot = N.filter(n => n.T > C.T_HOT).length, cold = N.filter(n => n.T < C.T_COLD).length;
      return { order, heat, hot, cold, neurons: N.length, links: this.links.size, lit: N.filter(n => n.lit > 0.3).length };
    }
  }

  // The word preset: four meaning groups and two ambiguous words. Frequencies are set by hand: there is no
  // cortex engine yet to turn a word into a frequency.
  function wordSpec(gap = 1.2) {
    const g = { remove: 2.0, show: 2.0 + gap, check: 2.0 + 2 * gap, place: 2.0 + 3 * gap };
    const groups = {
      remove: ["clear", "wipe", "remove", "junk"],
      show: ["write", "show", "text", "quote"],
      place: ["window", "put", "place", "position"],
      check: ["check", "if", "compare", "equals"],
    };
    const pos = { remove: [0.16, 0.3], show: [0.5, 0.2], check: [0.7, 0.68], place: [0.26, 0.76] };
    const neurons = [];
    for (const [grp, words] of Object.entries(groups)) words.forEach((w, k) => {
      const [cx, cy] = pos[grp], ang = (k / words.length) * Math.PI * 2;
      neurons.push({ label: w, group: grp, omega: g[grp] + (k - 1.5) * 0.12, x: cx + 0.11 * Math.cos(ang), y: cy + 0.11 * Math.sin(ang) });
    });
    neurons.push({ label: "crap", omega: (g.remove + g.show) / 2, x: 0.4, y: 0.25 });
    neurons.push({ label: "test", omega: (g.show + g.check) / 2 + 0.05, x: 0.46, y: 0.5 });
    neurons.push({ label: "zorp", omega: g.place + 0.35, x: 0.92, y: 0.42, hidden: true });
    const links = [];
    for (const words of Object.values(groups)) for (let i = 0; i < words.length; i++) for (let j = i + 1; j < words.length; j++) links.push([words[i], words[j], 0.5]);
    for (const w of ["clear", "wipe", "junk", "write", "text", "quote"]) links.push(["crap", w, 0.4]);
    for (const w of ["text", "write", "check", "compare"]) links.push(["test", w, 0.4]);
    const sentences = [
      ["clear", "crap"], ["wipe", "crap"], ["crap", "junk"], ["remove", "crap"],
      ["write", "crap"], ["show", "crap"], ["crap", "text"], ["quote", "crap"],
      ["write", "test"], ["test", "text"], ["check", "test"], ["if", "test"], ["compare", "test"],
      ["put", "window"], ["place", "position"], ["clear", "junk"], ["write", "text"], ["check", "equals"],
    ];
    return { neurons, links, sentences, groups: g, truth: { crap: { clear: "remove", wipe: "remove", junk: "remove", remove: "remove", write: "show", show: "show", text: "show", quote: "show" }, test: { write: "show", text: "show", show: "show", check: "check", if: "check", compare: "check", equals: "check" } } };
  }


  // ---------- the brain system: archetype -> personality (MBTI) -> the 16 energies ----------
  // Each of the 8 functions owns 2 energies (the owner's 5 links, the rest proposed and still to confirm).
  const FUNCTIONS = {
    Ni: ["mechanical", "work"], Ne: ["radiant", "oscillation"], Ti: ["nuclear", "gravity"], Te: ["kinetic", "ionization"],
    Fi: ["electric", "chemical"], Fe: ["heat", "magnetic"], Si: ["rest", "potential"], Se: ["thermal", "elastic"],
  };
  const STACKS = {
    INTJ: ["Ni", "Te", "Fi", "Se"], INTP: ["Ti", "Ne", "Si", "Fe"], ENTJ: ["Te", "Ni", "Se", "Fi"], ENTP: ["Ne", "Ti", "Fe", "Si"],
    INFJ: ["Ni", "Fe", "Ti", "Se"], INFP: ["Fi", "Ne", "Si", "Te"], ENFJ: ["Fe", "Ni", "Se", "Ti"], ENFP: ["Ne", "Fi", "Te", "Si"],
    ISTJ: ["Si", "Te", "Fi", "Ne"], ISFJ: ["Si", "Fe", "Ti", "Ne"], ESTJ: ["Te", "Si", "Ne", "Fi"], ESFJ: ["Fe", "Si", "Ne", "Ti"],
    ISTP: ["Ti", "Se", "Ni", "Fe"], ISFP: ["Fi", "Se", "Ni", "Te"], ESTP: ["Se", "Ti", "Fe", "Ni"], ESFP: ["Se", "Fi", "Te", "Ni"],
  };
  const STACK_WEIGHT = [1.6, 1.3, 0.9, 0.6], SHADOW_WEIGHT = 0.5;

  // The personality: the 16 energies an MBTI type leans toward, by its function order.
  function personality(type) {
    const stack = STACKS[type], e = {};
    for (const [fn, keys] of Object.entries(FUNCTIONS)) {
      const at = stack.indexOf(fn), w = at >= 0 ? STACK_WEIGHT[at] : SHADOW_WEIGHT;
      for (const k of keys) e[k] = w;
    }
    // where it reactively steers the network: extraverts run warmer, judgers keep more order
    // (a neutral network runs at about 0.85 mean temperature and 0.80 sync along its lines)
    const target = { heat: +(0.75 + (type[0] === "E" ? 0.25 : 0) + (type[3] === "P" ? 0.1 : 0)).toFixed(2), order: type[3] === "J" ? 0.86 : 0.76 };
    return { type, stack, energies: e, target };
  }

  // How an archetype decides (determination), the same whether the network is young or mature.
  function decisions(type, bias) {
    const T = type[2] === "T", J = type[3] === "J", N = type[1] === "N", S = !N, F = !T, P = !J;
    const d = {
      strict: 0.5 + (T ? 0.08 : -0.04) + (J ? 0.07 : -0.03) + (bias === "strict" ? 0.1 : 0),      // share of energies a meaning must win
      split: 0.4 - (N ? 0.08 : 0) + (J ? 0.04 : -0.04) - (bias === "split" ? 0.12 : 0),          // lower = splits meanings sooner
      patience: Math.round(40 * (S ? 1.6 : 1) * (F ? 1.3 : 1) * (P ? 1.1 : 0.9) * (bias === "keep" ? 2 : 1)),   // seconds before an unused line fades
      radiant: 0.5 - (N ? 0.1 : 0) - (P ? 0.08 : 0) - (bias === "explore" ? 0.12 : 0),          // lower = joins hidden neurons sooner
      replay: +((S ? 0.25 : 0.05) + (F ? 0.1 : 0) + (bias === "keep" ? 0.2 : 0)).toFixed(2),     // share of practice spent on fading words
      react: bias === "swift" ? 0.08 : bias === "order" ? 0.03 : 0.05,                           // how fast the personality reacts
    };
    d.strict = +Math.min(0.8, d.strict).toFixed(2); d.split = +Math.max(0.15, d.split).toFixed(2); d.radiant = +Math.max(0.2, d.radiant).toFixed(2);
    return d;
  }

  // The neutral rules, used when there is no archetype (the values the network was first tuned with).
  const NEUTRAL = { strict: 0.5, split: 0.4, patience: 40, radiant: 0.5, replay: 0, react: 0 };

  function findArchetype(name) {
    const clean = x => String(x).toLowerCase().replace(/^the\s+/, "").replace(/[^a-z ]/g, "").trim();
    const want = clean(name);
    return ARCHETYPES.find(a => clean(a.name) === want) || ARCHETYPES.find(a => clean(a.name).includes(want)) || null;
  }

  // Archetypes. Two are the owner's (Mirror Oracle, the Sensory Mythweaver); the MBTI each one picks is a choice of
  // character, from popular psychology, not tested science.
  const ARCHETYPES = [
    ["The Mirror Oracle", "INFJ", "strict", "Sees itself in others and the future in both: it reflects every context back until the true meaning shows.", true],
    ["The Veiled Seer", "INFJ", "keep", "Holds quiet visions for a long time and lets nothing meaningful fade."],
    ["The Dream Keeper", "INFJ", "explore", "Walks the edges of sleep, lighting up meanings no one else has noticed."],
    ["The Sensory Mythweaver", "ISFP", "explore", "Weaves myths out of colour, texture and feeling: every sensation becomes a story.", true],
    ["The Colour Drifter", "ISFP", "keep", "Moves gently through hues and moods, keeping each one it has loved."],
    ["The Wild Gardener", "ISFP", "split", "Lets meanings grow in every direction, then prunes them by feel."],
    ["The Silent Architect", "INTJ", "strict", "Plans the whole structure before laying a single brick."],
    ["The Midnight Strategist", "INTJ", "order", "Works alone in the dark hours, ordering every possibility into one path."],
    ["The Crystal Cartographer", "INTJ", "split", "Maps meanings with cold precision, splitting each region cleanly."],
    ["The Paradox Engineer", "INTP", "split", "Takes every idea apart to see whether its opposite also works."],
    ["The Quiet Logician", "INTP", "strict", "Accepts nothing that isn't proven, and says so softly."],
    ["The Fractal Hermit", "INTP", "explore", "Follows patterns inward forever, finding worlds inside words."],
    ["The Iron Conductor", "ENTJ", "order", "Sets the tempo for the whole network and keeps it in line."],
    ["The Sovereign Engine", "ENTJ", "swift", "Decides fast, drives hard, and turns every impact into motion."],
    ["The Storm Marshal", "ENTJ", "strict", "Commands chaos into formation, keeping only what holds."],
    ["The Spark Trickster", "ENTP", "explore", "Throws sparks at every idea to see which ones catch fire."],
    ["The Idea Smith", "ENTP", "split", "Hammers one word into many shapes and tests each blade."],
    ["The Chaos Cartographer", "ENTP", "swift", "Charts what's changing while it changes."],
    ["The Ember Poet", "INFP", "keep", "Keeps small fires of meaning burning long after others have gone out."],
    ["The Wandering Lantern", "INFP", "explore", "Carries a soft light into forgotten corners."],
    ["The Soft Rebel", "INFP", "split", "Refuses the obvious meaning and finds the personal one."],
    ["The Warm Beacon", "ENFJ", "order", "Draws every neuron toward one warm centre."],
    ["The Chorus Shepherd", "ENFJ", "keep", "Keeps every voice in the choir, even the quietest."],
    ["The Sun Herald", "ENFJ", "swift", "Announces meaning the moment it rises."],
    ["The Wildfire Muse", "ENFP", "explore", "Sets imagination alight and follows wherever it spreads."],
    ["The Starlight Vagabond", "ENFP", "split", "Collects meanings like stars, one for every road."],
    ["The Kaleidoscope Child", "ENFP", "swift", "Turns every context and sees a new pattern each time."],
    ["The Stone Keeper", "ISTJ", "keep", "Remembers every brick ever laid, exactly where it was."],
    ["The Ledger Monk", "ISTJ", "strict", "Records only what balances, and checks it twice."],
    ["The Clockwork Sentinel", "ISTJ", "order", "Keeps perfect time and lets nothing slip out of rhythm."],
    ["The Hearth Guardian", "ISFJ", "keep", "Tends the fire so the oldest meanings stay warm."],
    ["The Gentle Archivist", "ISFJ", "order", "Files every memory with care so it can be found again."],
    ["The Lantern Nurse", "ISFJ", "strict", "Heals only what it understands, one careful step at a time."],
    ["The Steel Steward", "ESTJ", "order", "Runs the network like a well-kept workshop."],
    ["The Bridge Builder", "ESTJ", "swift", "Connects what should be connected, fast and solid."],
    ["The Order Captain", "ESTJ", "strict", "Keeps the ranks straight and the rules clear."],
    ["The Village Heart", "ESFJ", "keep", "Knows everyone's story and keeps them all together."],
    ["The Table Host", "ESFJ", "order", "Seats every meaning where it belongs."],
    ["The Golden Thread", "ESFJ", "swift", "Ties people and meanings together before they drift apart."],
    ["The Lone Mechanic", "ISTP", "strict", "Strips each brick to its working parts and rebuilds it right."],
    ["The Edge Walker", "ISTP", "explore", "Tests the limits by standing on them."],
    ["The Silent Blade", "ISTP", "split", "Cuts one meaning cleanly into two."],
    ["The Thunder Runner", "ESTP", "swift", "Acts first, learns from the impact, acts again."],
    ["The Street Alchemist", "ESTP", "explore", "Turns whatever is lying around into something that works."],
    ["The Daredevil Spark", "ESTP", "split", "Tries the risky meaning just to see it fly."],
    ["The Stage Flame", "ESFP", "swift", "Lights up the moment and plays to every impact."],
    ["The Festival Spirit", "ESFP", "explore", "Brings every neuron to the party."],
    ["The Velvet Performer", "ESFP", "keep", "Remembers how every moment felt, and replays the best ones."],
  ].map(([name, type, bias, line, yours]) => ({ name, type, bias, line, yours: !!yours, personality: personality(type), decisions: decisions(type, bias) }));

  // Your own data. Lines like "group remove: clear, wipe, junk" make a meaning group; every other line is a sentence.
  // Questions: "crap @ clear = remove" reads crap in the context "clear" and expects the remove meaning
  // (or a word it should be linked to, or "not relevant").
  function parseData(text) {
    const groups = {}, sentences = [];
    for (const raw of String(text).split(/\n/)) {
      const line = raw.replace(/^\s*#.*|\s#(\s.*)?$/, "").trim().toLowerCase();   // "# note" lines; "#2" is a number
      if (!line) continue;
      const g = line.match(/^group\s+([a-z0-9_-]+)\s*:\s*(.+)$/);
      if (g) { groups[g[1]] = (g[2].match(/[a-z0-9']+/g) || []); continue; }
      const [said, made] = line.split("||");                    // "words || =write =wait": what brick built from it
      const words = said.match(/[#~a-z0-9'.]+/g) || [];
      if (words.length) { if (made) words.out = made.match(/[=a-z0-9_:-]+/g) || []; sentences.push(words); }
    }
    return { groups, sentences };
  }

  function parseQuestions(text) {
    const out = [];
    for (const raw of String(text).split(/\n/)) {
      const line = raw.replace(/^\s*#.*|\s#(\s.*)?$/, "").trim().toLowerCase();   // "# note" lines; "#2" is a number
      const m = line.match(/^([a-z0-9']+)\s*@\s*([^=]+?)\s*(?:=\s*(.+))?$/);
      if (m) out.push({ word: m[1], context: m[2].match(/[a-z0-9']+/g) || [], want: m[3] ? m[3].trim() : null, text: line });
    }
    return out;
  }

  // mode: "groups" = your groups set the frequency bands, other words sit at the average of the words used with them;
  // "use" = a stand-in encoder that orders words by use only (ignores groups); "random" = no encoder at all.
  function dataSpec(text, mode = "groups", seed = 1) {
    const { groups, sentences } = parseData(text);
    const r = rng(seed * 7919 + 3), omega = {}, groupOf = {}, words = [];
    const seen = new Set(), addWord = w => { if (!seen.has(w)) { seen.add(w); words.push(w); } };
    for (const g of Object.values(groups)) g.forEach(addWord);
    for (const st of sentences) st.forEach(addWord);
    const co = {};
    for (const st of sentences) for (const a of st) for (const b of st) if (a !== b) { co[a] = co[a] || {}; co[a][b] = (co[a][b] || 0) + 1; }
    const names = Object.keys(groups);
    names.forEach((g, i) => groups[g].forEach((w, k) => { groupOf[w] = groupOf[w] || g; if (mode === "groups") omega[w] = omega[w] ?? 2 + i * 1.2 + (k - (groups[g].length - 1) / 2) * 0.12; }));
    const band = Math.max(4, words.length / 25);   // more words, a wider band of frequencies (room for every meaning)
    if (mode === "random" || mode === "raw") words.forEach(w => omega[w] = 2 + r() * band);
    else {
      const free = words.filter(w => omega[w] === undefined);
      const fixed = words.length - free.length;
      free.forEach(w => omega[w] = 2 + r() * 4);
      for (let it = 0; it < 60; it++) {
        const next = {};
        for (const w of free) {
          let t = 0, n = 0;
          for (const [o, c] of Object.entries(co[w] || {})) { t += c * omega[o]; n += c; }
          next[w] = n ? 0.5 * omega[w] + 0.5 * t / n : omega[w];
        }
        if (fixed) free.forEach(w => omega[w] = next[w]);
        else free.slice().sort((a, b) => next[a] - next[b]).forEach((w, k) => omega[w] = 2 + 4 * k / Math.max(1, free.length - 1));   // keep them spread
      }
    }
    const raw = mode === "raw";      // raw: nothing given but the sentences; groups are only used to name what it learned
    const neurons = words.map(w => {
      const a = ((omega[w] - 1.6) / 5) * Math.PI * 2, rad = 0.3 + 0.1 * r();
      return { label: w, group: raw ? null : groupOf[w] || null, truth: groupOf[w] || null, omega: omega[w], x: 0.5 + rad * Math.cos(a), y: 0.5 + rad * 0.95 * Math.sin(a) };
    });
    if (raw) {
      const outs = [...new Set(sentences.flatMap(st => st.out || []))];
      outs.forEach((o, k) => neurons.push({ label: o, omega: 2 + band * (k + 0.5) / outs.length, x: 0.05 + 0.9 * (k + 0.5) / outs.length, y: 0.04, fixed: true }));
      for (const n of neurons) { const v = numberValue(n.label); if (v != null) { n.omega = numberOmega(v); n.fixed = true; } }
      return { neurons, links: [], sentences, groups: {} };
    }
    const links = [], linked = new Set(), key = (a, b) => a < b ? a + "|" + b : b + "|" + a;
    for (const g of Object.values(groups)) for (let i = 0; i < g.length; i++) for (let j = i + 1; j < g.length; j++) if (!linked.has(key(g[i], g[j]))) { linked.add(key(g[i], g[j])); links.push([g[i], g[j], 0.5]); }
    for (const a of Object.keys(co)) for (const b of Object.keys(co[a])) if (!linked.has(key(a, b))) { linked.add(key(a, b)); links.push([a, b, 0.4]); }
    return { neurons, links, sentences, groups: Object.fromEntries(names.map((g, i) => [g, 2 + i * 1.2])) };
  }

  const SAMPLE_DATA = `# Meaning groups: "group name: words". Every other line is a sentence it practises on.
group remove: clear, wipe, remove, junk
group show: write, show, text, quote
group check: check, if, compare, equals
group place: window, put, place, position
clear crap
wipe crap
crap junk
remove crap
write crap
show crap
crap text
quote crap
write test
test text
check test
if test
compare test
put window
place position
clear junk
write text
check equals`;

  const SAMPLE_QUESTIONS = `# word @ context = what you expect (a group, a word it should be linked to, or "not relevant")
crap @ clear = remove
crap @ write = show
crap @ quote = show
test @ check = check
test @ text = show
crap @ put = not relevant
test @ window = not relevant`;

  // A random field: n neurons with random frequencies and random links (for watching sync and heat).
  function fieldSpec(n = 16, seed = 7) {
    const r = rng(seed), neurons = [], links = [];
    for (let i = 0; i < n; i++) neurons.push({ label: "n" + (i + 1), omega: 2 + r() * 3, x: 0.1 + 0.8 * r(), y: 0.1 + 0.8 * r() });
    for (let i = 0; i < n; i++) for (let j = i + 1; j < n; j++) if (r() < 0.18) links.push(["n" + (i + 1), "n" + (j + 1), 0.3 + 0.4 * r()]);
    return { neurons, links, sentences: neurons.map(x => [x.label]) };
  }

  const api = { ORDER, FLIP, numberValue, ENERGIES, PROOF, C, Network, FUNCTIONS, STACKS, ARCHETYPES, NEUTRAL, findArchetype, personality, decisions, parseData, parseQuestions, dataSpec, SAMPLE_DATA, SAMPLE_QUESTIONS, wordSpec, fieldSpec, rng, defaults };
  if (typeof module !== "undefined" && module.exports) module.exports = api; else root.INeuron = api;
})(typeof window !== "undefined" ? window : globalThis);
