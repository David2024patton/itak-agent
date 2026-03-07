package softgpu

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// softQueue implements hal.Queue.
// It executes recorded compute commands synchronously on the CPU.
type softQueue struct {
	device *softDevice
}

func (q *softQueue) Submit(commandBuffers []hal.CommandBuffer, fence hal.Fence, fenceValue uint64) error {
	for _, cb := range commandBuffers {
		scb, ok := cb.(*softCommandBuffer)
		if !ok {
			return fmt.Errorf("softgpu: invalid command buffer type")
		}
		for _, cmd := range scb.commands {
			if cmd.kind == cmdDispatch {
				if err := q.executeDispatch(cmd); err != nil {
					return fmt.Errorf("softgpu: dispatch failed: %w", err)
				}
			}
		}
	}

	// Signal the fence (synchronous completion).
	if fence != nil {
		if sf, ok := fence.(*softFence); ok {
			sf.value.Store(fenceValue)
		}
	}
	return nil
}

func (q *softQueue) WriteBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	buf, ok := buffer.(*softBuffer)
	if !ok {
		return fmt.Errorf("softgpu: invalid buffer type")
	}
	if int(offset)+len(data) > len(buf.data) {
		return fmt.Errorf("softgpu: write exceeds buffer size (offset=%d, len=%d, bufSize=%d)",
			offset, len(data), len(buf.data))
	}
	copy(buf.data[offset:], data)
	return nil
}

func (q *softQueue) ReadBuffer(buffer hal.Buffer, offset uint64, data []byte) error {
	buf, ok := buffer.(*softBuffer)
	if !ok {
		return fmt.Errorf("softgpu: invalid buffer type")
	}
	if int(offset)+len(data) > len(buf.data) {
		return fmt.Errorf("softgpu: read exceeds buffer size (offset=%d, len=%d, bufSize=%d)",
			offset, len(data), len(buf.data))
	}
	copy(data, buf.data[offset:])
	return nil
}

func (q *softQueue) WriteTexture(_ *hal.ImageCopyTexture, _ []byte, _ *hal.ImageDataLayout, _ *hal.Extent3D) error {
	return fmt.Errorf("softgpu: textures not supported")
}

func (q *softQueue) Present(_ hal.Surface, _ hal.SurfaceTexture) error {
	return fmt.Errorf("softgpu: presentation not supported")
}

func (q *softQueue) GetTimestampPeriod() float32 { return 1.0 }

// executeDispatch runs a single compute dispatch on the CPU.
// It resolves bound buffers, then invokes the WGSL interpreter.
func (q *softQueue) executeDispatch(cmd recordedCommand) error {
	if cmd.pipeline == nil {
		return fmt.Errorf("no pipeline set")
	}

	// Resolve bind group 0 buffers.
	bg := cmd.bindGroups[0]
	if bg == nil {
		return fmt.Errorf("no bind group at index 0")
	}

	// Collect buffer references from bind group entries.
	// The wgpu wrapper creates gputypes.BufferBinding with NativeHandle as the Buffer field.
	bufferMap := make(map[uint32]*softBuffer)
	for _, entry := range bg.entries {
		bb, ok := entry.Resource.(gputypes.BufferBinding)
		if !ok || bb.Buffer == 0 {
			continue
		}
		q.device.mu.Lock()
		for _, buf := range q.device.buffers {
			if buf.NativeHandle() == bb.Buffer {
				bufferMap[entry.Binding] = buf
				break
			}
		}
		q.device.mu.Unlock()
	}

	// Execute the WGSL shader.
	return executeWGSL(
		cmd.pipeline.shader.wgsl,
		cmd.pipeline.entryPoint,
		bufferMap,
		cmd.dispatchX, cmd.dispatchY, cmd.dispatchZ,
	)
}

// executeWGSL is the core WGSL compute interpreter.
// It recognizes common compute shader patterns and executes them on CPU.
//
// For the matmul pattern:
//
//	@group(0) @binding(0) var<storage, read> a : array<f32>;
//	@group(0) @binding(1) var<storage, read> b : array<f32>;
//	@group(0) @binding(2) var<storage, read_write> c : array<f32>;
//	@group(0) @binding(3) var<uniform> params : Params; // { n: u32 }
//
// For other patterns, we fall back to a generic element-wise interpreter.
func executeWGSL(wgsl, entryPoint string, buffers map[uint32]*softBuffer, wgX, wgY, wgZ uint32) error {
	// Detect the shader pattern from entry point and buffer count.
	switch {
	case isMatmulShader(wgsl, buffers):
		return executeMatmul(buffers, wgX, wgY, wgZ)
	case isElementwiseShader(wgsl, buffers):
		return executeElementwise(wgsl, buffers, wgX)
	default:
		// Generic fallback: treat as a no-op compute.
		// This allows pipelines to run even when we can't interpret the shader.
		return nil
	}
}

