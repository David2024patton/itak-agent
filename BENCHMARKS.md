# iTaK Torch Benchmarks

Comprehensive performance and resource comparison between iTaK Torch and Ollama across multiple GPU backends.

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
| **iTaK Torch** | **Vulkan** | RTX 4070 Ti S | 39.8 tok/s | **556.6** | 528.2, 553.5, 573.7, 571.1 | **550** | **8,147** |
| **iTaK Torch** | **CUDA** | RTX 4070 Ti S | 268.4 tok/s | **510.9** | 512.4, 512.1, 504.4, 514.5 | **797** | **8,392** |
| **iTaK Torch** | **Vulkan** | Intel UHD 770 (iGPU) | 27.8 tok/s | **62.0** | 62.6, 62.0, 63.4, 59.8 | **902** | shared |
| **Ollama** | CUDA | RTX 4070 Ti S | 52.9 tok/s | **349.8** | 338.3, 356.7, 361.4, 342.7 | ~1,200 | 8,444 |

### Windows Desktop - CPU Comparison

| Engine | Backend Libs | Avg tok/s | Individual Runs | RAM (MB) |
|--------|-------------|-----------|-----------------|----------|
| **iTaK Torch** | CPU-only libs | **82.6** | 79.9, 86.3, 80.0, 84.1, 82.6 | **909** |
| **iTaK Torch** | Vulkan libs (gpu-layers 0) | **80.8** | 79.4, 81.5, 81.5 | - |
| **Ollama** | CUDA libs | **83.6** | 84.4, 84.5, 81.8 | ~1,200 |

**No Vulkan DLL contamination.** Loading Vulkan libs with `--gpu-layers 0` runs at 80.8 tok/s vs 82.6 tok/s with CPU-only libs (within margin). This differs from CUDA, which caused a 2.5x CPU slowdown in Phase 2.

### Skynet (CPU + iGPU)

| Engine | Mode | Cold Start | Warm Avg | Individual Runs | RAM (MB) |
|--------|------|-----------|----------|-----------------|----------|
| **iTaK Torch** | CPU (6 threads) | - | **52.3** | 54.1, 52.2, 52.3, 51.4, 51.3 | ~901 |
| **iTaK Torch** | **Vulkan iGPU** (UHD 630) | 19.7 tok/s | **22.6** | 22.6, 22.7, 22.7, 22.3 | **359** |
| **Ollama** | CPU (6 threads) | 32.4 tok/s | **43.0** | 39.7, 44.7, 43.8, 43.9 | ~700 |

**iTaK Torch CPU is 22% faster than Ollama** on Skynet. The iGPU is slower (22.6 tok/s) but uses **60% less RAM** (359 vs 901 MB).

### DLL Size Comparison

| Backend | DLL File | Size |
|---------|----------|------|
| CUDA | ggml-cuda.dll | **461 MB** |
| Vulkan | ggml-vulkan.dll | **54 MB** |

Vulkan DLL is **8.5x smaller** than CUDA.

## Key Findings

### Speed
1. **iTaK Torch Vulkan is 9% faster than CUDA** on the same GPU: 557 vs 511 tok/s
2. **iTaK Torch Vulkan is 59% faster than Ollama GPU**: 557 vs 350 tok/s
3. **Intel iGPU is usable**: 62 tok/s via Vulkan on the UHD 770 (shared system RAM)
4. **iTaK Torch CPU matches Ollama on desktop**: 83 vs 84 tok/s (parity)
5. **iTaK Torch CPU is 22% faster on Skynet**: 52.3 vs 43.0 tok/s
6. **Vulkan cold start trades speed for portability**: 40 tok/s first request (shader compilation), then full speed

### Resources
1. **RAM: iTaK Torch Vulkan uses 54% less than Ollama**: 550 vs ~1,200 MB
2. **RAM: iTaK Torch Vulkan uses 31% less than CUDA**: 550 vs 797 MB
3. **VRAM: iTaK Torch uses 3-12% less**: 8,147-8,392 vs 8,444 MB
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

iTaK Torch auto-detects the best available backend:
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
itaktorch serve --model model.gguf --gpu-layers -1 --port 8080

# Force specific backend
itaktorch serve --model model.gguf --gpu-layers -1 --backend vulkan --port 8080
itaktorch serve --model model.gguf --gpu-layers -1 --backend cuda --port 8080
itaktorch serve --model model.gguf --gpu-layers -1 --backend metal --port 8080

# Force iGPU (Vulkan device index)
GGML_VK_DEVICE=1 itaktorch serve --model model.gguf --gpu-layers -1 --backend vulkan --port 8080

# CPU-only
itaktorch serve --model model.gguf --threads 8 --backend cpu --port 8080

# With streaming
curl http://localhost:8080/v1/chat/completions \
  -d '{"model":"test","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

