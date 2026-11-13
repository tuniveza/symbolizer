//go:build linux && cgo

package main

// ---- the graphics card: OpenGL with no window ---------------------------------------------
//
// The 3D room's picture (kitty) is drawn on the graphics card when there
// is one (design_3d_gpu.go). This file is the link to it: OpenGL 3.3 in a
// context with no window (EGL), its functions looked up at run time, so
// sy builds and runs on a computer without them (it then draws on the
// processor as before). Every call here must come from the one thread
// that holds the context (see gpuRun).

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef void *EGLDisplay, *EGLContext, *EGLConfig, *EGLSurface, *EGLDeviceEXT;
typedef int32_t EGLint;
typedef unsigned int EGLBoolean, EGLenum;
typedef unsigned int GLenum, GLuint, GLbitfield;
typedef int GLint, GLsizei;
typedef float GLfloat;
typedef unsigned char GLubyte;
typedef char GLchar;
typedef intptr_t GLsizeiptr, GLintptr;

#define EGL_NONE 0x3038
#define EGL_EXTENSIONS 0x3055
#define EGL_OPENGL_API 0x30A2
#define EGL_PLATFORM_SURFACELESS_MESA 0x31DD
#define EGL_PLATFORM_DEVICE_EXT 0x313F
#define EGL_CONTEXT_MAJOR_VERSION 0x3098
#define EGL_CONTEXT_MINOR_VERSION 0x30FB
#define EGL_CONTEXT_OPENGL_PROFILE_MASK 0x30FD
#define EGL_RENDERABLE_TYPE 0x3040
#define EGL_OPENGL_BIT 0x0008
#define EGL_SURFACE_TYPE 0x3033
#define EGL_PBUFFER_BIT 0x0001

static void *(*p_eglGetProcAddress)(const char *);
static EGLDisplay (*p_eglGetPlatformDisplay)(EGLenum, void *, const intptr_t *);
static EGLDisplay (*p_eglGetPlatformDisplayEXT)(EGLenum, void *, const EGLint *);
static EGLBoolean (*p_eglQueryDevicesEXT)(EGLint, EGLDeviceEXT *, EGLint *);
static const char *(*p_eglQueryDeviceStringEXT)(EGLDeviceEXT, EGLint);
static EGLBoolean (*p_eglInitialize)(EGLDisplay, EGLint *, EGLint *);
static EGLBoolean (*p_eglTerminate)(EGLDisplay);
static EGLBoolean (*p_eglBindAPI)(EGLenum);
static EGLBoolean (*p_eglChooseConfig)(EGLDisplay, const EGLint *, EGLConfig *, EGLint, EGLint *);
static EGLContext (*p_eglCreateContext)(EGLDisplay, EGLConfig, EGLContext, const EGLint *);
static EGLBoolean (*p_eglDestroyContext)(EGLDisplay, EGLContext);
static EGLBoolean (*p_eglMakeCurrent)(EGLDisplay, EGLSurface, EGLSurface, EGLContext);
static const char *(*p_eglQueryString)(EGLDisplay, EGLint);

