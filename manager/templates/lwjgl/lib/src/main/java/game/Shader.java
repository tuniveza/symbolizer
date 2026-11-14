package game;

import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;

import static org.lwjgl.opengl.GL20.*;

public class Shader {

    // Reads shader source from src/main/resources — put your .vert/.frag files there.
    // Classloader resource loading works both when running from Eclipse AND from a packaged jar.
    public static String readShaderSource(String resourcePath) {
        try (InputStream in = Shader .class.getClassLoader().getResourceAsStream(resourcePath)) {
            if (in == null) {
                throw new RuntimeException("Shader resource not found: " + resourcePath);
            }
            return new String(in.readAllBytes(), StandardCharsets.UTF_8);
        } catch (IOException e) {
            throw new RuntimeException("Failed to read shader: " + resourcePath, e);
        }
    }

    public static int compileShader(String source, int type) {
        int shader = glCreateShader(type);
        glShaderSource(shader, source);
        glCompileShader(shader);

        if (glGetShaderi(shader, GL_COMPILE_STATUS) == GL_FALSE) {
            String log = glGetShaderInfoLog(shader);
            glDeleteShader(shader);
            throw new RuntimeException("Shader compile error:\n" + log);
        }
        return shader;
    }

    public static int createProgram(String vertPath, String fragPath) {
        int vertShader = compileShader(readShaderSource(vertPath), GL_VERTEX_SHADER);
        int fragShader = compileShader(readShaderSource(fragPath), GL_FRAGMENT_SHADER);

        int program = glCreateProgram();
        glAttachShader(program, vertShader);
        glAttachShader(program, fragShader);
        glLinkProgram(program);

        if (glGetProgrami(program, GL_LINK_STATUS) == GL_FALSE) {
            String log = glGetProgramInfoLog(program);
            throw new RuntimeException("Shader link error:\n" + log);
        }

        // Shaders are compiled into the program now; the standalone objects can go
        glDeleteShader(vertShader);
        glDeleteShader(fragShader);

        return program;
    }
}