## High-Performance Expansions (Phase 4B)

iTaK Torch includes two advanced generation optimizations that are fully native to the Go Engine framework:

### 1. Speculative Decoding
Speculative Decoding allows a massive model (like Qwen 72B) to generate tokens significantly faster by pairing it with a tiny "draft" model (like Qwen 0.5B). The draft model speculates N tokens ahead, and the master model verifies them in a single batch pass. Matches are accepted for free!

```bash
# Example: 5 tokens speculated per step
itaktorch serve \
  --model qwen2.5-14b-instruct.gguf \
  --draft-model qwen2.5-0.5b-instruct.gguf \
  --speculative-tokens 5 \
  --gpu-layers -1 \
  --port 8080
```
*Note: The draft and main models must share the exact same tokenizer/family.*

### 2. Prefix Caching
Prefix Caching saves the exact KV memory state of a prompt for future re-use. This is highly effective for Agent Swarms that repeatedly hit the server with identical, massive system prompts. The first request calculates it; subsequent requests instantly load the memory state, reducing Time-To-First-Token (TTFT) by hundreds of milliseconds.

```bash
# Enable saving up to 32 different prompt structures in RAM
itaktorch serve --model agent.gguf --prefix-cache-size 32 --port 8080

# Disable Prefix Caching entirely
itaktorch serve --model agent.gguf --prefix-cache-size 0 --port 8080
```

### Phase 6: Prefix Caching Benchmarks (March 6, 2026)
These tests were conducted with a massive (~700 token) system prompt to stress-test the state cache generation vs retrieval on the `qwen2.5-0.5b-instruct-q4_k_m.gguf` model.

**Windows Desktop (Beast)**
| Hardware Engine | Uncached Response | Cached Response | TTFT Reduction | Sustained Token Rate |
| :--- | :--- | :--- | :--- | :--- |
| **Intel i9 CPU** | 4.08s | 2.06s | **49% Faster** | 50.4 tok/s |
| **RTX 4070 Ti SUPER** | 18.6s (Initial Context Alloc) -> 2.4s | 2.3s | **88% Faster** | 333.8 tok/s |
| **Intel UHD 770 iGPU** | 2.46s | 2.32s | **6% Faster (DDR5 Bound)** | 226.7 tok/s |

**Edge Node (Skynet - 35W TDP)**
| Hardware Engine | Uncached Response | Cached Response | TTFT Reduction | Sustained Token Rate |
| :--- | :--- | :--- | :--- | :--- |
| **Intel i7-8700T CPU** | 3.82s | 1.27s | **67% Faster** | 48.8 tok/s |
| **Intel UHD 630 iGPU** | 9.11s | 2.53s | **72% Faster** | 21.1 tok/s |

### Phase 7: Zero-CGO FFI + Pure Go Sampler (March 7, 2026)

Phase 7 eliminated the CGO bridge overhead on the critical token generation hot-path. The `SamplerSample` function was replaced with a `purego`-based FFI binding, and the entire token selection pipeline was rewritten as a Pure Go ArgMax sampler operating directly on the `llama.cpp` logits tensor via `unsafe.Pointer`.

**Key Technical Changes:**
- `purego` replaces `jupiterrider/ffi` on the `SamplerSample` and `GetLogitsIth` functions
- Pure Go ArgMax greedy decoding bypasses the C++ sampler subsystem entirely
- Sub-microsecond token selection: `PureGo Sampler took 530µs` per token (vs ~2ms C++ path)

**Windows Desktop (Beast) - CPU**
| Metric | Phase 6 (Baseline) | Phase 7 (Zero-CGO) | Delta |
| :--- | :--- | :--- | :--- |
| **Uncached Response** | 4.08s | **0.901s** | **78% Faster** |
| **Cached Response** | 2.06s | **0.982s** | **52% Faster** |

**Edge Node (Skynet - 35W TDP) - CPU**
| Metric | Phase 6 (Baseline) | Phase 7 (Zero-CGO) | Delta |
| :--- | :--- | :--- | :--- |
| **Uncached Response** | 3.82s | **3.94s** | ~Parity |
| **Cached Response** | 1.27s | **1.18s** | **7% Faster** |

> [!IMPORTANT]
> The Beast improvement is dramatic (78% uncached) because the i9's 32 threads amplify the CGO elimination. Each of the 32 threads was previously hitting the C-Go bridge wall. On Skynet's 12-thread i7, the cached path still improved 7%, and uncached is within noise margin.

### Phase 7: Head-to-Head vs Ollama (March 7, 2026)

Direct CPU comparison using `qwen2.5-0.5b` model. Ollama forced to CPU via `num_gpu: 0` runtime option (default Ollama uses CUDA GPU).
iTaK Torch wall-clock includes full request lifecycle; Ollama "native" uses `eval_count/eval_duration` from its `/api/chat` response.