#define GLFUNCS(X) \
	X(void, glGenTextures, (GLsizei n, GLuint *t), (n, t)) \
	X(void, glDeleteTextures, (GLsizei n, const GLuint *t), (n, t)) \
	X(void, glBindTexture, (GLenum a, GLuint t), (a, t)) \
	X(void, glActiveTexture, (GLenum a), (a)) \
	X(void, glTexParameteri, (GLenum a, GLenum p, GLint v), (a, p, v)) \
	X(void, glTexImage2D, (GLenum a, GLint l, GLint f, GLsizei w, GLsizei h, GLint b, GLenum fm, GLenum ty, const void *d), (a, l, f, w, h, b, fm, ty, d)) \
	X(void, glTexSubImage2D, (GLenum a, GLint l, GLint x, GLint y, GLsizei w, GLsizei h, GLenum fm, GLenum ty, const void *d), (a, l, x, y, w, h, fm, ty, d)) \
	X(void, glTexImage3D, (GLenum a, GLint l, GLint f, GLsizei w, GLsizei h, GLsizei dp, GLint b, GLenum fm, GLenum ty, const void *d), (a, l, f, w, h, dp, b, fm, ty, d)) \
	X(void, glTexSubImage3D, (GLenum a, GLint l, GLint x, GLint y, GLint z, GLsizei w, GLsizei h, GLsizei dp, GLenum fm, GLenum ty, const void *d), (a, l, x, y, z, w, h, dp, fm, ty, d)) \
	X(void, glGenFramebuffers, (GLsizei n, GLuint *t), (n, t)) \
	X(void, glBindFramebuffer, (GLenum a, GLuint t), (a, t)) \
	X(void, glFramebufferTexture2D, (GLenum a, GLenum at, GLenum tt, GLuint t, GLint l), (a, at, tt, t, l)) \
	X(GLenum, glCheckFramebufferStatus, (GLenum a), (a)) \
	X(void, glViewport, (GLint x, GLint y, GLsizei w, GLsizei h), (x, y, w, h)) \
	X(void, glClearBufferiv, (GLenum b, GLint d, const GLint *v), (b, d, v)) \
	X(void, glClearBufferfv, (GLenum b, GLint d, const GLfloat *v), (b, d, v)) \
	X(void, glEnable, (GLenum a), (a)) \
	X(void, glDisable, (GLenum a), (a)) \
	X(void, glDepthFunc, (GLenum a), (a)) \
	X(void, glDepthMask, (GLubyte a), (a)) \
	X(GLuint, glCreateShader, (GLenum a), (a)) \
	X(void, glShaderSource, (GLuint s, GLsizei n, const GLchar *const *src, const GLint *len), (s, n, src, len)) \
	X(void, glCompileShader, (GLuint s), (s)) \
	X(void, glGetShaderiv, (GLuint s, GLenum p, GLint *v), (s, p, v)) \
	X(void, glGetShaderInfoLog, (GLuint s, GLsizei n, GLsizei *l, GLchar *b), (s, n, l, b)) \
	X(GLuint, glCreateProgram, (void), ()) \
	X(void, glAttachShader, (GLuint p, GLuint s), (p, s)) \
	X(void, glLinkProgram, (GLuint p), (p)) \
	X(void, glGetProgramiv, (GLuint s, GLenum p, GLint *v), (s, p, v)) \
	X(void, glGetProgramInfoLog, (GLuint s, GLsizei n, GLsizei *l, GLchar *b), (s, n, l, b)) \
	X(void, glUseProgram, (GLuint p), (p)) \
	X(GLint, glGetUniformLocation, (GLuint p, const GLchar *n), (p, n)) \
	X(void, glUniform1i, (GLint l, GLint a), (l, a)) \
	X(void, glUniform1ui, (GLint l, GLuint a), (l, a)) \
	X(void, glUniform1f, (GLint l, GLfloat a), (l, a)) \
	X(void, glUniform3f, (GLint l, GLfloat a, GLfloat b, GLfloat c), (l, a, b, c)) \
	X(void, glUniform1iv, (GLint l, GLsizei n, const GLint *v), (l, n, v)) \
	X(void, glGenVertexArrays, (GLsizei n, GLuint *t), (n, t)) \
	X(void, glBindVertexArray, (GLuint t), (t)) \
	X(void, glGenBuffers, (GLsizei n, GLuint *t), (n, t)) \
	X(void, glBindBuffer, (GLenum a, GLuint t), (a, t)) \
	X(void, glBufferData, (GLenum a, GLsizeiptr n, const void *d, GLenum u), (a, n, d, u)) \
	X(void, glVertexAttribPointer, (GLuint i, GLint n, GLenum t, GLubyte nm, GLsizei s, const void *o), (i, n, t, nm, s, o)) \
	X(void, glVertexAttribIPointer, (GLuint i, GLint n, GLenum t, GLsizei s, const void *o), (i, n, t, s, o)) \
	X(void, glEnableVertexAttribArray, (GLuint i), (i)) \
	X(void, glDrawArrays, (GLenum m, GLint f, GLsizei n), (m, f, n)) \
	X(void, glPixelStorei, (GLenum p, GLint v), (p, v)) \
	X(void, glReadPixels, (GLint x, GLint y, GLsizei w, GLsizei h, GLenum f, GLenum t, void *d), (x, y, w, h, f, t, d)) \
	X(GLenum, glGetError, (void), ()) \
	X(void, glFinish, (void), ()) \
	X(void *, glMapBufferRange, (GLenum t, GLintptr o, GLsizeiptr n, GLbitfield a), (t, o, n, a)) \
	X(GLubyte, glUnmapBuffer, (GLenum t), (t)) \
	X(void, glGetIntegerv, (GLenum p, GLint *v), (p, v)) \
	X(const GLubyte *, glGetString, (GLenum p), (p))

