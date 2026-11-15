// Emit test vectors: fn x_hex y_hex
const buf = new ArrayBuffer(8), f = new Float64Array(buf), u = new BigUint64Array(buf);
const hex = x => { f[0] = x; return u[0].toString(16).padStart(16, "0"); };
let s = 12345; const r = () => { s = (s * 1103515245 + 12345) % 2147483648; return s / 2147483648; };
const out = [];
const N = +process.argv[2] || 200000;
const pow2 = (x) => x ** 2;
for (let i = 0; i < N; i++) {
  const m = Math.pow(10, r() * 14 - 6) * (r() < 0.5 ? -1 : 1);
  const a = (r() - 0.5) * 20, b = (r() - 0.5) * 2e4, c = (r() - 0.5) * 2e6;
  const h = Math.pow(10, r() * 300) * (r() < 0.5 ? -1 : 1);
  for (const x of [m, a, b, c, h]) { out.push("sin " + hex(x) + " " + hex(Math.sin(x))); out.push("cos " + hex(x) + " " + hex(Math.cos(x))); }
  const e1 = -r() * 50, e2 = (r() - 0.5) * 1400, e3 = -Math.pow(r() * 3, 2);
  for (const x of [e1, e2, e3]) out.push("exp " + hex(x) + " " + hex(Math.exp(x)));
  const l1 = Math.pow(10, r() * 12 - 4), l2 = 1 + r() * 20000;
  for (const x of [l1, l2]) { out.push("log " + hex(x) + " " + hex(Math.log(x))); out.push("log10 " + hex(x) + " " + hex(Math.log10(x))); }
  const q = (r() - 0.5) * 30 * Math.pow(10, r() * 4 - 2);
  out.push("sq " + hex(q) + " " + hex(pow2(q)) + " " + hex(q * q));
  // toFixed (with exact ties: odd multiples of 1/64) and number printing
  const t1 = (r() - 0.5) * Math.pow(10, r() * 8 - 4), t2 = Math.round((r() - 0.5) * 20000) / 64;
  for (const x of [t1, t2]) for (const d of [2, 3, 4]) out.push("fx" + d + " " + hex(x) + " " + hex(+x.toFixed(d)));
  const p = (r() - 0.5) * Math.pow(10, r() * 50 - 25);
  for (const x of [p, t1, t2, q * q]) out.push("str " + hex(x) + " " + String(x));
}
process.stdout.write(out.join("\n") + "\n");