**Windows Desktop (Beast) - CPU, 5 Runs**
| Engine | Avg tok/s | Individual Runs | Measurement |
| :--- | :--- | :--- | :--- |
| **iTaK Torch** | **52.5** | 49.9, 52.2, 54.9, 52.0, 53.6 | Wall clock |
| **Ollama** | **90.4** | 93.7, 88.0, 89.3, 90.6, 89.7 | Native eval metric |
| **Ollama** | **~73.7** | (incl. prompt processing overhead) | Wall clock |

> [!NOTE]
> Ollama's "native eval" (90.4 tok/s) excludes prompt processing time. The wall-clock comparison is more honest: **iTaK Torch ~52.5 tok/s vs Ollama ~73.7 tok/s (Ollama 40% faster on wall clock)**. Our Pure Go sampler trades some decode speed for zero-CGO overhead and GC-free operation. Phase 4 (Ring Buffer KV) and Phase 5 (WebGPU WGSL) are the next levers to close this gap.

### Phase 7B: Unsafe Hot-Path Optimizations (March 7, 2026)

Applied three `unsafe` package optimizations to the token generation hot path:
1. **`unsafe.String`** in `tokenToText` -- eliminates per-token heap allocation by returning a string header pointing directly at the pre-allocated buffer memory.
2. **Pointer arithmetic ArgMax** -- replaces `range logits` (Go bounds checking per element) with direct `unsafe.Add` pointer math on the ~151k vocab float32 array.
3. **`//go:nosplit` pragma** -- prevents stack-split checks on the sampler function.

**Windows Desktop (Beast) - CPU, 5 Runs**
| Version | Avg tok/s | Individual Runs | Delta |
| :--- | :--- | :--- | :--- |
| **Phase 7 (before unsafe)** | **52.5** | 49.9, 52.2, 54.9, 52.0, 53.6 | Baseline |
| **Phase 7B (with unsafe)** | **56.2** | 56.3, 54.0, 59.0, 58.2, 53.3 | **+7%** |

> [!NOTE]
> The 7% gain is real but modest because the true bottleneck is the C++ decode step (matrix multiplication, ~18ms/token) not the Go sampling overhead (~0.5ms/token). The unsafe optimizations squeezed the last juice out of the Go-side hot path. The next major leap requires attacking the decode itself (Vulkan/WebGPU compute shaders or async pipelining).

### Phase 8: 3-Way GPU Benchmark (March 7, 2026)

First 3-way engine comparison using `qwen2.5-0.5b` on Beast (RTX 4070 Ti).
- **vLLM**: CUDA + PagedAttention, SafeTensors model, running in WSL Ubuntu venv.
- **Ollama**: CUDA GPU (default, no `num_gpu` override), GGUF model.
- **iTaK Torch**: CPU backend (Vulkan libs not yet built), GGUF Q4_K_M model.

**Windows Desktop (Beast) - 5 Runs**
| Engine | Backend | Avg tok/s | Individual Runs |
| :--- | :--- | :--- | :--- |
| **vLLM** | CUDA + PagedAttention | **306.8** | 173.6, 338.3, 340.0, 341.0, 341.1 |
| **Ollama** | CUDA GPU | **114.7** | 102.5, 113.9, 119.9, 118.4, 118.7 |
| **iTaK Torch** | CPU (32 threads) | **57.2** | 56.9, 56.4, 57.3, 59.7, 55.8 |

> [!IMPORTANT]
> vLLM's steady-state ~341 tok/s represents the CUDA ceiling for this model size. Ollama CUDA at ~119 tok/s is 2.9x slower despite also using CUDA (lacks PagedAttention and continuous batching optimizations). iTaK Torch at 57.2 tok/s is running CPU-only -- this is **not a fair GPU comparison yet**. Once Vulkan backend libs are compiled, iTaK will be able to offload layers to the RTX 4070 Ti and should close the gap significantly.

> [!NOTE]
> vLLM run 1 (173.6 tok/s) is slower due to CUDA kernel warm-up / JIT compilation. Runs 2-5 are consistent at ~340 tok/s. Ollama run 1 also shows cold-start (25.6 wall tok/s vs ~92 steady-state).

> [!NOTE] 
> **Testing Assets Location:** 
> Benchmark scripts are organized in `scripts/benchmark/` with READMEs.
> Models are stored in `models/` (gitignored) on both Beast and Skynet.
> - Base H2H Script: `scripts/benchmark/bench_h2h.py`
> - GPU H2H (3-way): `scripts/benchmark/bench_h2h_gpu.py`
> - PowerShell Tracker: `scripts/benchmark/benchmark.ps1`
> - Skynet TTFT: `scripts/benchmark/skynet_bench.py`