#define DECLARE(ret, name, args, call) static ret (*p_##name) args;
GLFUNCS(DECLARE)
#define WRAP(ret, name, args, call) static ret sy_##name args { return p_##name call; }
GLFUNCS(WRAP)

static int sy_load_gl(void) {
#define LOAD(ret, name, args, call) p_##name = (ret (*) args)p_eglGetProcAddress(#name); if (!p_##name) return 0;
	GLFUNCS(LOAD)
	return 1;
}

// sy_attribs points inputs 0 (x, y: floats) and 1 (the face: a whole
// number) at the bound buffer, 12 bytes a corner.
static void sy_attribs(void) {
	p_glVertexAttribPointer(0, 2, 0x1406, 0, 12, (const void *)0);
	p_glEnableVertexAttribArray(0);
	p_glVertexAttribIPointer(1, 1, 0x1404, 12, (const void *)8);
	p_glEnableVertexAttribArray(1);
}

static EGLDisplay sy_dpy;
static EGLContext sy_ctx;

static int has_ext(const char *list, const char *ext) {
	if (!list) return 0;
	size_t n = strlen(ext);
	for (const char *p = list; (p = strstr(p, ext)) != NULL; p += n)
		if ((p == list || p[-1] == ' ') && (p[n] == ' ' || p[n] == 0)) return 1;
	return 0;
}

// sy_context makes an OpenGL 3.3 core context on display d, current with
// no surface. 1 when it works.
static int sy_context(EGLDisplay d) {
	if (!d) return 0;
	EGLint maj, min;
	if (!p_eglInitialize(d, &maj, &min)) return 0;
	if (!p_eglBindAPI(EGL_OPENGL_API)) { p_eglTerminate(d); return 0; }
	EGLConfig cfg = 0;
	const char *ext = p_eglQueryString(d, EGL_EXTENSIONS);
	if (!has_ext(ext, "EGL_KHR_surfaceless_context")) { p_eglTerminate(d); return 0; }
	if (!has_ext(ext, "EGL_KHR_no_config_context") && !has_ext(ext, "EGL_MESA_configless_context")) {
		EGLint want[] = {EGL_RENDERABLE_TYPE, EGL_OPENGL_BIT, EGL_NONE};
		EGLint n = 0;
		if (!p_eglChooseConfig(d, want, &cfg, 1, &n) || n < 1) { p_eglTerminate(d); return 0; }
	}
	EGLint attrs[] = {EGL_CONTEXT_MAJOR_VERSION, 3, EGL_CONTEXT_MINOR_VERSION, 3,
		EGL_CONTEXT_OPENGL_PROFILE_MASK, 1, EGL_NONE};
	EGLContext c = p_eglCreateContext(d, cfg, 0, attrs);
	if (!c) { p_eglTerminate(d); return 0; }
	if (!p_eglMakeCurrent(d, 0, 0, c)) { p_eglDestroyContext(d, c); p_eglTerminate(d); return 0; }
	if (!sy_load_gl()) { p_eglMakeCurrent(d, 0, 0, 0); p_eglDestroyContext(d, c); p_eglTerminate(d); return 0; }
	sy_dpy = d;
	sy_ctx = c;
	return 1;
}

static int is_software(void) {
	const char *r = (const char *)p_glGetString(0x1F01);
	return !r || strstr(r, "llvmpipe") || strstr(r, "softpipe") || strstr(r, "Software");
}

