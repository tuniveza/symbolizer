require "raylib-cr"

Raylib.init_window(1280, 720, "__NAME__")
Raylib.set_target_fps(60)

pos = Raylib::Vector2.new(x: 640, y: 360)
until Raylib.close_window?
  dt = Raylib.get_frame_time
  pos.x -= 300 * dt if Raylib.key_down?(Raylib::KeyboardKey::Left)
  pos.x += 300 * dt if Raylib.key_down?(Raylib::KeyboardKey::Right)
  pos.y -= 300 * dt if Raylib.key_down?(Raylib::KeyboardKey::Up)
  pos.y += 300 * dt if Raylib.key_down?(Raylib::KeyboardKey::Down)

  Raylib.begin_drawing
  Raylib.clear_background(Raylib::Color.new(r: 20, g: 20, b: 30, a: 255))
  Raylib.draw_rectangle_v(Raylib::Vector2.new(x: pos.x - 16, y: pos.y - 16), Raylib::Vector2.new(x: 32, y: 32), Raylib::SKYBLUE)
  Raylib.draw_text("__NAME__", 20, 20, 20, Raylib::RAYWHITE)
  Raylib.end_drawing
end

Raylib.close_window
