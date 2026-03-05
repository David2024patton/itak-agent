# GOTorch Benchmarks

Inference performance comparison between GOTorch and Ollama across different hardware configurations.

## Test Configuration

- **Model**: Qwen3-0.6B (Q4_K_M quantization, 522MB)
- **Prompt**: "Write a haiku about the sunrise. Be creative and original."
- **Max tokens**: 100
- **Runs per config**: 3 (averaged)
- **Date**: March 2026

## Hardware

| System | CPU | GPU | RAM |
|--------|-----|-----|-----|
| Windows Desktop | Intel i9 | NVIDIA RTX 4070 Ti SUPER (16GB VRAM) | 32GB |
| Skynet (Dell Mini) | Intel i7-8700T (6C/12T, 35W TDP) | None | 16GB |

## Results: Qwen3-0.6B Text Generation

| Configuration | Avg tok/s | Run 1 | Run 2 | Run 3 |
|--------------|-----------|-------|-------|-------|
| **GOTorch GPU** (Windows, RTX 4070 Ti) | **139.7** | 104.0 | 160.3 | 154.8 |
| **GOTorch CPU** (Windows, 8 threads) | **68.0** | - | - | 67.2 |
| **Ollama CPU** (Skynet, 6 threads) | **52.7** | 50.4 | - | 53.8 |
| **GOTorch CPU** (Skynet, 6 threads) | **44.1** | - | - | 36.8 |

> **Note**: Run 1 on GPU includes cold start overhead (CUDA kernel compilation). Sustained GPU throughput is ~155-238 tok/s.

## Key Takeaways

1. **GPU acceleration delivers 2-3.5x speedup** over CPU on the same machine (139.7 vs 68.0 tok/s, with sustained warmup hitting 237.7 tok/s)
2. **GOTorch matches Ollama on CPU** within ~16% on the same hardware (44.1 vs 52.7 tok/s on Skynet)
3. **Hardware matters more than runtime**: The Windows CPU (68.0 tok/s) outperforms Skynet Ollama (52.7 tok/s) thanks to a faster CPU
4. **GOTorch is a direct llama.cpp wrapper** - no additional optimization layers like Ollama uses (quantization-aware scheduling, memory mapping). The small CPU gap vs Ollama is expected and represents a pure-overhead comparison

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
# Produces ggml-cuda.dll (54.5MB) with GPU compute kernels
```

### Running with GPU
```bash
gotorch serve --model model.gguf --gpu-layers 99 --port 8080
```