static void sy_drop(void) {
	p_eglMakeCurrent(sy_dpy, 0, 0, 0);
	p_eglDestroyContext(sy_dpy, sy_ctx);
	p_eglTerminate(sy_dpy);
	sy_dpy = 0;
	sy_ctx = 0;
}

// boot_vga is 1 when EGL device d is the card that shows the screen
// (/sys/class/drm/cardN/device/boot_vga), 0 when not, -1 when not known.
static int boot_vga(EGLDeviceEXT d) {
	if (!p_eglQueryDeviceStringEXT) return -1;
	const char *file = p_eglQueryDeviceStringEXT(d, 0x3233); // EGL_DRM_DEVICE_FILE_EXT: /dev/dri/cardN
	if (!file) return -1;
	const char *name = strrchr(file, '/');
	char path[256];
	snprintf(path, sizeof path, "/sys/class/drm/%s/device/boot_vga", name ? name + 1 : file);
	FILE *f = fopen(path, "r");
	if (!f) return -1;
	int c = fgetc(f);
	fclose(f);
	return c == '1' ? 1 : 0;
}

// sy_gl_start makes the context: on the graphics card that shows the
// screen (so the other one, this laptop's NVIDIA card, is not woken),
// else on another card EGL lists, else on Mesa's display with no window,
// but never on the processor (llvmpipe). discrete: the other cards
// first. 1 when it works.
static int sy_gl_start(int discrete) {
	void *lib = dlopen("libEGL.so.1", RTLD_NOW | RTLD_LOCAL);
	if (!lib) return 0;
	p_eglGetProcAddress = dlsym(lib, "eglGetProcAddress");
	if (!p_eglGetProcAddress) return 0;
#define EGLF(name) p_##name = (void *)p_eglGetProcAddress(#name); if (!p_##name) p_##name = dlsym(lib, #name);
	EGLF(eglGetPlatformDisplay) EGLF(eglGetPlatformDisplayEXT) EGLF(eglQueryDevicesEXT) EGLF(eglQueryDeviceStringEXT)
	EGLF(eglInitialize) EGLF(eglTerminate) EGLF(eglBindAPI) EGLF(eglChooseConfig) EGLF(eglCreateContext)
	EGLF(eglDestroyContext) EGLF(eglMakeCurrent) EGLF(eglQueryString)
	if (!p_eglInitialize || !p_eglCreateContext || !p_eglMakeCurrent || !p_eglBindAPI || !p_eglQueryString) return 0;
	EGLDeviceEXT devs[16];
	EGLint n = 0;
	if (p_eglQueryDevicesEXT && p_eglGetPlatformDisplayEXT && !p_eglQueryDevicesEXT(16, devs, &n)) n = 0;
	for (int pass = 0; pass < 2; pass++) {
		for (int i = 0; i < n; i++) {
			if (p_eglQueryDeviceStringEXT && has_ext(p_eglQueryDeviceStringEXT(devs[i], EGL_EXTENSIONS), "EGL_MESA_device_software")) continue;
			int shows = boot_vga(devs[i]) == 1;
			if ((pass == 0) != (shows != (discrete != 0))) continue;
			if (sy_context(p_eglGetPlatformDisplayEXT(EGL_PLATFORM_DEVICE_EXT, devs[i], 0))) {
				if (!is_software()) return 1;
				sy_drop();
			}
		}
	}
	EGLDisplay d = 0;
	if (p_eglGetPlatformDisplay) d = p_eglGetPlatformDisplay(EGL_PLATFORM_SURFACELESS_MESA, 0, 0);
	else if (p_eglGetPlatformDisplayEXT) d = p_eglGetPlatformDisplayEXT(EGL_PLATFORM_SURFACELESS_MESA, 0, 0);
	if (sy_context(d)) {
		if (!is_software()) return 1;
		sy_drop();
	}
	return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"
)

