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
    WORK: 0.35, OSC: 0.5, SHARE: 0.25, SPLIT_GAP: 0.4, MEAN_GAP: 1.0, CO_MIN: 0.15, CO_LINK: 0.1, MIN_GROUP: 2, NOISE: 0.6, LIT_DECAY: 0.4, INBOX_DECAY: 0.015, IMAGINE_EVERY: 20, TRAIN_EVERY: 20,
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

  function defaults() { const e = {}; for (const x of ENERGIES) e[x.key] = 1; return e; }

  class Network {
    // brain: an archetype (object or name). Its personality sets the 16 energies and steers heat and order;
    // its decisions set the rules (how strict, how soon to split, how long to keep lines, ...). No brain = neutral.
    constructor(spec, seed = 1, energies = null, brain = null) {
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
        inbox: new Map(), co: new Map(), stepIn: new Map(), stepHeat: new Map(), stepThermal: new Map(), stepLit: new Map(), litBy: null, refined: 0, x: n.x ?? r(), y: n.y ?? r(), relayed: 0, hidden: !!n.hidden,
      };
      this.N.push(neuron);
      return neuron;
    }

    byLabel(label) { return this.N.find(n => n.label === label); }
    key(a, b) { return a.id < b.id ? a.id + "|" + b.id : b.id + "|" + a.id; }
    link(a, b) { return this.links.get(this.key(a, b)); }
    connect(a, b, w = 0.5) {
      if (!a || !b || a === b) return;
      this.links.set(this.key(a, b), { a: a.id, b: b.id, w, w0: w, carried: 0, idle: 0 });
    }
    linksOf(n) { const out = []; for (const l of this.links.values()) if (l.a === n.id || l.b === n.id) out.push(l); return out; }
    get(id) { return this.N.find(n => n.id === id); }

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

      for (let i = 0; i < n; i++) {
        const a = N[i];
        for (let j = i + 1; j < n; j++) {
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
            const imp = C.RELAY * e.kinetic * c * a.amp * a.E * s * dt;
            inc[j] += imp; b.inbox.set(a.id, (b.inbox.get(a.id) || 0) + imp); b.stepIn.set(a.id, imp); a.relayed += imp;
            if (l) l.carried += imp;
          }
          if (c > 0 && s > bar(a) && b.E > 0.01) {
            const imp = C.RELAY * e.kinetic * c * b.amp * b.E * s * dt;
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
      // nuclear fission: a cold neuron whose impacts come from two separate groups splits into two
      for (const a of [...this.N]) {
        if (a.T >= C.T_COLD) continue;
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
        if (this.linksOf(a).length || a.lit < this.d.radiant || a.litBy == null) continue;
        const src = this.get(a.litBy);
        if (!src || e.radiant <= 0) continue;
        a.omega0 = a.omega = a.omega + 0.6 * (src.omega - a.omega);
        this.connect(a, src, 0.3);
        a.hidden = false;
        this.say(`radiant: hidden ${a.label} was lit up by ${src.label}; the imagination layer tuned it closer and joined them`);
      }
    }

    fission(a, sp) {
      const base = a.word;
      this.senseCount = this.senseCount || {};
      const mk = (part, mean, side) => {
        const suffix = this.senseCount[base] = (this.senseCount[base] || 0) + 1;
        const child = this.add({ label: `${base}·${suffix}`, word: base, omega: mean, x: a.x + side * 0.05, y: a.y + 0.05 });
        child.T = a.T; child.amp = a.amp; child.theta = a.theta; child.parentLabel = a.label;
        for (const { s } of part) { const old = this.link(a, s); this.connect(child, s, old ? old.w : 0.3); }
        return child;
      };
      const c1 = mk(sp.lo, sp.loMean, -1), c2 = mk(sp.hi, sp.hiMean, 1);
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
      this.N = this.N.filter(x => x !== a);
      for (const x of this.N) { x.inbox.delete(a.id); x.co.delete(a.id); }
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
      s = s || this.sentences[Math.floor(this.rand() * this.sentences.length)];
      this.stimulate(s, 1.2);
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
        for (const r of this.N) {
          if (isCtx(r)) continue;
          const take = m => { let v = 0; for (const [id, a] of m) { const src = this.get(id); if (src && isCtx(src)) v += a; } return v; };
          const imp = take(r.stepIn), allIn = [...r.stepIn.values()].reduce((t, v) => t + v, 0);
          add(r, "kinetic", imp); add(r, "mechanical", imp * C.ABSORB);
          add(r, "heat", take(r.stepHeat)); add(r, "thermal", take(r.stepThermal)); add(r, "radiant", take(r.stepLit));
          add(r, "potential", allIn > 0 ? r.E * imp / allIn * dt : 0);
          add(r, "work", take(r.inbox) * dt);
          add(r, "chemical", Math.exp(-(((r.omega0 - ctxOmega0) / C.SIGMA) ** 2)) * dt);
          let fsAll = 0;
          for (const o of this.N) if (o !== r && !isCtx(o)) fsAll += this.sync(r, o).fsim;
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

    fromContext(s, ctx) { let t = 0; for (const [id, v] of (s.stepIn || new Map())) { const src = this.get(id); if (src && (ctx.has(src.word) || this.isNear(src, ctx))) t += v; } return t; }
    isNear(src, ctx) { for (const l of this.linksOf(src)) { const o = this.get(l.a === src.id ? l.b : l.a); if (o && ctx.has(o.word)) return true; } return false; }

    // The group a neuron's linked neighbours mostly belong to (for display and scoring only; the dynamics never read it).
    meaningOf(n) {
      if (n.group) return n.group;                                       // a word that never split keeps its own group
      const count = {};
      for (const l of this.linksOf(n)) { const o = this.get(l.a === n.id ? l.b : l.a); if (o && o.group) count[o.group] = (count[o.group] || 0) + l.w; }
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
      if (this.N.some(x => x.group === want)) return this.meaningOf(r.winner) === want;
      return this.neighbours(r.winner).slice(0, 4).includes(want);
    }

    // Overall readings for the page.
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
      const line = raw.replace(/#.*/, "").trim().toLowerCase();
      if (!line) continue;
      const g = line.match(/^group\s+([a-z0-9_-]+)\s*:\s*(.+)$/);
      if (g) { groups[g[1]] = (g[2].match(/[a-z0-9']+/g) || []); continue; }
      const words = line.match(/[a-z0-9']+/g) || [];
      if (words.length) sentences.push(words);
    }
    return { groups, sentences };
  }

  function parseQuestions(text) {
    const out = [];
    for (const raw of String(text).split(/\n/)) {
      const line = raw.replace(/#.*/, "").trim().toLowerCase();
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
    if (mode === "random") words.forEach(w => omega[w] = 2 + r() * 4);
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
    const neurons = words.map(w => {
      const a = ((omega[w] - 1.6) / 5) * Math.PI * 2, rad = 0.3 + 0.1 * r();
      return { label: w, group: groupOf[w] || null, omega: omega[w], x: 0.5 + rad * Math.cos(a), y: 0.5 + rad * 0.95 * Math.sin(a) };
    });
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

  const api = { ENERGIES, PROOF, C, Network, FUNCTIONS, STACKS, ARCHETYPES, NEUTRAL, findArchetype, personality, decisions, parseData, parseQuestions, dataSpec, SAMPLE_DATA, SAMPLE_QUESTIONS, wordSpec, fieldSpec, rng, defaults };
  if (typeof module !== "undefined" && module.exports) module.exports = api; else root.INeuron = api;
})(typeof window !== "undefined" ? window : globalThis);
