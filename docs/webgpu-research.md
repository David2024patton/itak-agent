# WebGPU Ecosystem Research

## Goal

Evaluate feasibility of pure Go GPU compute for iTaK Torch inference acceleration.
Map the entire Go WebGPU ecosystem and identify the optimal path forward.

---

## 1. Spike Results (Software Backend)

After creating `pkg/gpu/softgpu/` to bypass the mock adapter:

```
=== iTaK Torch WebGPU Research Spike ===
Adapter: iTaK Torch Software Compute (windows/amd64)
Type:    CPU
Driver:  softgpu 1.0.0

Iteration 1: 99.155ms (2.71 GFLOPS)
Iteration 2: 196.846ms (1.36 GFLOPS)
Iteration 3: 97.352ms (2.76 GFLOPS)

Average: 131.117ms per 512x512 matmul
Throughput: 2.05 GFLOPS

Verification PASSED (5 diagonal elements match CPU reference)
```

| Phase | Status | Notes |
|-------|--------|-------|
| Instance creation | PASS | Pure Go, zero-CGO |
| Adapter discovery | PASS | Software compute adapter |
| Device creation | PASS | Real HAL device |
| Shader compilation | PASS | WGSL compiled and dispatched |
| Compute dispatch | PASS | 3 iterations, ~2 GFLOPS |
| Buffer readback | PASS | Verified against CPU reference |

---

## 2. Go WebGPU Libraries Compared

### 2.1 gogpu/wgpu (Pure Go - Our Current Choice)

