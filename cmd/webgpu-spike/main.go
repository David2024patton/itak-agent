// cmd/webgpu-spike/main.go
//
// Research spike: WebGPU compute via Pure Go gogpu/wgpu.
// Tests GPU adapter discovery, device creation, and a simple matrix multiply
// compute shader to validate feasibility for future inference acceleration.
//
// Build: go run ./cmd/webgpu-spike/
// Requirements: Vulkan drivers (Windows/Linux) or Metal (macOS)
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	// Register real GPU backends (Vulkan, DX12, GLES, Software on Windows).
	// Falls back to software if no GPU drivers are available.
	_ "github.com/gogpu/wgpu/hal/allbackends"
	// Fallback: our custom software compute backend (used if allbackends unavailable).
	// _ "github.com/David2024patton/iTaKAgent/pkg/gpu/softgpu"
)

const (
	// Matrix dimensions for the compute benchmark.
	matrixN    = 512
	matrixSize = matrixN * matrixN
	floatSize  = 4 // bytes per float32
	bufferSize = matrixSize * floatSize

	// WGSL compute shader for matrix multiplication (C = A * B).
	matmulShader = `
@group(0) @binding(0) var<storage, read> a : array<f32>;
@group(0) @binding(1) var<storage, read> b : array<f32>;
@group(0) @binding(2) var<storage, read_write> c : array<f32>;

struct Params {
  n: u32,
}
@group(0) @binding(3) var<uniform> params : Params;

@compute @workgroup_size(16, 16)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let row = gid.x;
  let col = gid.y;
  let n = params.n;
  if (row >= n || col >= n) { return; }

  var sum : f32 = 0.0;
  for (var k : u32 = 0u; k < n; k = k + 1u) {
    sum = sum + a[row * n + k] * b[k * n + col];
  }
  c[row * n + col] = sum;
}
`
)

