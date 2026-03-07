// Package compute provides GPU-accelerated tensor operations using WebGPU (gogpu/wgpu).
//
// It wraps the wgpu Instance/Adapter/Device/Queue lifecycle and provides
// high-level tensor operations (MatMul, Softmax, RMSNorm, etc.) backed by
// WGSL compute shaders. All operations are pure Go with zero CGO dependencies.
//
// Usage:
//
//	dev, err := compute.NewDevice()
//	defer dev.Release()
//
//	a := compute.NewTensor(dev, []int{512, 512}, dataA)
//	b := compute.NewTensor(dev, []int{512, 512}, dataB)
//	c := compute.MatMul(dev, a, b)
//	result := c.ToCPU()
package compute

import (
	"fmt"
	"sync"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	// Register platform-native GPU backends.
	_ "github.com/gogpu/wgpu/hal/allbackends"
)

// Device manages the GPU lifecycle and caches compiled pipelines.
type Device struct {
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue

	// Adapter info for diagnostics.
	AdapterName string
	Backend     string
	DeviceType  string

	// Pipeline cache: shader source hash -> compiled pipeline.
	pipelines  map[string]*wgpu.ComputePipeline
	pipelineMu sync.Mutex

	// Bind group layout cache.
	layouts  map[string]*wgpu.BindGroupLayout
	layoutMu sync.Mutex
}

// NewDevice creates a GPU device using the best available backend.
// On Windows this prefers Vulkan > DX12 > Software.
func NewDevice() (*Device, error) {
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("compute: CreateInstance: %w", err)
	}

	adapter, err := instance.RequestAdapter(nil)
	if err != nil {
		instance.Release()
		return nil, fmt.Errorf("compute: no GPU adapter: %w", err)
	}

	info := adapter.Info()

	device, err := adapter.RequestDevice(nil)
	if err != nil {
		adapter.Release()
		instance.Release()
		return nil, fmt.Errorf("compute: RequestDevice: %w", err)
	}

	queue := device.Queue()

	d := &Device{
		instance:    instance,
		adapter:     adapter,
		device:      device,
		queue:       queue,
		AdapterName: info.Name,
		Backend:     info.Backend.String(),
		DeviceType:  info.DeviceType.String(),
		pipelines:   make(map[string]*wgpu.ComputePipeline),
		layouts:     make(map[string]*wgpu.BindGroupLayout),
	}

	return d, nil
}

// Release frees all GPU resources.
func (d *Device) Release() {
	d.pipelineMu.Lock()
	for _, p := range d.pipelines {
		p.Release()
	}
	d.pipelines = nil
	d.pipelineMu.Unlock()

	d.layoutMu.Lock()
	for _, l := range d.layouts {
		l.Release()
	}
	d.layouts = nil
	d.layoutMu.Unlock()

	if d.queue != nil {
		d.queue = nil
	}
	if d.device != nil {
		d.device.Release()
	}
	if d.adapter != nil {
		d.adapter.Release()
	}
	if d.instance != nil {
		d.instance.Release()
	}
}

// String returns a human-readable device description.
func (d *Device) String() string {
	return fmt.Sprintf("%s (%s, %s)", d.AdapterName, d.Backend, d.DeviceType)
}

// createBuffer creates a GPU buffer with the given usage flags.
func (d *Device) createBuffer(label string, size uint64, usage gputypes.BufferUsage) (*wgpu.Buffer, error) {
	return d.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: label,
		Size:  size,
		Usage: usage,
	})
}

// writeBuffer uploads data to a GPU buffer via the queue.
func (d *Device) writeBuffer(buf *wgpu.Buffer, offset uint64, data []byte) error {
	return d.queue.WriteBuffer(buf, offset, data)
}

// getOrCreatePipeline returns a cached pipeline or compiles a new one.
func (d *Device) getOrCreatePipeline(key string, shaderSrc string, entryPoint string, layout *wgpu.PipelineLayout) (*wgpu.ComputePipeline, error) {
	d.pipelineMu.Lock()
	defer d.pipelineMu.Unlock()

	if p, ok := d.pipelines[key]; ok {
		return p, nil
	}

	module, err := d.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: key,
		WGSL:  shaderSrc,
	})
	if err != nil {
		return nil, fmt.Errorf("compute: compile shader %q: %w", key, err)
	}
	defer module.Release()

	pipeline, err := d.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      key,
		Layout:     layout,
		Module:     module,
		EntryPoint: entryPoint,
	})
	if err != nil {
		return nil, fmt.Errorf("compute: create pipeline %q: %w", key, err)
	}

	d.pipelines[key] = pipeline
	return pipeline, nil
}
