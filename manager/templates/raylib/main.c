#include <raylib.h>
#include <raymath.h>

int main(void) {
    SetConfigFlags(FLAG_WINDOW_RESIZABLE | FLAG_VSYNC_HINT);
    InitWindow(1280, 720, "__NAME__");

    Vector2 pos = {640, 360};
    const float speed = 300.0f;

    while (!WindowShouldClose()) {
        float dt = GetFrameTime();
        Vector2 dir = {
            (float)(IsKeyDown(KEY_D) - IsKeyDown(KEY_A)),
            (float)(IsKeyDown(KEY_S) - IsKeyDown(KEY_W)),
        };
        pos = Vector2Add(pos, Vector2Scale(Vector2Normalize(dir), speed * dt));

        BeginDrawing();
        ClearBackground((Color){24, 26, 36, 255});
        DrawCircleV(pos, 24, ORANGE);
        DrawText("WASD to move", 20, 20, 20, RAYWHITE);
        DrawFPS(20, 50);
        EndDrawing();
    }

    CloseWindow();
    return 0;
}
