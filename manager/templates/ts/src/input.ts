// keyboard state: Input.down("KeyA"), Input.pressed("Space") (true only on the first frame)
const held = new Set<string>();
const fresh = new Set<string>();

addEventListener("keydown", (e) => {
  if (!held.has(e.code)) fresh.add(e.code);
  held.add(e.code);
});
addEventListener("keyup", (e) => held.delete(e.code));
addEventListener("blur", () => held.clear());

export const Input = {
  down: (code: string) => held.has(code),
  pressed: (code: string) => fresh.has(code),
  endFrame: () => fresh.clear(),
};
