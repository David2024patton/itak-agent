package softgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

// softCommandEncoder implements hal.CommandEncoder.
// It records compute dispatch commands and executes them on Finish.
type softCommandEncoder struct {
	device   *softDevice
	commands []recordedCommand
	finished bool
}

type recordedCommand struct {
	kind       commandKind
	pipeline   *softComputePipeline
	bindGroups map[uint32]*softBindGroup
	dispatchX  uint32
	dispatchY  uint32
	dispatchZ  uint32
}

type commandKind int

const (
	cmdDispatch commandKind = iota
)

func (e *softCommandEncoder) BeginEncoding(_ string) error              { return nil }
func (e *softCommandEncoder) DiscardEncoding()                          { e.finished = true }
func (e *softCommandEncoder) ResetAll(_ []hal.CommandBuffer)            {}
func (e *softCommandEncoder) TransitionBuffers(_ []hal.BufferBarrier)   {}
func (e *softCommandEncoder) TransitionTextures(_ []hal.TextureBarrier) {}
func (e *softCommandEncoder) ClearBuffer(_ hal.Buffer, _, _ uint64)     {}

func (e *softCommandEncoder) CopyBufferToBuffer(src, dst hal.Buffer, regions []hal.BufferCopy) {
	srcBuf, okS := src.(*softBuffer)
	dstBuf, okD := dst.(*softBuffer)
	if !okS || !okD {
		return
	}
	for _, r := range regions {
		copy(dstBuf.data[r.DstOffset:r.DstOffset+r.Size], srcBuf.data[r.SrcOffset:r.SrcOffset+r.Size])
	}
}

func (e *softCommandEncoder) CopyBufferToTexture(_ hal.Buffer, _ hal.Texture, _ []hal.BufferTextureCopy) {
}
func (e *softCommandEncoder) CopyTextureToBuffer(_ hal.Texture, _ hal.Buffer, _ []hal.BufferTextureCopy) {
}
func (e *softCommandEncoder) CopyTextureToTexture(_, _ hal.Texture, _ []hal.TextureCopy)          {}
func (e *softCommandEncoder) ResolveQuerySet(_ hal.QuerySet, _, _ uint32, _ hal.Buffer, _ uint64) {}

func (e *softCommandEncoder) BeginRenderPass(_ *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	return &stubRenderPass{}
}

func (e *softCommandEncoder) BeginComputePass(_ *hal.ComputePassDescriptor) hal.ComputePassEncoder {
	return &softComputePass{encoder: e}
}

func (e *softCommandEncoder) EndEncoding() (hal.CommandBuffer, error) {
	if e.finished {
		return nil, fmt.Errorf("softgpu: encoder already finished")
	}
	e.finished = true
	return &softCommandBuffer{commands: e.commands, device: e.device}, nil
}

// softComputePass implements hal.ComputePassEncoder.
type softComputePass struct {
	encoder    *softCommandEncoder
	pipeline   *softComputePipeline
	bindGroups map[uint32]*softBindGroup
}

func (p *softComputePass) End() {}

func (p *softComputePass) SetPipeline(pipeline hal.ComputePipeline) {
	if cp, ok := pipeline.(*softComputePipeline); ok {
		p.pipeline = cp
	}
}

func (p *softComputePass) SetBindGroup(index uint32, group hal.BindGroup, _ []uint32) {
	if bg, ok := group.(*softBindGroup); ok {
		if p.bindGroups == nil {
			p.bindGroups = make(map[uint32]*softBindGroup)
		}
		p.bindGroups[index] = bg
	}
}

func (p *softComputePass) Dispatch(x, y, z uint32) {
	p.encoder.commands = append(p.encoder.commands, recordedCommand{
		kind:       cmdDispatch,
		pipeline:   p.pipeline,
		bindGroups: p.cloneBindGroups(),
		dispatchX:  x,
		dispatchY:  y,
		dispatchZ:  z,
	})
}

func (p *softComputePass) DispatchIndirect(_ hal.Buffer, _ uint64) {}

func (p *softComputePass) cloneBindGroups() map[uint32]*softBindGroup {
	result := make(map[uint32]*softBindGroup, len(p.bindGroups))
	for k, v := range p.bindGroups {
		result[k] = v
	}
	return result
}

// softCommandBuffer holds recorded commands for execution.
type softCommandBuffer struct {
	device   *softDevice
	commands []recordedCommand
}

func (cb *softCommandBuffer) Destroy() {}

// stubRenderPass is a no-op render pass for the compute-only backend.
type stubRenderPass struct{}

func (s *stubRenderPass) End()                                                          {}
func (s *stubRenderPass) SetPipeline(_ hal.RenderPipeline)                              {}
func (s *stubRenderPass) SetBindGroup(_ uint32, _ hal.BindGroup, _ []uint32)            {}
func (s *stubRenderPass) SetVertexBuffer(_ uint32, _ hal.Buffer, _ uint64)              {}
func (s *stubRenderPass) SetIndexBuffer(_ hal.Buffer, _ gputypes.IndexFormat, _ uint64) {}
func (s *stubRenderPass) SetViewport(_, _, _, _, _, _ float32)                          {}
func (s *stubRenderPass) SetScissorRect(_, _, _, _ uint32)                              {}
func (s *stubRenderPass) SetBlendConstant(_ *gputypes.Color)                            {}
func (s *stubRenderPass) SetStencilReference(_ uint32)                                  {}
func (s *stubRenderPass) Draw(_, _, _, _ uint32)                                        {}
func (s *stubRenderPass) DrawIndexed(_, _, _ uint32, _ int32, _ uint32)                 {}
func (s *stubRenderPass) DrawIndirect(_ hal.Buffer, _ uint64)                           {}
func (s *stubRenderPass) DrawIndexedIndirect(_ hal.Buffer, _ uint64)                    {}
func (s *stubRenderPass) ExecuteBundle(_ hal.RenderBundle)                              {}
