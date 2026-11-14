const ctx = document.getElementById("c").getContext("2d");
let t = 0;
function frame() {
  t += 1 / 60;
  ctx.fillStyle = "#000";
  ctx.fillRect(0, 0, 640, 360);
  ctx.fillStyle = "salmon";
  ctx.fillRect(320 + Math.cos(t) * 120 - 16, 180 + Math.sin(t) * 80 - 16, 32, 32);
  requestAnimationFrame(frame);
}
frame();
