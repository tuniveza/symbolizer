// Build the page: the engine inlined, and the measured results written in.
const fs = require("fs");
const dir = __dirname;
const R = JSON.parse(fs.readFileSync(dir + "/results.json", "utf8"));
const S = require("./sim.js");
const b = R.base, u = R.unseen, avg = xs => Math.round(xs.filter(x => x != null).reduce((a, c) => a + c, 0) / xs.filter(x => x != null).length);
const essential = Object.entries(R.drives).filter(([, v]) => v.right < b.right - 15).map(([k, v]) => `${S.ENERGIES.find(e => e.key === k).name} (${v.right}%)`);
const harmless = Object.entries(R.drives).filter(([, v]) => v.right >= b.right - 5).map(([k]) => S.ENERGIES.find(e => e.key === k).name);
const gapKeys = Object.keys(R.gaps).sort((a, c) => c - a);
const gapText = gapKeys.map(g => `${g}: ${R.gaps[g].right}%`).join(" · ");

const rows = [
  { test: "Right meaning in a trained context", value: `${b.right}% right · ${b.wrong}% wrong · ${b.refused}% held back`, pct: b.right,
    means: `15 contexts × ${b.seeds} runs. When it isn't sure, it says "partly" instead of guessing, so it was never wrong.` },
  { test: "Unrelated context marked “not relevant”", value: `${b.notRelevantOk}%`, pct: b.notRelevantOk,
    means: `"put crap", "window test" and 4 more. The 16 percentages show no sense gets a fair share, so it claims no meaning.` },
  { test: "Contexts it never saw in training", value: `${u.right}% right · ${u.wrong}% wrong`, pct: u.right,
    means: `"quote crap" and "compare test" were left out of training. It still read them right through the lines it already had.` },
  { test: "Words split into meanings by themselves", value: `crap ${b.splitCrap}/${b.seeds} · test ${b.splitTest}/${b.seeds}`, pct: 100 * (b.splitCrap + b.splitTest) / (2 * b.seeds),
    means: `First split after about ${b.firstSplit} s. No word with only one meaning ever split (${b.wrongSplits} wrong splits).` },
  { test: "A third meaning grows on its own", value: `${R.third.three}/${R.third.seeds} runs`, pct: 100 * R.third.three / R.third.seeds,
    means: `When "crap" is also used with put and place, it ends up with 3 senses.` },
  { test: "Forgetting with no practice", value: `~${avg(R.forget.times)} s`, pct: null,
    means: `Stop feeding sentences and a meaning loses its lines after about ${avg(R.forget.times)} s of simulated time (refinement removes unused lines).` },
  { test: "How close meaning groups can be", value: gapText, pct: null,
    means: `Frequency gap between groups. Splitting comes from use, so closeness barely matters here.` },
  { test: "Random frequencies (no cortex engine)", value: `${R.random.right}% right · ${R.random.wrong}% wrong`, pct: R.random.right,
    means: `Frequencies scrambled, as if nothing encoded the words. ${R.random.right < b.right - 10 ? "Reading gets clearly worse" : "Reading mostly survives"}: frequencies carry part of the meaning.` },
  { test: "Energies it can't do without", value: essential.length ? essential.join(", ") : "none on this test", pct: null,
    means: `Each energy switched off on its own (6 runs each). These dropped accuracy by more than 15 points. Barely missed: ${harmless.join(", ")}.` },
  { test: "Speed as it grows", value: Object.entries(R.scale).map(([n, ms]) => `${n}: ${ms} ms`).join(" · "), pct: null,
    means: `Time per step on a vocabulary-like network (about 5 lines a word). Once big, a neuron only checks neurons close to its frequency and its own lines, so 10× more neurons costs about 10× more time.` },
];