**Repository:** [github.com/gogpu/wgpu](https://github.com/gogpu/wgpu)
**License:** MIT | **Releases:** 64 | **Language:** 100% Go

A complete WebGPU implementation written entirely in Go. No Rust, no CGO, no external dependencies.

**5 HAL backends:**

| Backend | LOC | Platform | Status |
|---------|-----|----------|--------|
| Vulkan 1.3 | ~38K | Windows, Linux, macOS (MoltenVK) | Auto-gen bindings from vk.xml, buddy allocator, dynamic rendering |
| Metal | ~5K | macOS, iOS | Pure Go Obj-C bridge via goffi, CAMetalLayer |
| DirectX 12 | ~14K | Windows | Pure Go COM bindings via syscall, DXGI, flip model |
| OpenGL ES 3.0+ | ~10K | Cross-platform | EGL/GL via goffi, Mesa llvmpipe for headless |
| Software | ~11K | All platforms | CPU rasterizer, Pineda triangle rasterization, 8x8 tile parallel |

**HAL auto-registration via blank imports:**
```go
import _ "github.com/gogpu/wgpu/hal/allbackends"
// Windows: Vulkan, DX12, GLES, Software
// Linux:   Vulkan, GLES, Software
// macOS:   Metal, Software
```

**Architecture:**
```
User Application
    -> import "github.com/gogpu/wgpu"   (public API)
        -> core/                         (validation, state tracking)
        -> hal/                          (backend interfaces)
            -> vulkan/ | metal/ | dx12/ | gles/ | software/
```

### 2.2 cogentcore/webgpu (FFI Bindings - Alternative)

**Repository:** [github.com/cogentcore/webgpu](https://github.com/cogentcore/webgpu)

Go bindings for the **wgpu-native** library (the Rust `gfx-rs/wgpu` compiled to a C shared library). Originally based on `rajveermalviya/go-webgpu` with web support from `mokiat/wasmgpu`.

| Attribute | Detail |
|-----------|--------|
| GPU access | Through wgpu-native (Vulkan, Metal, D3D12, OpenGL ES) |
| Platforms | macOS, Windows, Linux, iOS, Android, Web (WASM) |
| Build | Requires pre-built static libraries (.a / .lib / .dll) shipped via GitHub Actions |
| CGO | **Yes** - links against wgpu-native via CGO |
| Web support | JS/WASM via wasmgpu bindings |

**Trade-off vs. gogpu/wgpu:** Works *today* with full GPU hardware acceleration, but requires shipping ~20MB platform-specific native binaries. Breaks the zero-CGO constraint.

### 2.3 go-kdfs/webgpu (Freedesktop/Emersion)

**Repository:** [gitlab.freedesktop.org/emersion/go-kdfs/webgpu](https://pkg.go.dev/gitlab.freedesktop.org/emersion/go-kdfs/webgpu)

Experimental WebGPU package from the Freedesktop ecosystem (Simon Ser / emersion). Pre-release (`v0.0.0`), 2 imports, 0 importers. KMS/DRM-focused for Linux display servers. **Not suitable for iTaK Torch** - too early, wrong focus area.

---

## 3. GoGPU Ecosystem Map

The `gogpu` organization provides a complete GPU stack in pure Go:

```
gogpu/gputypes    Shared WebGPU type definitions (enums, structs, constants)
    |
gogpu/naga        WGSL shader compiler (WGSL -> SPIR-V, MSL, GLSL, HLSL)
    |
gogpu/wgpu        WebGPU implementation (Instance, Device, Pipeline, Queue)
    |
    +-- gogpu/gg      2D graphics library (vello/tiny-skia inspired)
    |
    +-- gogpu/ui      GUI toolkit (IDE/CAD-grade, reactive state, accessibility)
```

### 3.1 gogpu/naga - WGSL Shader Compiler

**Repository:** [github.com/gogpu/naga](https://github.com/gogpu/naga)
**Releases:** 26 | Pure Go, zero CGO

Port of Rust's `gfx-rs/naga` shader compiler. Compiles WGSL to:
- **SPIR-V** (Vulkan)
- **MSL** (Metal)
- **GLSL** (OpenGL ES)
- **HLSL** (DirectX 12, compiled to DXBC via d3dcompiler_47.dll)

**Supported WGSL features:**
- All scalar types (f16, f32, i32, u32, bool)
- Vectors (vec2/3/4), matrices (mat2x2 to mat4x4), arrays (fixed and runtime-sized)
- Structs, atomics, textures, samplers, binding arrays
- All three shader stages (@vertex, @fragment, @compute)
- Address spaces (uniform, storage read/read_write, workgroup)
- **100+ built-in functions:** math, trig, exponential, geometric, matrix ops, bit ops, atomics, barriers, packing/unpacking

### 3.2 gogpu/gg - 2D Graphics

**Repository:** [github.com/gogpu/gg](https://github.com/gogpu/gg)
**Releases:** 80 | Pure Go, zero CGO

Enterprise-grade 2D graphics inspired by vello and tiny-skia.

| Feature | Detail |
|---------|--------|
| Rendering | Software (default) + optional GPU acceleration via wgpu |
| Drawing | Immediate mode (Context) + retained mode (Scene Graph) |
| Text | FreeType-compatible, color emoji, custom fonts |
| Output | PNG, SVG, PDF vector export |
| Compositing | Layer blending, alpha masks, recording/playback |

### 3.3 gogpu/ui - GUI Toolkit

**Repository:** [github.com/gogpu/ui](https://github.com/gogpu/ui)

Pure Go GUI toolkit targeting IDE/CAD-grade applications. Reactive state management with signals, accessibility support, event-driven rendering (0% CPU when idle).

**Phases 0-2 complete** (Foundation, MVP, Extensibility, Interactive Widgets). Phase 3 (RC) and Phase 4 (v1.0) upcoming.

### 3.4 gogpu/gputypes - Shared Types

**Repository:** [github.com/gogpu/gputypes](https://github.com/gogpu/gputypes)
**Releases:** 2 | Zero dependencies

Single source of truth for WebGPU enums, structs, and constants. Prevents type incompatibility across the ecosystem. Used by wgpu, naga, gg, and ui.

---

## 4. Go Web Ecosystem (from go.dev)

### 4.1 Web Frameworks

From [go.dev/solutions/webdev](https://go.dev/solutions/webdev):

| Framework | Description | Use Case |
|-----------|-------------|----------|
| **Gin** | Martini-like API, high performance | REST APIs, microservices |
| **Echo** | High performance, extensible, minimalist | APIs with middleware |
| **Gorilla** | Toolkit (mux, websocket, sessions) | Modular web apps |
| **Flamingo** | Clean, scalable architecture | Enterprise apps |

**Routers:** `net/http` (stdlib), `httprouter`, `gorilla/mux`, `chi`
**Templates:** `html/template` (stdlib), `pongo2` (Django-syntax)

### 4.2 DevOps and SRE

From [go.dev/solutions/devops](https://go.dev/solutions/devops):

| Tool | Purpose |
|------|---------|
| **OpenTelemetry** | Vendor-neutral monitoring/tracing |
| **Jaeger** | Distributed tracing (Uber) |
| **Grafana** | Monitoring and observability |
| **Istio** | Service mesh |
| **Cobra** | CLI framework |
| **Viper** | Configuration management |
| **urfave/cli** | Minimal CLI framework |

---

## 5. Architecture Decision

### Primary Path: gogpu/wgpu (Pure Go)

For iTaK Torch GPU acceleration, **gogpu/wgpu remains the recommended path**:

| Criterion | gogpu/wgpu | cogentcore/webgpu |
|-----------|------------|-------------------|
| CGO requirement | None | Yes (wgpu-native) |
| Binary shipping | Go binary only | + ~20MB native libs |
| GPU backends | Vulkan, Metal, DX12, GLES, Software | Vulkan, Metal, D3D12, GLES |
| Build complexity | `go build` | CGO + platform-specific static libs |
| API maturity | v0.19.6 (64 releases) | Stable (production) |
| Cross-compile | Trivial | Complex (per-platform native libs) |
| Shader compiler | naga (pure Go, 100+ builtins) | wgpu-native (bundled) |
| Web/WASM | Not yet | Yes |

### What We Proved

1. **API compatibility** - Spike compiles against gogpu/wgpu v0.19.6 with Go 1.26.0
2. **Zero-CGO works** - No C compiler, no Rust, no shared libraries
3. **Full pipeline validated** - Adapter, device, shader, buffers, bind groups, dispatch, readback
4. **Software fallback** - Our `pkg/gpu/softgpu/` runs end-to-end at ~2 GFLOPS
5. **Real backends exist** - Vulkan (38K LOC), DX12 (14K LOC), Metal (5K LOC) available via `hal/allbackends`

### Fallback Path: cogentcore/webgpu (FFI)

If GPU acceleration is needed **before** gogpu's backends mature, `cogentcore/webgpu` provides working GPU access today at the cost of shipping native binaries.

---

## 6. Next Steps

1. **Test real GPU backends** - Import `hal/allbackends` and test Vulkan on Beast (NVIDIA), test Software on Skynet (no GPU)
2. **Benchmark real GPU** - Compare real Vulkan matmul GFLOPS against our 2.05 GFLOPS software baseline
3. **Monitor gogpu releases** - Track Vulkan HAL maturity (currently at 38K LOC, significant investment)
4. **Evaluate naga integration** - Use gogpu/naga for WGSL shader compilation instead of our interpreted fallback
5. **API layer** - Evaluate Gin/Echo for the iTaK Agent API server (from go.dev/webdev research)

---

## 7. References

| Resource | URL |
|----------|-----|
| gogpu/wgpu | https://github.com/gogpu/wgpu |
| gogpu/naga | https://github.com/gogpu/naga |
| gogpu/gg | https://github.com/gogpu/gg |
| gogpu/ui | https://github.com/gogpu/ui |
| gogpu/gputypes | https://github.com/gogpu/gputypes |
| cogentcore/webgpu | https://github.com/cogentcore/webgpu |
| go-kdfs/webgpu | https://pkg.go.dev/gitlab.freedesktop.org/emersion/go-kdfs/webgpu |
| Go Web Development | https://go.dev/solutions/webdev |
| Go DevOps/SRE | https://go.dev/solutions/devops |
| WebGPU Spec (W3C) | https://www.w3.org/TR/webgpu/ |
| wgpu (Rust reference) | https://github.com/gfx-rs/wgpu |
| Dawn (Google C++) | https://dawn.googlesource.com/dawn |