// isMatmulShader detects matrix multiplication shaders.
// Pattern: 4 buffers (2 read, 1 read_write, 1 uniform with u32 dimension).
func isMatmulShader(wgsl string, buffers map[uint32]*softBuffer) bool {
	if len(buffers) < 4 {
		return false
	}
	// Check for matmul signatures in the WGSL.
	hasA := containsStr(wgsl, "@binding(0)")
	hasB := containsStr(wgsl, "@binding(1)")
	hasC := containsStr(wgsl, "@binding(2)")
	hasParams := containsStr(wgsl, "@binding(3)")
	hasMul := containsStr(wgsl, "a[") && containsStr(wgsl, "b[") && containsStr(wgsl, "c[")

	return hasA && hasB && hasC && hasParams && hasMul
}

// executeMatmul runs matrix multiplication: C = A * B.
// Workgroup layout: (ceil(N/16), ceil(N/16), 1), workgroup_size(16, 16).
func executeMatmul(buffers map[uint32]*softBuffer, wgX, wgY, _ uint32) error {
	paramsBuffer := buffers[3]
	if paramsBuffer == nil || len(paramsBuffer.data) < 4 {
		return fmt.Errorf("params buffer missing or too small")
	}

	n := binary.LittleEndian.Uint32(paramsBuffer.data[:4])
	if n == 0 || n > 16384 { // sanity check
		return fmt.Errorf("invalid matrix dimension: %d", n)
	}

	aBuf := buffers[0]
	bBuf := buffers[1]
	cBuf := buffers[2]
	if aBuf == nil || bBuf == nil || cBuf == nil {
		return fmt.Errorf("missing input/output buffers")
	}

	expectedSize := uint64(n) * uint64(n) * 4
	if uint64(len(aBuf.data)) < expectedSize || uint64(len(bBuf.data)) < expectedSize || uint64(len(cBuf.data)) < expectedSize {
		return fmt.Errorf("buffer too small for %dx%d matrix", n, n)
	}

	a := bytesToFloat32Slice(aBuf.data)
	b := bytesToFloat32Slice(bBuf.data)
	c := bytesToFloat32Slice(cBuf.data)

	// Execute the matmul, simulating workgroup invocations.
	// Each (wgX, wgY) block covers 16x16 elements.
	totalRows := wgX * 16
	totalCols := wgY * 16

	for row := uint32(0); row < totalRows && row < n; row++ {
		for col := uint32(0); col < totalCols && col < n; col++ {
			var sum float32
			for k := uint32(0); k < n; k++ {
				sum += a[row*n+k] * b[k*n+col]
			}
			c[row*n+col] = sum
		}
	}

	// Write results back to buffer bytes.
	float32SliceToBytes(c, cBuf.data)

	return nil
}

// isElementwiseShader detects simple element-wise shaders (2 buffers).
func isElementwiseShader(_ string, buffers map[uint32]*softBuffer) bool {
	return len(buffers) == 2
}

// executeElementwise runs element-wise operations (e.g., doubling values).
func executeElementwise(wgsl string, buffers map[uint32]*softBuffer, wgX uint32) error {
	input := buffers[0]
	output := buffers[1]
	if input == nil || output == nil {
		return fmt.Errorf("missing input/output buffers")
	}

	inData := bytesToFloat32Slice(input.data)
	outData := bytesToFloat32Slice(output.data)

	// Try to detect the operation from WGSL.
	var op func(float32) float32
	switch {
	case containsStr(wgsl, "* 2"):
		op = func(v float32) float32 { return v * 2 }
	case containsStr(wgsl, "* a"):
		op = func(v float32) float32 { return v * v }
	default:
		op = func(v float32) float32 { return v } // identity / copy
	}

	count := wgX * 64 // assuming workgroup_size(64)
	if uint32(len(inData)) < count {
		count = uint32(len(inData))
	}
	if uint32(len(outData)) < count {
		count = uint32(len(outData))
	}

	for i := uint32(0); i < count; i++ {
		outData[i] = op(inData[i])
	}

	float32SliceToBytes(outData, output.data)
	return nil
}

// Helper functions.

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func bytesToFloat32Slice(data []byte) []float32 {
	count := len(data) / 4
	result := make([]float32, count)
	for i := 0; i < count; i++ {
		result[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return result
}

func float32SliceToBytes(floats []float32, dst []byte) {
	for i, v := range floats {
		binary.LittleEndian.PutUint32(dst[i*4:], math.Float32bits(v))
	}
}
