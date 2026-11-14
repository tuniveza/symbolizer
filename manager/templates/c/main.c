#include <SDL3/SDL.h>
#include <SDL3/SDL_main.h>
#include <glad/gl.h>

int main(int argc, char **argv) {
    (void)argc; (void)argv;

    if (!SDL_Init(SDL_INIT_VIDEO)) {
        SDL_Log("SDL_Init: %s", SDL_GetError());
        return 1;
    }
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, 3);
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, 3);
    SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_CORE);

    SDL_Window *win = SDL_CreateWindow("__NAME__", 1280, 720, SDL_WINDOW_OPENGL | SDL_WINDOW_RESIZABLE);
    SDL_GLContext ctx = SDL_GL_CreateContext(win);
    if (!win || !ctx || !gladLoadGL((GLADloadfunc)SDL_GL_GetProcAddress)) {
        SDL_Log("window/GL init failed: %s", SDL_GetError());
        return 1;
    }
    SDL_GL_SetSwapInterval(1);

    bool running = true;
    while (running) {
        SDL_Event e;
        while (SDL_PollEvent(&e)) {
            if (e.type == SDL_EVENT_QUIT) running = false;
            if (e.type == SDL_EVENT_KEY_DOWN && e.key.key == SDLK_ESCAPE) running = false;
        }

        int w, h;
        SDL_GetWindowSizeInPixels(win, &w, &h);
        glViewport(0, 0, w, h);

        float t = SDL_GetTicks() / 1000.0f;
        glClearColor(0.10f, 0.12f + 0.06f * SDL_sinf(t), 0.18f, 1.0f);
        glClear(GL_COLOR_BUFFER_BIT);

        SDL_GL_SwapWindow(win);
    }

    SDL_GL_DestroyContext(ctx);
    SDL_DestroyWindow(win);
    SDL_Quit();
    return 0;
}