// The OpenGL numbers used.
const (
	glTexture2D        = 0x0DE1
	glTexture2DArray   = 0x8C1A
	glTextureMinFilter = 0x2801
	glTextureMagFilter = 0x2800
	glNearest          = 0x2600
	glTextureWrapS     = 0x2802
	glTextureWrapT     = 0x2803
	glClampToEdge      = 0x812F
	glTextureBaseLevel = 0x813C
	glTextureMaxLevel  = 0x813D
	glR32UI            = 0x8236
	glRedInteger       = 0x8D94
	glUnsignedInt      = 0x1405
	glRGBA32UI         = 0x8D70
	glRGBAInteger      = 0x8D99
	glRGBA32F          = 0x8814
	glRGBA             = 0x1908
	glRGB              = 0x1907
	glFloat            = 0x1406
	glR32F             = 0x822E
	glRed              = 0x1903
	glR8               = 0x8229
	glUnsignedByte     = 0x1401
	glR32I             = 0x8235
	glInt              = 0x1404
	glRGBA8            = 0x8058
	glDepth32F         = 0x8CAC
	glDepthComponent   = 0x1902
	glFramebuffer      = 0x8D40
	glColorAttachment0 = 0x8CE0
	glDepthAttachment  = 0x8D00
	glFramebufferDone  = 0x8CD5
	glDepthTest        = 0x0B71
	glLess             = 0x0201
	glColor            = 0x1800
	glDepth            = 0x1801
	glArrayBuffer      = 0x8892
	glStreamDraw       = 0x88E0
	glTriangles        = 0x0004
	glTexture0         = 0x84C0
	glPackAlignment    = 0x0D05
	glUnpackAlignment  = 0x0CF5
	glVertexShader     = 0x8B31
	glFragmentShader   = 0x8B30
	glCompileStatus    = 0x8B81
	glLinkStatus       = 0x8B82
	glRenderer         = 0x1F01
	glMaxTextureSize   = 0x0D33
	glMaxArrayLayers   = 0x88FF
)

// gpuReq is work for the graphics card's thread.
type gpuReq struct {
	do   func()
	done chan struct{}
}

// gpuThread is the one thread that talks to the graphics card.
var gpuThread struct {
	reqs     chan gpuReq
	ok       bool
	renderer string
}

// gpuStart starts the graphics card's thread and its context (discrete:
// try every card EGL lists before the screen's own). False when there is
// no card to draw on.
func gpuStart(discrete bool) bool {
	ready := make(chan bool)
	go func() {
		runtime.LockOSThread() // the context belongs to this thread, for good
		d := C.int(0)
		if discrete {
			d = 1
		}
		if C.sy_gl_start(d) == 0 {
			ready <- false
			return
		}
		gpuThread.renderer = C.GoString((*C.char)(unsafe.Pointer(C.sy_glGetString(glRenderer))))
		gpuThread.reqs = make(chan gpuReq)
		gpuThread.ok = true
		ready <- true
		for q := range gpuThread.reqs {
			q.do()
			close(q.done)
		}
	}()
	return <-ready
}

// gpuRun runs do on the graphics card's thread and waits for it.
func gpuRun(do func()) {
	q := gpuReq{do: do, done: make(chan struct{})}
	gpuThread.reqs <- q
	<-q.done
}

// gpuRendererName is the card's name, as OpenGL gives it.
func gpuRendererName() string { return gpuThread.renderer }

// ---- the calls, for design_3d_gpu.go -------------------------------------------------------

func glGenTexture() uint32 {
	var t C.GLuint
	C.sy_glGenTextures(1, &t)
	return uint32(t)
}

func glGenFramebuffer() uint32 {
	var t C.GLuint
	C.sy_glGenFramebuffers(1, &t)
	return uint32(t)
}

func glGenVertexArray() uint32 {
	var t C.GLuint
	C.sy_glGenVertexArrays(1, &t)
	return uint32(t)
}

func glGenBuffer() uint32 {
	var t C.GLuint
	C.sy_glGenBuffers(1, &t)
	return uint32(t)
}

func glBindTexture(target, t uint32) { C.sy_glBindTexture(C.GLenum(target), C.GLuint(t)) }
func glActiveTexture(unit int)       { C.sy_glActiveTexture(C.GLenum(glTexture0 + unit)) }
func glTexParameteri(target, p uint32, v int32) {
	C.sy_glTexParameteri(C.GLenum(target), C.GLenum(p), C.GLint(v))
}

