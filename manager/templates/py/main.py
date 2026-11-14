import pygame

WIDTH, HEIGHT = 1280, 720
TITLE = "__NAME__"


def main():
    pygame.init()
    screen = pygame.display.set_mode((WIDTH, HEIGHT))
    pygame.display.set_caption(TITLE)
    clock = pygame.time.Clock()
    pos = pygame.Vector2(WIDTH / 2, HEIGHT / 2)

    running = True
    while running:
        dt = clock.tick(60) / 1000
        for event in pygame.event.get():
            if event.type == pygame.QUIT or (event.type == pygame.KEYDOWN and event.key == pygame.K_ESCAPE):
                running = False

        keys = pygame.key.get_pressed()
        move = pygame.Vector2(keys[pygame.K_d] - keys[pygame.K_a], keys[pygame.K_s] - keys[pygame.K_w])
        if move.length_squared():
            pos += move.normalize() * 300 * dt

        screen.fill((30, 30, 40))
        pygame.draw.circle(screen, (240, 180, 60), pos, 20)
        pygame.display.flip()

    pygame.quit()


if __name__ == "__main__":
    main()
