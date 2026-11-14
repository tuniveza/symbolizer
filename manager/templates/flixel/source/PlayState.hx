package;

import flixel.FlxG;
import flixel.FlxSprite;
import flixel.FlxState;
import flixel.text.FlxText;
import flixel.util.FlxColor;

class PlayState extends FlxState
{
	var player:FlxSprite;

	override public function create()
	{
		super.create();
		add(new FlxText(10, 10, 0, "__NAME__", 16));
		player = new FlxSprite(FlxG.width / 2, FlxG.height / 2);
		player.makeGraphic(32, 32, FlxColor.WHITE);
		add(player);
	}

	override public function update(elapsed:Float)
	{
		super.update(elapsed);
		var speed = 200 * elapsed;
		if (FlxG.keys.pressed.LEFT) player.x -= speed;
		if (FlxG.keys.pressed.RIGHT) player.x += speed;
		if (FlxG.keys.pressed.UP) player.y -= speed;
		if (FlxG.keys.pressed.DOWN) player.y += speed;
	}
}
