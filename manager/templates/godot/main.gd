extends Node2D

const SPEED := 300.0
var pos := Vector2(640, 360)

func _process(delta: float) -> void:
	var dir := Vector2(
		Input.get_axis("ui_left", "ui_right"),
		Input.get_axis("ui_up", "ui_down"))
	pos += dir * SPEED * delta
	if Input.is_action_just_pressed("ui_cancel"):
		get_tree().quit()
	queue_redraw()

func _draw() -> void:
	draw_rect(Rect2(pos - Vector2(16, 16), Vector2(32, 32)), Color.CORNFLOWER_BLUE)