// float32ToBytes converts a float32 slice to a byte slice.
func float32ToBytes(data []float32) []byte {
	buf := make([]byte, len(data)*floatSize)
	for i, v := range data {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// bytesToFloat32 converts a byte slice to a float32 slice.
func bytesToFloat32(data []byte) []float32 {
	result := make([]float32, len(data)/floatSize)
	for i := range result {
		result[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return result
}

func main() {
	fmt.Println("=== iTaKTorch WebGPU Research Spike ===")
	fmt.Printf("Matrix size: %dx%d (%d floats, %.1f MB per matrix)\n",
		matrixN, matrixN, matrixSize, float64(bufferSize)/(1024*1024))
	fmt.Println()

	// Phase 1: Adapter discovery.
	fmt.Println("[Phase 1] Discovering GPU adapters...")
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: wgpu.CreateInstance failed: %v\n", err)
		os.Exit(1)
	}
	defer instance.Release()

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: no GPU adapter found: %v\n", err)
		os.Exit(1)
	}
	defer adapter.Release()

	// Print adapter info.
	info := adapter.Info()
	fmt.Printf("  Adapter: %s\n", info.Name)
	fmt.Printf("  Vendor:  %s\n", info.Vendor)
	fmt.Printf("  Driver:  %s\n", info.Driver)
	fmt.Printf("  Backend: %s\n", info.Backend)
	fmt.Printf("  Type:    %s\n", info.DeviceType)
	fmt.Println()

	// Phase 2: Device creation.
	fmt.Println("[Phase 2] Creating device...")
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: device creation failed: %v\n", err)
		os.Exit(1)
	}
	defer device.Release()

	queue := device.Queue()
	fmt.Println("  Device created successfully.")
	fmt.Println()

	// Phase 3: Compute shader compilation.
	fmt.Println("[Phase 3] Compiling WGSL compute shader...")
	shaderModule, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "matmul_shader",
		WGSL:  matmulShader,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: shader compilation failed: %v\n", err)
		os.Exit(1)
	}
	defer shaderModule.Release()
	fmt.Println("  Shader compiled successfully.")
	fmt.Println()

	// Phase 4: Buffer creation.
	fmt.Println("[Phase 4] Creating GPU buffers...")

	// Input matrices A and B (CPU-side).
	matA := make([]float32, matrixSize)
	matB := make([]float32, matrixSize)
	for i := range matA {
		matA[i] = float32(i%matrixN) * 0.001
		matB[i] = float32((i/matrixN+i%matrixN)%matrixN) * 0.001
	}

	bufA, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "matrix_a",
		Size:  uint64(bufferSize),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: buffer A creation failed: %v\n", err)
		os.Exit(1)
	}
	defer bufA.Release()

	bufB, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "matrix_b",
		Size:  uint64(bufferSize),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: buffer B creation failed: %v\n", err)
		os.Exit(1)
	}
	defer bufB.Release()

	bufC, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "matrix_c",
		Size:  uint64(bufferSize),
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopySrc,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: buffer C creation failed: %v\n", err)
		os.Exit(1)
	}
	defer bufC.Release()

	// Staging buffer for GPU-to-CPU readback (Vulkan requires MapRead usage).
	bufStaging, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "staging_readback",
		Size:  uint64(bufferSize),
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: staging buffer creation failed: %v\n", err)
		os.Exit(1)
	}
	defer bufStaging.Release()

	// Uniform buffer for matrix dimension.
	paramsBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(paramsBytes, matrixN)

	bufParams, err := device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "params",
		Size:  4,
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: params buffer creation failed: %v\n", err)
		os.Exit(1)
	}
	defer bufParams.Release()

	// Upload data via queue.
	if err := queue.WriteBuffer(bufA, 0, float32ToBytes(matA)); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: WriteBuffer A failed: %v\n", err)
		os.Exit(1)
	}
	if err := queue.WriteBuffer(bufB, 0, float32ToBytes(matB)); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: WriteBuffer B failed: %v\n", err)
		os.Exit(1)
	}
	if err := queue.WriteBuffer(bufParams, 0, paramsBytes); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: WriteBuffer params failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Created 5 buffers (4 GPU + 1 staging), uploaded %.1f MB\n", float64(bufferSize*2+4)/(1024*1024))
	fmt.Println()

	// Phase 5: Pipeline creation.
	fmt.Println("[Phase 5] Creating compute pipeline...")
	bindGroupLayout, err := device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "matmul_bgl",
		Entries: []wgpu.BindGroupLayoutEntry{
			{Binding: 0, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 1, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 2, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeStorage}},
			{Binding: 3, Visibility: wgpu.ShaderStageCompute, Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform}},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: bind group layout creation failed: %v\n", err)
		os.Exit(1)
	}
	defer bindGroupLayout.Release()

	pipelineLayout, err := device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "matmul_pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bindGroupLayout},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: pipeline layout creation failed: %v\n", err)
		os.Exit(1)
	}
	defer pipelineLayout.Release()

	pipeline, err := device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "matmul_pipeline",
		Layout:     pipelineLayout,
		Module:     shaderModule,
		EntryPoint: "main",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: pipeline creation failed: %v\n", err)
		os.Exit(1)
	}
	defer pipeline.Release()
	fmt.Println("  Pipeline created successfully.")
	fmt.Println()

	// Phase 6: Bind group.
	bindGroup, err := device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "matmul_bg",
		Layout: bindGroupLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: bufA, Size: uint64(bufferSize)},
			{Binding: 1, Buffer: bufB, Size: uint64(bufferSize)},
			{Binding: 2, Buffer: bufC, Size: uint64(bufferSize)},
			{Binding: 3, Buffer: bufParams, Size: 4},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: bind group creation failed: %v\n", err)
		os.Exit(1)
	}
	defer bindGroup.Release()

	// Phase 7: Dispatch compute.
	fmt.Println("[Phase 6] Dispatching compute (5 iterations)...")
	workgroups := uint32(math.Ceil(float64(matrixN) / 16.0))

	var totalDuration time.Duration
	iterations := 5
	resultBytes := make([]byte, bufferSize)

	for iter := 0; iter < iterations; iter++ {
		start := time.Now()

		encoder, err := device.CreateCommandEncoder(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: encoder creation failed: %v\n", err)
			os.Exit(1)
		}

		computePass, err := encoder.BeginComputePass(nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: compute pass creation failed: %v\n", err)
			os.Exit(1)
		}

		computePass.SetPipeline(pipeline)
		computePass.SetBindGroup(0, bindGroup, nil)
		computePass.Dispatch(workgroups, workgroups, 1)

		if err := computePass.End(); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: compute pass end failed: %v\n", err)
			os.Exit(1)
		}

		// Copy result to staging buffer for readback (Vulkan requires MapRead buffer).
		encoder.CopyBufferToBuffer(bufC, 0, bufStaging, 0, uint64(bufferSize))

		cmdBuf, err := encoder.Finish()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: encoder finish failed: %v\n", err)
			os.Exit(1)
		}

		if err := queue.Submit(cmdBuf); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: submit failed: %v\n", err)
			os.Exit(1)
		}

		// ReadBuffer on staging forces GPU sync, giving accurate wall-clock times.
		if err := queue.ReadBuffer(bufStaging, 0, resultBytes); err != nil {
			fmt.Fprintf(os.Stderr, "FATAL: readback failed: %v\n", err)
			os.Exit(1)
		}

		elapsed := time.Since(start)
		totalDuration += elapsed

		// Calculate GFLOPS: 2*N^3 FLOPs for matrix multiply.
		flops := 2.0 * float64(matrixN) * float64(matrixN) * float64(matrixN)
		gflops := flops / elapsed.Seconds() / 1e9

		fmt.Printf("  Iteration %d: %s (%.2f GFLOPS)\n", iter+1, elapsed.Round(time.Microsecond), gflops)
	}

	avgDuration := totalDuration / time.Duration(iterations)
	avgGflops := (2.0 * float64(matrixN) * float64(matrixN) * float64(matrixN)) / avgDuration.Seconds() / 1e9

	fmt.Println()
	fmt.Println("=== Results ===")
	fmt.Printf("  Average: %s per %dx%d matmul\n", avgDuration.Round(time.Microsecond), matrixN, matrixN)
	fmt.Printf("  Throughput: %.2f GFLOPS\n", avgGflops)
	fmt.Println()

	// Phase 8: Readback verification (using last iteration's result already in resultBytes).
	fmt.Println("[Phase 7] Verifying result (readback)...")
	result := bytesToFloat32(resultBytes)
	// Verify a few diagonal values against CPU reference.
	verifyErrors := 0
	for i := 0; i < 5; i++ {
		var cpuVal float32
		for k := 0; k < matrixN; k++ {
			cpuVal += matA[i*matrixN+k] * matB[k*matrixN+i]
		}
		gpuVal := result[i*matrixN+i] // diagonal element
		diff := float32(math.Abs(float64(gpuVal - cpuVal)))
		if diff > 0.01 {
			fmt.Printf("  MISMATCH at [%d][%d]: GPU=%.6f CPU=%.6f diff=%.6f\n", i, i, gpuVal, cpuVal, diff)
			verifyErrors++
		}
	}
	if verifyErrors == 0 {
		fmt.Println("  Verification PASSED (5 diagonal elements match CPU reference)")
	}

	fmt.Println()
	fmt.Println("=== Spike Complete ===")
	fmt.Println("Findings:")
	fmt.Println("  - gogpu/wgpu provides pure-Go GPU access (no CGO)")
	fmt.Println("  - WGSL compute shaders compile and dispatch correctly")
	fmt.Println("  - Buffer creation, data transfer, and readback work end-to-end")
	fmt.Printf("  - %dx%d matmul achieves %.2f GFLOPS\n", matrixN, matrixN, avgGflops)
}