const verdicts = [
  { kind: "yes", label: "Works", title: "Finds meanings from use", text: `Ambiguous words split by themselves when their contexts never meet: crap ${b.splitCrap}/${b.seeds}, test ${b.splitTest}/${b.seeds}, and no false splits.` },
  { kind: "yes", label: "Works", title: "Proves or rejects a meaning", text: `With 16 percentages per sense it picked the right meaning ${b.right}% of the time, was never wrong, and marked unrelated contexts not relevant ${b.notRelevantOk}% of the time.` },
  { kind: "yes", label: "Works", title: "Keeps growing", text: `A third use of a word grew a third sense in ${R.third.three} of ${R.third.seeds} runs. This is your "keep generating bricks" idea, at small size.` },
  { kind: "part", label: "Partly", title: "It forgets", text: `A meaning nobody uses fades after about ${avg(R.forget.times)} s. Good for tidiness, bad for rare words. It needs practice or replay (like sleep) to keep rare meanings.` },
  { kind: "part", label: "Partly", title: "Needs your cortex engines", text: `With scrambled frequencies it scored ${R.random.right}% instead of ${b.right}%. Splitting works from use alone, but reading a context also relies on frequencies, and nothing makes those yet.` },
  { kind: "no", label: "Limit", title: "Doesn't understand", text: `It groups words by how they're used together. It can't know a meaning nobody ever used, and two meanings that share the same contexts can never be told apart.` },
  { kind: "part", label: "Partly", title: "Size", text: `1,000 neurons take ${R.scale[1000]} ms per step and 10,000 take ${R.scale[10000]} ms: neurons only check nearby frequencies now. A full English vocabulary is still slow, but no longer out of reach.` },
  { kind: "yes", label: "Works", title: "Learns by itself", text: `Pick "Nothing given" under Frequencies: no groups, no lines, random frequencies. Ni guesses each next word and learns from every wrong guess; Ne proposes words used alike. In brick, its proposals are checked by brick's compiler, and new bricks are only kept when they build exactly the same bits.` },
];

const choices = [
  `<b>Frequencies are set by hand</b>, one band per meaning group, with "crap" and "test" starting between two groups.`,
  `<b>In sync</b> = how close two frequencies are × how aligned their rhythms are. An impact relays above 0.45 (a little lower when a neuron is warm: potential).`,
  `<b>Hot</b> is a temperature above 1.0 and <b>cold</b> is below 0.5. Splitting (nuclear fission) only happens while cold.`,
  `<b>The imagination layer sees use, not only impacts.</b> It notes which words come in the same sentence. Without this, "crap" was pulled to one side before it ever cooled down, and never split.`,
  `<b>A word splits</b> when its partners form two groups that are never used together (each with 2+ members and at least 25% of its use).`,
  `<b>A context attaches to the sense it fits best.</b> Without this, new senses kept splitting again and again.`,
  `<b>Ionization only pushes apart neurons with no line between them.</b> Otherwise it pushed a new sense away from its own meaning group.`,
  `<b>Lines unused for about 40 s fade</b> (refinement). This is what causes forgetting.`,
  `<b>Proving a meaning:</b> for each energy, a sense's percentage is its share of the context's energy among all neurons reached. Over a fair share counts as a match. More than half the energies = accurate, a quarter = partly, less = not relevant.`,
  `<b>What each energy counts as evidence is my reading:</b> ${Object.entries(S.PROOF).map(([k, v]) => `${S.ENERGIES.find(e => e.key === k).name}: ${v}`).join("; ")}.`,
];

let html = fs.readFileSync(dir + "/page.html", "utf8");
html = html.replace("/*SIM*/", () => fs.readFileSync(dir + "/sim.js", "utf8").replace(/<\/script/gi, "<\\/script"));
html = html.replace("/*ARCH*/null", () => fs.readFileSync(dir + "/archetype-results.json", "utf8").replace(/<\//g, "<\\/"));
html = html.replace("/*RESULTS*/null", () => JSON.stringify({ rows, verdicts, choices }).replace(/<\//g, "<\\/"));
fs.writeFileSync(dir + "/pages/imagination-neurons.html", html);
console.log("built", html.length, "bytes");

// The archetype gallery: the engine and the measured archetype results written in.
let gal = fs.readFileSync(dir + "/gallery.html", "utf8");
gal = gal.replace("/*SIM*/", () => fs.readFileSync(dir + "/sim.js", "utf8").replace(/<\/script/gi, "<\\/script"));
gal = gal.replace("/*ARCH*/null", () => fs.readFileSync(dir + "/archetype-results.json", "utf8").replace(/<\//g, "<\\/"));
fs.writeFileSync(dir + "/pages/imagination-archetypes.html", gal);
console.log("built gallery", gal.length, "bytes");
