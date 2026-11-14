module main

import gg

struct App {
mut:
	ctx &gg.Context = unsafe { nil }
	x   f32         = 640
	y   f32         = 360
}

fn frame(mut app App) {
	speed := f32(5)
	if app.ctx.pressed_keys[int(gg.KeyCode.left)] { app.x -= speed }
	if app.ctx.pressed_keys[int(gg.KeyCode.right)] { app.x += speed }
	if app.ctx.pressed_keys[int(gg.KeyCode.up)] { app.y -= speed }
	if app.ctx.pressed_keys[int(gg.KeyCode.down)] { app.y += speed }
	app.ctx.begin()
	app.ctx.draw_rect_filled(app.x - 16, app.y - 16, 32, 32, gg.Color{100, 149, 237, 255})
	app.ctx.draw_text(20, 20, '__NAME__', color: gg.white, size: 20)
	app.ctx.end()
}

fn main() {
	mut app := &App{}
	app.ctx = gg.new_context(
		width:        1280
		height:       720
		window_title: '__NAME__'
		bg_color:     gg.Color{20, 20, 30, 255}
		frame_fn:     frame
		user_data:    app
	)
	app.ctx.run()
}
