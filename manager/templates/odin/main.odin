package main

import rl "vendor:raylib"

main :: proc() {
	rl.InitWindow(1280, 720, "__NAME__")
	defer rl.CloseWindow()
	rl.SetTargetFPS(60)

	pos := rl.Vector2{640, 360}
	for !rl.WindowShouldClose() {
		dt := rl.GetFrameTime()
		if rl.IsKeyDown(.LEFT) || rl.IsKeyDown(.A) do pos.x -= 300 * dt
		if rl.IsKeyDown(.RIGHT) || rl.IsKeyDown(.D) do pos.x += 300 * dt
		if rl.IsKeyDown(.UP) || rl.IsKeyDown(.W) do pos.y -= 300 * dt
		if rl.IsKeyDown(.DOWN) || rl.IsKeyDown(.S) do pos.y += 300 * dt

		rl.BeginDrawing()
		rl.ClearBackground({20, 20, 30, 255})
		rl.DrawRectangleV(pos - 16, {32, 32}, rl.SKYBLUE)
		rl.DrawText("__NAME__", 20, 20, 20, rl.RAYWHITE)
		rl.EndDrawing()
	}
}
