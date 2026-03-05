# GOTorch Benchmarks

Comprehensive performance and resource comparison between GOTorch and Ollama.

## Test Configuration

- **Model**: Qwen3-0.6B (Q4_K_M quantization, 378MB GGUF)
- **Prompt**: "Write a haiku about the sunrise. Be creative and original."
- **Max tokens**: 100
- **Runs per config**: 3 (cold start removed, warm runs averaged)
- **Date**: March 5, 2026

## Hardware

| System | CPU | GPU | RAM |
|--------|-----|-----|-----|
| Windows Desktop | Intel i9 (32 threads) | NVIDIA RTX 4070 Ti SUPER (16GB VRAM) | 32GB |
| Skynet (Dell Mini) | Intel i7-8700T (6C/12T, 35W TDP) | None | 16GB |

## Results: Speed + Resource Comparison

### Windows Desktop

| Engine | Mode | Avg tok/s | tok/s (runs) | RAM (MB) | VRAM (MB) | GPU% |
|--------|------|-----------|-------------|----------|-----------|------|
| **GOTorch** | CPU (8 threads) | **78.3** | 82.6, 80.6, 71.8 | **874** | 0 | 0% |
| **GOTorch** | GPU (all layers) | **242.4** | 140.1*, 243.6, 241.1 | **905** | 10,080 | 38% |
| **Ollama** | GPU (default) | **350.0** | 52.4*, 349.9, 359.9 | ~1,200 | 8,690 | 55% |

*Run 1 includes cold start (model load/CUDA graph warmup)

### Skynet (CPU-Only Comparison)

| Engine | Mode | Avg tok/s | tok/s (runs) | RAM (MB) |
|--------|------|-----------|-------------|----------|
| **GOTorch** | CPU (6 threads) | **50.5** | 58.2, 51.2, 42.1 | ~500 |
| **Ollama** | CPU (6 threads) | **52.7** | 50.4, -, 53.8 | ~700 |

## Key Findings

### Speed
1. **Ollama GPU is faster**: 350 vs 242 tok/s on GPU (Ollama +45%)
2. **GOTorch CPU nearly matches Ollama CPU**: 50.5 vs 52.7 tok/s on Skynet (4% gap)
3. **GOTorch GPU cold start is faster**: 140.1 vs 52.4 tok/s first request (2.7x faster warmup)
4. **Prompt processing**: GOTorch GPU: 9-10ms, GOTorch CPU: 27-44ms

### Resources (Where GOTorch Wins)
1. **RAM: GOTorch uses 30% less** (874 vs ~1200 MB on CPU, 905 vs ~1200 on GPU)
2. **GPU utilization: GOTorch uses 30% less GPU**: 38% vs 55%
3. **VRAM**: GOTorch uses slightly more (10 GB vs 8.7 GB) due to full layer offload

### CUDA DLL Contamination (Fixed in Phase 2)
When `ggml-cuda.dll` is loaded even with `--gpu-layers 0`, CPU throughput drops from **~80 to ~30 tok/s** (2.5x slowdown). Phase 2 added smart lib path selection:
- CPU mode (`--gpu-layers 0`): loads from `./lib/{os}_{arch}/` (CPU-only)
- GPU mode (`--gpu-layers N`): loads from `./lib/{os}_{arch}_cuda/` (with CUDA)

## Phase 1 + 2 Optimizations

| Optimization | Impact |
|-------------|--------|
| Pre-allocated token buffer | -GC pressure, +consistent tok/s |
| Smart lib path selection | +2.5x CPU throughput (fixes CUDA contamination) |
| mmap preserved | Faster model load |
| Batch size 2048 | Faster prompt processing |
| Model warmup | Faster first request |
| Auto-detect threads | Optimal thread count per system |

## Phase 3 Optimizations (GPU Speed)

Target: close the GPU gap from 242 -> 300+ tok/s (Ollama: 350).

| Optimization | Expected Impact | Details |
|-------------|-----------------|---------|
| Async GPU/CPU overlap | +7-15% | `Synchronize()` before `SamplerSample`. Decode returns immediately on CUDA, CPU does batch prep while GPU computes. Based on Ollama PR #11863 (+7% on RTX 4090) |
| Stop-sequence skip | +2-3% | Skip `result.String()` allocation when `params.Stop` is empty (most requests). Eliminates per-token heap allocation of growing output buffer |
| Flash attention auto-enable | +3-5% | Auto-enabled when GPU layers > 0. Previously required explicit `--flash-attn=true` |

**Needs benchmarking on GPU hardware to confirm actual gains.**

## Build Details

### CPU Build
```bash
cmake -B build -DBUILD_SHARED_LIBS=ON -DGGML_CUDA=OFF
cmake --build build --config Release
```

### GPU Build (CUDA)
```bash
cmake -B build -DBUILD_SHARED_LIBS=ON -DGGML_CUDA=ON
cmake --build build --config Release
```

### Running
```bash
# CPU-only (auto-selects CPU libs)
gotorch serve --model model.gguf --threads 8 --port 8080

# GPU (auto-selects CUDA libs)
gotorch serve --model model.gguf --gpu-layers 99 --port 8080
```
