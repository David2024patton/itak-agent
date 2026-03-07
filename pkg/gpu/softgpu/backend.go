// Package softgpu provides a pure Go software compute backend for the gogpu/wgpu HAL.
//
// This backend executes WGSL compute shaders on CPU, providing a zero-dependency
// WebGPU compute path that works without GPU drivers or external libraries.
// It registers itself as a real HAL backend via init(), enabling the full
// wgpu pipeline (Instance -> Adapter -> Device -> Shader -> Pipeline -> Dispatch)
// without falling back to the mock adapter.
//
// Limitations:
//   - Compute-only (no render passes, no surfaces)
//   - WGSL execution is interpreted (not compiled to native)
//   - Single-threaded dispatch (no workgroup parallelism)
//   - Render pipeline / texture operations are stubs
//
// Usage: import _ "github.com/David2024patton/iTaKAgent/pkg/gpu/softgpu"
package softgpu

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

func init() {
	hal.RegisterBackend(&softBackend{})
}

// softBackend implements hal.Backend for the software compute backend.
type softBackend struct{}

func (b *softBackend) Variant() gputypes.Backend { return gputypes.BackendVulkan }

func (b *softBackend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	return &softInstance{}, nil
}

// softInstance implements hal.Instance.
type softInstance struct{}

func (i *softInstance) CreateSurface(_, _ uintptr) (hal.Surface, error) {
	return nil, fmt.Errorf("softgpu: surfaces not supported (compute-only backend)")
}

func (i *softInstance) EnumerateAdapters(_ hal.Surface) []hal.ExposedAdapter {
	return []hal.ExposedAdapter{
		{
			Adapter: &softAdapter{},
			Info: gputypes.AdapterInfo{
				Name:       fmt.Sprintf("iTaKTorch Software Compute (%s/%s)", runtime.GOOS, runtime.GOARCH),
				Vendor:     "iTaKTorch",
				VendorID:   0x6070,
				DeviceID:   0x0001,
				DeviceType: gputypes.DeviceTypeCPU,
				Driver:     "softgpu 1.0.0",
				DriverInfo: fmt.Sprintf("Pure Go software compute, %d cores", runtime.NumCPU()),
				Backend:    gputypes.BackendVulkan,
			},
			Features: 0,
			Capabilities: hal.Capabilities{
				Limits: gputypes.DefaultLimits(),
			},
		},
	}
}

func (i *softInstance) Destroy() {}

// softAdapter implements hal.Adapter.
type softAdapter struct{}

func (a *softAdapter) Open(features gputypes.Features, limits gputypes.Limits) (hal.OpenDevice, error) {
	dev := &softDevice{
		buffers:          make(map[uintptr]*softBuffer),
		shaderModules:    make(map[uintptr]*softShaderModule),
		bindGroupLayouts: make(map[uintptr]*softBindGroupLayout),
		bindGroups:       make(map[uintptr]*softBindGroup),
		pipelineLayouts:  make(map[uintptr]*softPipelineLayout),
		computePipelines: make(map[uintptr]*softComputePipeline),
	}
	q := &softQueue{device: dev}
	return hal.OpenDevice{Device: dev, Queue: q}, nil
}

func (a *softAdapter) TextureFormatCapabilities(_ gputypes.TextureFormat) hal.TextureFormatCapabilities {
	return hal.TextureFormatCapabilities{}
}

func (a *softAdapter) SurfaceCapabilities(_ hal.Surface) *hal.SurfaceCapabilities { return nil }
func (a *softAdapter) Destroy()                                                   {}

// nextID generates unique resource IDs.
var nextID atomic.Uintptr

func newID() uintptr {
	return nextID.Add(1)
}

// softDevice implements hal.Device.
type softDevice struct {
	mu               sync.Mutex
	buffers          map[uintptr]*softBuffer
	shaderModules    map[uintptr]*softShaderModule
	bindGroupLayouts map[uintptr]*softBindGroupLayout
	bindGroups       map[uintptr]*softBindGroup
	pipelineLayouts  map[uintptr]*softPipelineLayout
	computePipelines map[uintptr]*softComputePipeline
}

// Buffer management.

type softBuffer struct {
	id    uintptr
	label string
	data  []byte
	usage gputypes.BufferUsage
}

func (b *softBuffer) Destroy()              {}
func (b *softBuffer) NativeHandle() uintptr { return b.id }

func (d *softDevice) CreateBuffer(desc *hal.BufferDescriptor) (hal.Buffer, error) {
	buf := &softBuffer{
		id:    newID(),
		label: desc.Label,
		data:  make([]byte, desc.Size),
		usage: desc.Usage,
	}
	d.mu.Lock()
	d.buffers[buf.id] = buf
	d.mu.Unlock()
	return buf, nil
}

func (d *softDevice) DestroyBuffer(buffer hal.Buffer) {
	if b, ok := buffer.(*softBuffer); ok {
		d.mu.Lock()
		delete(d.buffers, b.id)
		d.mu.Unlock()
	}
}

// Shader module.

type softShaderModule struct {
	id   uintptr
	wgsl string
}

func (s *softShaderModule) Destroy() {}

func (d *softDevice) CreateShaderModule(desc *hal.ShaderModuleDescriptor) (hal.ShaderModule, error) {
	if desc.Source.WGSL == "" {
		return nil, fmt.Errorf("softgpu: only WGSL shaders are supported")
	}
	sm := &softShaderModule{
		id:   newID(),
		wgsl: desc.Source.WGSL,
	}
	d.mu.Lock()
	d.shaderModules[sm.id] = sm
	d.mu.Unlock()
	return sm, nil
}

