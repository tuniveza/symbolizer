# __NAME__ — DragonRuby. Saves hot-reload while the game runs. Docs: ~/an/st/tools/tc/dragonruby/docs
def tick args
  args.state.player ||= { x: 640, y: 360, w: 32, h: 32 }
  p = args.state.player
  p.x += args.inputs.left_right * 5
  p.y += args.inputs.up_down * 5

  args.outputs.background_color = [20, 20, 30]
  args.outputs.solids << { **p, r: 100, g: 149, b: 237 }
  args.outputs.labels << { x: 20, y: 700, text: "__NAME__  (arrows/WASD to move)", r: 255, g: 255, b: 255 }
end