// glTexImage2D gives texture target level a picture of w×h, from data
// (nil: room only). data must not hold Go pointers.
func glTexImage2D(target uint32, level int, internal int32, w, h int, format, typ uint32, data unsafe.Pointer) {
	C.sy_glTexImage2D(C.GLenum(target), C.GLint(level), C.GLint(internal), C.GLsizei(w), C.GLsizei(h), 0, C.GLenum(format), C.GLenum(typ), data)
}

func glTexSubImage2D(target uint32, level, x, y, w, h int, format, typ uint32, data unsafe.Pointer) {
	C.sy_glTexSubImage2D(C.GLenum(target), C.GLint(level), C.GLint(x), C.GLint(y), C.GLsizei(w), C.GLsizei(h), C.GLenum(format), C.GLenum(typ), data)
}

func glTexImage3D(target uint32, level int, internal int32, w, h, d int, format, typ uint32, data unsafe.Pointer) {
	C.sy_glTexImage3D(C.GLenum(target), C.GLint(level), C.GLint(internal), C.GLsizei(w), C.GLsizei(h), C.GLsizei(d), 0, C.GLenum(format), C.GLenum(typ), data)
}

func glTexSubImage3D(target uint32, level, x, y, z, w, h, d int, format, typ uint32, data unsafe.Pointer) {
	C.sy_glTexSubImage3D(C.GLenum(target), C.GLint(level), C.GLint(x), C.GLint(y), C.GLint(z), C.GLsizei(w), C.GLsizei(h), C.GLsizei(d), C.GLenum(format), C.GLenum(typ), data)
}

func glBindFramebuffer(fb uint32) { C.sy_glBindFramebuffer(glFramebuffer, C.GLuint(fb)) }
func glFramebufferTexture(attach, tex uint32) {
	C.sy_glFramebufferTexture2D(glFramebuffer, C.GLenum(attach), glTexture2D, C.GLuint(tex), 0)
}
func glFramebufferComplete() bool {
	return C.sy_glCheckFramebufferStatus(glFramebuffer) == glFramebufferDone
}
func glViewport(w, h int) { C.sy_glViewport(0, 0, C.GLsizei(w), C.GLsizei(h)) }
func glClearInt(v int32) {
	c := [4]C.GLint{C.GLint(v), 0, 0, 0}
	C.sy_glClearBufferiv(glColor, 0, &c[0])
}
func glClearDepth(v float32) {
	c := C.GLfloat(v)
	C.sy_glClearBufferfv(glDepth, 0, &c)
}
func glEnable(c uint32)    { C.sy_glEnable(C.GLenum(c)) }
func glDisable(c uint32)   { C.sy_glDisable(C.GLenum(c)) }
func glDepthFunc(f uint32) { C.sy_glDepthFunc(C.GLenum(f)) }
func glUseProgram(p uint32) {
	C.sy_glUseProgram(C.GLuint(p))
}
func glBindVertexArray(v uint32) { C.sy_glBindVertexArray(C.GLuint(v)) }
func glPixelStorei(p uint32, v int) {
	C.sy_glPixelStorei(C.GLenum(p), C.GLint(v))
}
func glGetError() uint32 { return uint32(C.sy_glGetError()) }
func glFinish()          { C.sy_glFinish() }
func glGetInteger(p uint32) int {
	var v C.GLint
	C.sy_glGetIntegerv(C.GLenum(p), &v)
	return int(v)
}

// glVertices fills buffer b with data (floats and whole numbers, 12
// bytes a corner: x, y and the face) and points the program's inputs 0
// and 1 at it.
func glVertices(b uint32, data []byte) {
	C.sy_glBindBuffer(glArrayBuffer, C.GLuint(b))
	C.sy_glBufferData(glArrayBuffer, C.GLsizeiptr(len(data)), unsafe.Pointer(&data[0]), glStreamDraw)
	C.sy_attribs()
}

func glDrawTriangles(n int) { C.sy_glDrawArrays(glTriangles, 0, C.GLsizei(n)) }

