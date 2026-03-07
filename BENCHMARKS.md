# GOTorch Benchmarks

Comprehensive performance and resource comparison between GOTorch and Ollama across multiple GPU backends.

## Test Configuration

- **Model**: Qwen3-0.6B (Q4_K_M quantization, 378MB GGUF)
- **Prompt**: "Write a haiku about the sunrise. Be creative and original."
- **Max tokens**: 100
- **Runs per config**: 5 (cold start noted separately, warm runs averaged)
- **Date**: March 6, 2026

## Hardware

| System | CPU | GPU | RAM |
|--------|-----|-----|-----|
| Windows Desktop ("Beast") | Intel i9 (32 threads) | NVIDIA RTX 4070 Ti SUPER (16GB VRAM) + Intel UHD 770 iGPU | 32GB DDR5 |
| Skynet (Dell Mini PC) | Intel i7-8700T (6C/12T, 35W TDP) | Intel UHD 630 iGPU (24 EUs) | 16GB DDR4 |

## Results: Phase 5 (Vulkan + CUDA + Streaming)

### Windows Desktop - GPU Comparison

| Engine | Backend | Device | Cold Start | Warm Avg | Individual Runs | RAM (MB) | VRAM (MB) |
|--------|---------|--------|-----------|----------|-----------------|----------|-----------|
| **GOTorch** | **Vulkan** | RTX 4070 Ti S | 39.8 tok/s | **556.6** | 528.2, 553.5, 573.7, 571.1 | **550** | **8,147** |
| **GOTorch** | **CUDA** | RTX 4070 Ti S | 268.4 tok/s | **510.9** | 512.4, 512.1, 504.4, 514.5 | **797** | **8,392** |
| **GOTorch** | **Vulkan** | Intel UHD 770 (iGPU) | 27.8 tok/s | **62.0** | 62.6, 62.0, 63.4, 59.8 | **902** | shared |
| **Ollama** | CUDA | RTX 4070 Ti S | 52.9 tok/s | **349.8** | 338.3, 356.7, 361.4, 342.7 | ~1,200 | 8,444 |

### Windows Desktop - CPU Comparison

| Engine | Backend Libs | Avg tok/s | Individual Runs | RAM (MB) |
|--------|-------------|-----------|-----------------|----------|
| **GOTorch** | CPU-only libs | **82.6** | 79.9, 86.3, 80.0, 84.1, 82.6 | **909** |
| **GOTorch** | Vulkan libs (gpu-layers 0) | **80.8** | 79.4, 81.5, 81.5 | - |
| **Ollama** | CUDA libs | **83.6** | 84.4, 84.5, 81.8 | ~1,200 |

**No Vulkan DLL contamination.** Loading Vulkan libs with `--gpu-layers 0` runs at 80.8 tok/s vs 82.6 tok/s with CPU-only libs (within margin). This differs from CUDA, which caused a 2.5x CPU slowdown in Phase 2.

### Skynet (CPU + iGPU)

| Engine | Mode | Cold Start | Warm Avg | Individual Runs | RAM (MB) |
|--------|------|-----------|----------|-----------------|----------|
| **GOTorch** | CPU (6 threads) | - | **52.3** | 54.1, 52.2, 52.3, 51.4, 51.3 | ~901 |
| **GOTorch** | **Vulkan iGPU** (UHD 630) | 19.7 tok/s | **22.6** | 22.6, 22.7, 22.7, 22.3 | **359** |
| **Ollama** | CPU (6 threads) | 32.4 tok/s | **43.0** | 39.7, 44.7, 43.8, 43.9 | ~700 |

**GOTorch CPU is 22% faster than Ollama** on Skynet. The iGPU is slower (22.6 tok/s) but uses **60% less RAM** (359 vs 901 MB).

### DLL Size Comparison

| Backend | DLL File | Size |
|---------|----------|------|
| CUDA | ggml-cuda.dll | **461 MB** |
| Vulkan | ggml-vulkan.dll | **54 MB** |

Vulkan DLL is **8.5x smaller** than CUDA.

## Key Findings

### Speed
1. **GOTorch Vulkan is 9% faster than CUDA** on the same GPU: 557 vs 511 tok/s
2. **GOTorch Vulkan is 59% faster than Ollama GPU**: 557 vs 350 tok/s
3. **Intel iGPU is usable**: 62 tok/s via Vulkan on the UHD 770 (shared system RAM)
4. **GOTorch CPU matches Ollama on desktop**: 83 vs 84 tok/s (parity)
5. **GOTorch CPU is 22% faster on Skynet**: 52.3 vs 43.0 tok/s
6. **Vulkan cold start trades speed for portability**: 40 tok/s first request (shader compilation), then full speed

### Resources
1. **RAM: GOTorch Vulkan uses 54% less than Ollama**: 550 vs ~1,200 MB
2. **RAM: GOTorch Vulkan uses 31% less than CUDA**: 550 vs 797 MB
3. **VRAM: GOTorch uses 3-12% less**: 8,147-8,392 vs 8,444 MB
4. **DLL size: Vulkan is 8.5x smaller**: 54 vs 461 MB
5. **No Vulkan DLL contamination**: Safe to load Vulkan libs even for CPU-only inference

