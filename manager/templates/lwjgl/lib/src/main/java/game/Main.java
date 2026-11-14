package game;


import java.io.IOException;
import java.io.InputStream;
import java.io.UncheckedIOException;
import java.nio.ByteBuffer;
import java.nio.FloatBuffer;
import java.nio.IntBuffer;

import org.joml.Matrix4f;

import org.lwjgl.BufferUtils;
import org.lwjgl.Version;
import org.lwjgl.glfw.GLFWErrorCallback;
import org.lwjgl.opengl.GL;
import org.lwjgl.system.MemoryStack;

import static org.lwjgl.glfw.GLFW.*;
import static org.lwjgl.opengl.GL11.*;
import static org.lwjgl.opengl.GL13.GL_MULTISAMPLE;
import static org.lwjgl.opengl.GL20.*;
import static org.lwjgl.stb.STBImage.*;
import static org.lwjgl.system.MemoryUtil.NULL;

public class Main {

    private static long window;
    private static int sp;
    
    
    private static void setMat4(int program, String name, Matrix4f mat) {
        try (MemoryStack stack = MemoryStack.stackPush()) {
            glUniformMatrix4fv(glGetUniformLocation(program, name), false, mat.get(stack.mallocFloat(16)));
        }
    }
   

    public static void main(String[] args) {
        System.out.println("LWJGL version: " + Version.getVersion());

        GLFWErrorCallback.createPrint(System.err).set();

        if (!glfwInit()) {
            throw new IllegalStateException("Unable to initialize GLFW");
        }

        glfwWindowHint(GLFW_CONTEXT_VERSION_MAJOR, 2);
        glfwWindowHint(GLFW_CONTEXT_VERSION_MINOR, 1);
        glfwWindowHint(GLFW_VISIBLE, GLFW_TRUE);
        glfwWindowHint(GLFW_RESIZABLE, GLFW_TRUE);
        glfwWindowHint(GLFW_SAMPLES, 4); // request 4x multisampling for smooth edges

        window = glfwCreateWindow(640, 480, "LWJGL Triangle", NULL, NULL);
        if (window == NULL) {
            throw new RuntimeException("Failed to create the GLFW window");
        }

        glfwMakeContextCurrent(window);
        glfwSwapInterval(1);
        glfwShowWindow(window);

        GL.createCapabilities();
        System.out.println("OpenGL version: " + glGetString(GL_VERSION));
        
       
        
        Shader shader = new Shader();
        
        sp = shader.createProgram("shaders/basic.vert", "shaders/basic.frag");
        
        try (MemoryStack stack = MemoryStack.stackPush()) {
            FloatBuffer buf = stack.mallocFloat(16);
        }


        glClearColor(0.1f, 0.1f, 0.15f, 1.0f);
        glEnable(GL_MULTISAMPLE);
        
        
        IntBuffer width = BufferUtils.createIntBuffer(1);
        IntBuffer height = BufferUtils.createIntBuffer(1);
        IntBuffer components = BufferUtils.createIntBuffer(1);
        // put the image in lib/src/main/resources/textures/
        ByteBuffer file = ioResourceToByteBuffer("textures/environment.jpg");
        if (file != null) {
            ByteBuffer data = stbi_load_from_memory(file, width, height, components, 4);
            int id = glGenTextures();
            glBindTexture(GL_TEXTURE_2D, id);
            glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
            glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
            glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA, width.get(), height.get(), 0, GL_RGBA, GL_UNSIGNED_BYTE, data);
            stbi_image_free(data);
        } else {
            System.out.println("texture textures/environment.jpg not found, skipping");
        }

        while (!glfwWindowShouldClose(window)) {
            frame(); // <-- called fresh every iteration, so HotSwap edits here apply live

            glfwPollEvents();
            if (glfwGetKey(window, GLFW_KEY_ESCAPE) == GLFW_PRESS) {
                glfwSetWindowShouldClose(window, true);
            }
        }

        glfwDestroyWindow(window);
        glfwTerminate();
        System.out.println("Closed cleanly");
    }

    // reads a classpath resource (src/main/resources) into a direct buffer for stb; null if missing
    private static ByteBuffer ioResourceToByteBuffer(String resource) {
        try (InputStream in = Main.class.getClassLoader().getResourceAsStream(resource)) {
            if (in == null) return null;
            byte[] bytes = in.readAllBytes();
            ByteBuffer buf = BufferUtils.createByteBuffer(bytes.length);
            buf.put(bytes).flip();
            return buf;
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    private static float angle = 0f;
    private static float panX = 0.0f;
    
 
    
    // Edit THIS method while the app is running and save (Ctrl+S) to hot-swap it live.
    private static void frame() {
        glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT);
        
       
        
        
        
        // Fix aspect ratio distortion: square coordinate space regardless of window shape
        int[] w = new int[1], h = new int[1];
        glfwGetFramebufferSize(window, w, h);
        glViewport(0, 0, w[0], h[0]);
        float aspect = (float) w[0] / h[0];
        //panX += 0.1f;
        
        Matrix4f 
        model = new Matrix4f()
        .rotateZ((float) Math.toRadians(angle))
        .translate(0.0f, 0.0f, 0.0f);
        Matrix4f view = new Matrix4f()
        		.lookAt(
        		panX, 0, 6,  
        		panX, 0, 0,  
        		0, 1, 0);
        Matrix4f 
        projection = new Matrix4f()
        .perspective((float) Math.toRadians(45.0f), aspect, 0.1f, 100.0f);
        
        glUseProgram(sp);
        
        setMat4(sp, "model", model);
        setMat4(sp, "view", view);
        setMat4(sp, "projection", projection);
       

        // Fix matrix accumulation: reset transform every frame instead of stacking rotations
       // glLoadIdentity();
        //glRotatef(angle, 0f, 0f, 1f);
        angle += 0.01f; // degrees per frame

        glBegin(GL_TRIANGLES);
        glColor3f(1.0f, 0.2f, 0.2f);
        glVertex2f(0.0f, 0.5f);

        glColor3f(0.2f, 1.0f, 0.2f);
        glVertex2f(-0.5f, -0.5f);

        glColor3f(0.2f, 0.2f, 1.0f);
        glVertex2f(0.5f, -0.5f);
        glEnd();

        glfwSwapBuffers(window);
    }
}