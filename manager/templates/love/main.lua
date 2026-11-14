local x, y = 640, 360

function love.load()
  love.graphics.setDefaultFilter("nearest", "nearest")
end

function love.update(dt)
  local speed = 200
  if love.keyboard.isDown("left", "a") then x = x - speed * dt end
  if love.keyboard.isDown("right", "d") then x = x + speed * dt end
  if love.keyboard.isDown("up", "w") then y = y - speed * dt end
  if love.keyboard.isDown("down", "s") then y = y + speed * dt end
end

function love.draw()
  love.graphics.print("__NAME__", 10, 10)
  love.graphics.rectangle("fill", x - 16, y - 16, 32, 32)
end

function love.keypressed(key)
  if key == "escape" then love.event.quit() end
end