func (d *softDevice) DestroyShaderModule(module hal.ShaderModule) {}

// Bind group layout.

type softBindGroupLayout struct {
	id      uintptr
	entries []gputypes.BindGroupLayoutEntry
}

func (l *softBindGroupLayout) Destroy() {}

func (d *softDevice) CreateBindGroupLayout(desc *hal.BindGroupLayoutDescriptor) (hal.BindGroupLayout, error) {
	bgl := &softBindGroupLayout{
		id:      newID(),
		entries: desc.Entries,
	}
	d.mu.Lock()
	d.bindGroupLayouts[bgl.id] = bgl
	d.mu.Unlock()
	return bgl, nil
}

func (d *softDevice) DestroyBindGroupLayout(_ hal.BindGroupLayout) {}

// Bind group.

type softBindGroup struct {
	id      uintptr
	entries []gputypes.BindGroupEntry
}

func (g *softBindGroup) Destroy() {}

func (d *softDevice) CreateBindGroup(desc *hal.BindGroupDescriptor) (hal.BindGroup, error) {
	bg := &softBindGroup{
		id:      newID(),
		entries: desc.Entries,
	}
	d.mu.Lock()
	d.bindGroups[bg.id] = bg
	d.mu.Unlock()
	return bg, nil
}

func (d *softDevice) DestroyBindGroup(_ hal.BindGroup) {}

// Pipeline layout.

type softPipelineLayout struct {
	id uintptr
}

func (l *softPipelineLayout) Destroy() {}

func (d *softDevice) CreatePipelineLayout(desc *hal.PipelineLayoutDescriptor) (hal.PipelineLayout, error) {
	pl := &softPipelineLayout{id: newID()}
	d.mu.Lock()
	d.pipelineLayouts[pl.id] = pl
	d.mu.Unlock()
	return pl, nil
}

func (d *softDevice) DestroyPipelineLayout(_ hal.PipelineLayout) {}

// Compute pipeline.

type softComputePipeline struct {
	id         uintptr
	shader     *softShaderModule
	entryPoint string
}

func (p *softComputePipeline) Destroy() {}

func (d *softDevice) CreateComputePipeline(desc *hal.ComputePipelineDescriptor) (hal.ComputePipeline, error) {
	sm, ok := desc.Compute.Module.(*softShaderModule)
	if !ok {
		return nil, fmt.Errorf("softgpu: invalid shader module type")
	}
	cp := &softComputePipeline{
		id:         newID(),
		shader:     sm,
		entryPoint: desc.Compute.EntryPoint,
	}
	d.mu.Lock()
	d.computePipelines[cp.id] = cp
	d.mu.Unlock()
	return cp, nil
}

func (d *softDevice) DestroyComputePipeline(_ hal.ComputePipeline) {}

// Command encoder.

func (d *softDevice) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	return &softCommandEncoder{device: d}, nil
}

// Fence.

type softFence struct {
	value atomic.Uint64
}

func (f *softFence) Destroy() {}

func (d *softDevice) CreateFence() (hal.Fence, error) {
	return &softFence{}, nil
}

func (d *softDevice) DestroyFence(_ hal.Fence) {}

func (d *softDevice) Wait(fence hal.Fence, value uint64, _ time.Duration) (bool, error) {
	// Software backend completes synchronously, so the fence is always reached.
	return true, nil
}

func (d *softDevice) ResetFence(_ hal.Fence) error             { return nil }
func (d *softDevice) GetFenceStatus(_ hal.Fence) (bool, error) { return true, nil }
func (d *softDevice) WaitIdle() error                          { return nil }
func (d *softDevice) Destroy()                                 {}
func (d *softDevice) FreeCommandBuffer(_ hal.CommandBuffer)    {}

// Stubs for non-compute operations.

func (d *softDevice) CreateTexture(_ *hal.TextureDescriptor) (hal.Texture, error) {
	return nil, fmt.Errorf("softgpu: textures not supported")
}
func (d *softDevice) DestroyTexture(_ hal.Texture) {}
func (d *softDevice) CreateTextureView(_ hal.Texture, _ *hal.TextureViewDescriptor) (hal.TextureView, error) {
	return nil, fmt.Errorf("softgpu: texture views not supported")
}
func (d *softDevice) DestroyTextureView(_ hal.TextureView) {}
func (d *softDevice) CreateSampler(_ *hal.SamplerDescriptor) (hal.Sampler, error) {
	return nil, fmt.Errorf("softgpu: samplers not supported")
}
func (d *softDevice) DestroySampler(_ hal.Sampler) {}
func (d *softDevice) CreateRenderPipeline(_ *hal.RenderPipelineDescriptor) (hal.RenderPipeline, error) {
	return nil, fmt.Errorf("softgpu: render pipelines not supported")
}
func (d *softDevice) DestroyRenderPipeline(_ hal.RenderPipeline) {}
func (d *softDevice) CreateQuerySet(_ *hal.QuerySetDescriptor) (hal.QuerySet, error) {
	return nil, fmt.Errorf("softgpu: query sets not supported")
}
func (d *softDevice) DestroyQuerySet(_ hal.QuerySet) {}
func (d *softDevice) CreateRenderBundleEncoder(_ *hal.RenderBundleEncoderDescriptor) (hal.RenderBundleEncoder, error) {
	return nil, fmt.Errorf("softgpu: render bundles not supported")
}
func (d *softDevice) DestroyRenderBundle(_ hal.RenderBundle) {}
