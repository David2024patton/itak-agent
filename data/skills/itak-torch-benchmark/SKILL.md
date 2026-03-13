---
name: itak-torch-benchmark
description: Comprehensive benchmarking suite for iTaK Torch inference engine. Run when performance features are added, models change, or hardware is upgraded. Tests iTaK Torch vs Ollama across Beast (GPU/CPU) and Skynet (CPU/iGPU) with full resource monitoring.
---

# iTaK Torch Benchmark Skill

## Overview

This skill runs the benchmark suite from the iTaK Torch project repo. All scripts and documentation live in `GOAgent/benchmarks/` so Skynet can `git pull` and run them directly.

> [!IMPORTANT]
> The canonical benchmark scripts and README live in the **iTaK Torch repo**:
> `e:\.agent\iTaK Torch\benchmarks\`
>
> This skill file just tells Antigravity when and how to invoke them.

## When to Use

- After implementing new performance features in iTaK Torch
- When upgrading models (new GGUF files)
- When testing on new hardware
- Before publishing performance claims (LinkedIn, docs)

## Engine Matrix

### GPU Engines (Beast only)
| Engine | Backend |
|--------|---------|
| Ollama GPU | Default GPU acceleration |
| iTaK Torch Vulkan | `lib/windows_amd64_vulkan/` |
| iTaK Torch CUDA | `lib/windows_amd64_cuda/` |

### CPU Engines (Beast + Skynet)
| Engine | How |
|--------|-----|
| Ollama CPU | REST API with `num_gpu: 0` forces CPU |
| iTaK Torch CPU | `lib/windows_amd64/` (Beast) or `lib/linux_amd64/` (Skynet) |

> [!NOTE]
> `CUDA_VISIBLE_DEVICES=""` does NOT work for Ollama because it runs as a service.
> The benchmark uses the `/api/generate` endpoint with `"num_gpu": 0` to force true CPU inference.

## Quick Start

### Beast (Windows)

```powershell
# From the iTaK Torch project dir
& "e:\.agent\iTaK Torch\benchmarks\scripts\benchmark_beast.ps1" -Category matrix    # GPU + CPU engines
& "e:\.agent\iTaK Torch\benchmarks\scripts\benchmark_beast.ps1" -Category all       # Everything
& "e:\.agent\iTaK Torch\benchmarks\scripts\benchmark_beast.ps1" -Category unit      # Just tests
```

### Skynet (Ubuntu)

```bash
ssh skynet@192.168.0.217
cd ~/iTaK-Torch
git pull
bash benchmarks/scripts/benchmark_skynet.sh all
```

## Metrics Captured Per Engine

Every engine gets identical treatment:
- **tok/s**: Mean, min, max across N iterations
- **VRAM**: Before/after (MB)
- **GPU Temperature**: Before/after (C)
- **GPU Power Draw**: Before/after (W)
- **System RAM**: Before/after (GB)

## Verified Baseline (March 8, 2026 - Beast)

**Beast**: i9-14900K, 128GB RAM, RTX 4070 Ti SUPER 16GB

| Engine | tok/s | vs Ollama GPU | VRAM (MB) | Power (W) | Temp (C) |
|--------|-------|---------------|-----------|-----------|----------|
| **iTaK Torch Vulkan** | **705.8** | **+12.6%** | 11,511 | 192.8 | 49 |
| iTaK Torch CUDA | 661.2 | +5.5% | 11,500 | 163.5 | 48 |
| Ollama GPU | 626.7 | baseline | 11,597 | 140.8 | 40 |
| iTaK Torch CPU | 108.8 | -82.6% | 10,501 | 17.9 | 33 |
| Ollama CPU | 90.7 | -85.5% | 10,501 | 18.0 | 33 |

> [!NOTE]
> iTaK Torch CPU (108.8 tok/s) beats Ollama CPU (90.7 tok/s) by 20% on CPU-only workloads.

**Skynet**: TBD (pending after Beast scripts are verified)

## Update Checklist

> [!IMPORTANT]
> When new performance features are added to iTaK Torch:

1. Update `benchmarks/scripts/benchmark_beast.ps1` and `benchmark_skynet.sh`
2. Update `benchmarks/README.md` with the new metric
3. Run the full suite to establish new baseline
4. Update the baseline table above

## Full Documentation

See [benchmarks/README.md](file:///e:/.agent/iTaK%20Torch/benchmarks/README.md) for complete documentation including parameters, environment variables, prerequisites, and output format.