// glProgram compiles and links a vertex and a fragment shader.
func glProgram(vs, fs string) (uint32, error) {
	compile := func(kind uint32, src string) (C.GLuint, error) {
		s := C.sy_glCreateShader(C.GLenum(kind))
		cs := C.CString(src)
		defer C.free(unsafe.Pointer(cs))
		C.sy_glShaderSource(s, 1, &cs, nil)
		C.sy_glCompileShader(s)
		var ok C.GLint
		C.sy_glGetShaderiv(s, glCompileStatus, &ok)
		if ok == 0 {
			buf := make([]byte, 8192)
			var n C.GLsizei
			C.sy_glGetShaderInfoLog(s, C.GLsizei(len(buf)), &n, (*C.GLchar)(unsafe.Pointer(&buf[0])))
			return 0, errors.New(strings.TrimSpace(string(buf[:n])))
		}
		return s, nil
	}
	v, err := compile(glVertexShader, vs)
	if err != nil {
		return 0, fmt.Errorf("vertex shader: %w", err)
	}
	f, err := compile(glFragmentShader, fs)
	if err != nil {
		return 0, fmt.Errorf("fragment shader: %w", err)
	}
	p := C.sy_glCreateProgram()
	C.sy_glAttachShader(p, v)
	C.sy_glAttachShader(p, f)
	C.sy_glLinkProgram(p)
	var ok C.GLint
	C.sy_glGetProgramiv(p, glLinkStatus, &ok)
	if ok == 0 {
		buf := make([]byte, 8192)
		var n C.GLsizei
		C.sy_glGetProgramInfoLog(p, C.GLsizei(len(buf)), &n, (*C.GLchar)(unsafe.Pointer(&buf[0])))
		return 0, errors.New("link: " + strings.TrimSpace(string(buf[:n])))
	}
	return uint32(p), nil
}

// glUniforms is a program's uniforms by name, looked up once.
type glUniforms struct {
	prog uint32
	at   map[string]C.GLint
}

func (u *glUniforms) loc(name string) C.GLint {
	if l, ok := u.at[name]; ok {
		return l
	}
	if u.at == nil {
		u.at = map[string]C.GLint{}
	}
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	l := C.sy_glGetUniformLocation(C.GLuint(u.prog), cs)
	u.at[name] = l
	return l
}

func (u *glUniforms) i(name string, v int)     { C.sy_glUniform1i(u.loc(name), C.GLint(v)) }
func (u *glUniforms) ui(name string, v uint32) { C.sy_glUniform1ui(u.loc(name), C.GLuint(v)) }
func (u *glUniforms) f(name string, v float64) { C.sy_glUniform1f(u.loc(name), C.GLfloat(v)) }
func (u *glUniforms) v3(name string, v r3vec) {
	C.sy_glUniform3f(u.loc(name), C.GLfloat(v.x), C.GLfloat(v.y), C.GLfloat(v.z))
}
func (u *glUniforms) iv(name string, v []int32) {
	C.sy_glUniform1iv(u.loc(name), C.GLsizei(len(v)), (*C.GLint)(unsafe.Pointer(&v[0])))
}

// glReadPBO reads the w×h RGBA pixels of the bound framebuffer through
// pixel buffer pbo (which the driver keeps in memory that is quick to
// read, where reading the framebuffer straight is slow), and copies the
// first len(out) bytes of them into out.
func glReadPBO(pbo uint32, w, h int, out []byte) bool {
	n := C.GLsizeiptr(4 * w * h)
	if int(n) < len(out) {
		return false
	}
	C.sy_glBindBuffer(0x88EB, C.GLuint(pbo))  // GL_PIXEL_PACK_BUFFER
	C.sy_glBufferData(0x88EB, n, nil, 0x88E1) // GL_STREAM_READ
	C.sy_glPixelStorei(glPackAlignment, 1)
	C.sy_glReadPixels(0, 0, C.GLsizei(w), C.GLsizei(h), glRGBA, glUnsignedByte, nil)
	p := C.sy_glMapBufferRange(0x88EB, 0, n, 1) // GL_MAP_READ_BIT
	if p == nil {
		C.sy_glBindBuffer(0x88EB, 0)
		return false
	}
	copy(out, unsafe.Slice((*byte)(p), len(out)))
	C.sy_glUnmapBuffer(0x88EB)
	C.sy_glBindBuffer(0x88EB, 0)
	return true
}
