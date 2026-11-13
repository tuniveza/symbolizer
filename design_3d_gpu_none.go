//go:build !(linux && cgo)

package main

// Without cgo (or off Linux) there is no link to the graphics card: the
// 3D room is always drawn on the processor (see design_3d_gpu.go).

var room3DGPU = false

func gpuReady() bool { return false }

func (r *room3D) gpuDraw(c *r3shade, rm *r3room) bool { return false }