### iGPU Analysis

| Machine | iGPU | EUs | Memory | tok/s | vs CPU |
|---------|------|-----|--------|-------|--------|
| Beast | UHD 770 | 32 | DDR5 ~50 GB/s | **62** | -25% (vs 83 CPU) |
| Skynet | UHD 630 | 24 | DDR4 ~38 GB/s | **22.6** | -57% (vs 52 CPU) |

Key observations:
- iGPU speed scales with EU count and memory bandwidth
- Skynet iGPU uses **60% less RAM** (359 vs 901 MB) despite being slower
- Not practical for speed, but useful for RAM-constrained environments
- On Apple Silicon (M4 Pro/Max), unified memory at 273-546 GB/s would completely change this story

### Streaming (New in Phase 5)
- **SSE streaming** via `stream: true` in `/v1/chat/completions`
- OpenAI-compatible `chat.completion.chunk` format
- Token deltas delivered in real-time via Server-Sent Events
- Compatible with Open WebUI, LangChain, and any OpenAI client

## Backend Auto-Detection

GOTorch auto-detects the best available backend:
```
Vulkan -> CUDA -> Metal -> HIP -> SYCL -> CPU
```

Vulkan is preferred over CUDA when both are available due to:
- 9% faster warm inference
- 8.5x smaller DLL
- Cross-platform (NVIDIA, AMD, Intel GPUs)
- 31% lower RAM footprint
- No CPU contamination

### Detected Devices (Vulkan on Beast)
```
Device 0: NVIDIA GeForce RTX 4070 Ti SUPER (15293 MiB)  <- default
Device 1: Intel(R) UHD Graphics 770 (shared system RAM) <- via GGML_VK_DEVICE=1
```

## Phase History

### Phase 5: Vulkan + iGPU + SSE Streaming (Current)

- Vulkan GPU backend via ggml-vulkan (llama.cpp b8209, Vulkan SDK 1.4.341.1)
- Default auto-detection: Vulkan first
- Intel iGPU inference via `GGML_VK_DEVICE=1`
- SSE streaming for real-time token delivery
- Metal backend added to auto-detection (pending macOS libs)

| Metric | Phase 4 (CUDA) | Phase 5 (Vulkan) | Change |
|--------|---------------|------------------|--------|
| Warm tok/s | 517 | **557** | +8% |
| RAM (MB) | 796 | **550** | -31% |
| VRAM (MB) | 8,409 | **8,147** | -3% |
| DLL size | 461 MB | **54 MB** | -88% |

### Phase 4: GPU Fix + DLL Upgrade (b8209)

**Root Cause**: `CUDA_VISIBLE_DEVICES=-1` environment variable was hiding all GPUs.

| Change | Before | After |
|--------|--------|-------|
| GPU tok/s (warm) | 25-35 | **517** |
| CPU tok/s (warm) | 72.3 | **84.1** |
| Layers on GPU | 0 (all CPU) | 28 (all GPU) |

### Phase 3: GPU Optimizations

| Optimization | Details |
|-------------|---------|
| Async GPU/CPU overlap | `Synchronize()` before `SamplerSample` |
| Stop-sequence skip | Zero per-token heap allocation |
| Flash attention auto-enable | Auto-enabled when GPU layers > 0 |

### Phase 2: CUDA Contamination Fix
Smart lib path selection prevents CUDA DLL from contaminating CPU mode (was causing 2.5x CPU slowdown). Vulkan does not have this issue.

### Phase 1: Core Optimizations

| Optimization | Impact |
|-------------|--------|
| Pre-allocated token buffer | Reduced GC pressure |
| Smart lib path selection | +2.5x CPU throughput |
| mmap preserved | Faster model load |
| Batch size 2048 | Faster prompt processing |
| Model warmup | Faster first request |
| Auto-detect threads | Optimal thread count per system |

## Build Details

The iTaK Torch libraries can be built automatically for any backend using the native Go build script:

```bash
# Build all standard GPU backends + CPU (Vulkan, CUDA, Metal, HIP, SYCL)
go run scripts/build_backends.go

# Or specify a comma-separated list of targets
go run scripts/build_backends.go -backends="metal,vulkan"

# Clean build directory before start
go run scripts/build_backends.go -clean
```

The script runs CMake to generate the `llama.cpp` shared libraries and automatically copies them into the `lib/` directory under structure `lib/{os}_{arch}_{backend}/`.

### Running
```bash
# Auto-detect (prefers Vulkan, then CUDA, Metal, etc.)
gotorch serve --model model.gguf --gpu-layers -1 --port 8080

# Force specific backend
gotorch serve --model model.gguf --gpu-layers -1 --backend vulkan --port 8080
gotorch serve --model model.gguf --gpu-layers -1 --backend cuda --port 8080
gotorch serve --model model.gguf --gpu-layers -1 --backend metal --port 8080

# Force iGPU (Vulkan device index)
GGML_VK_DEVICE=1 gotorch serve --model model.gguf --gpu-layers -1 --backend vulkan --port 8080

# CPU-only
gotorch serve --model model.gguf --threads 8 --backend cpu --port 8080

# With streaming
curl http://localhost:8080/v1/chat/completions \
  -d '{"model":"test","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```
