import { Input } from "./input.ts";

const canvas = document.getElementById("game") as HTMLCanvasElement;
const ctx = canvas.getContext("2d")!;

function resize() {
  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.round(innerWidth * dpr);
  canvas.height = Math.round(innerHeight * dpr);
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.imageSmoothingEnabled = false;
}
addEventListener("resize", resize);
resize();

const player = { x: innerWidth / 2, y: innerHeight / 2, speed: 240, size: 32 };

function update(dt: number) {
  if (Input.down("KeyA") || Input.down("ArrowLeft")) player.x -= player.speed * dt;
  if (Input.down("KeyD") || Input.down("ArrowRight")) player.x += player.speed * dt;
  if (Input.down("KeyW") || Input.down("ArrowUp")) player.y -= player.speed * dt;
  if (Input.down("KeyS") || Input.down("ArrowDown")) player.y += player.speed * dt;
}

function draw() {
  ctx.fillStyle = "#111";
  ctx.fillRect(0, 0, innerWidth, innerHeight);
  ctx.fillStyle = "salmon";
  ctx.fillRect(player.x - player.size / 2, player.y - player.size / 2, player.size, player.size);
}

let last = performance.now();
function frame(now: number) {
  const dt = Math.min((now - last) / 1000, 0.1); // seconds, clamped so a tab switch doesn't teleport you
  last = now;
  update(dt);
  draw();
  Input.endFrame();
  requestAnimationFrame(frame);
}
requestAnimationFrame(frame);
