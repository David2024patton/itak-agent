package compute

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gogpu/wgpu"
)

// Tensor represents a multi-dimensional array stored on the GPU.
type Tensor struct {
	dev    *Device
	buf    *wgpu.Buffer
	shape  []int
	size   int // total number of float32 elements
	nbytes uint64
}

// NewTensor creates a GPU tensor with the given shape and uploads data.
// Data must have exactly product(shape) float32 elements.
func NewTensor(dev *Device, shape []int, data []float32) (*Tensor, error) {
	size := 1
	for _, dim := range shape {
		if dim <= 0 {
			return nil, fmt.Errorf("compute: invalid dimension %d in shape %v", dim, shape)
		}
		size *= dim
	}

	if len(data) != size {
		return nil, fmt.Errorf("compute: data length %d doesn't match shape %v (need %d)", len(data), shape, size)
	}

	nbytes := uint64(size * 4)
	buf, err := dev.createBuffer("tensor", nbytes,
		wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err != nil {
		return nil, fmt.Errorf("compute: create tensor buffer: %w", err)
	}

	if err := dev.writeBuffer(buf, 0, float32ToBytes(data)); err != nil {
		buf.Release()
		return nil, fmt.Errorf("compute: upload tensor data: %w", err)
	}

	shapeCopy := make([]int, len(shape))
	copy(shapeCopy, shape)

	return &Tensor{
		dev:    dev,
		buf:    buf,
		shape:  shapeCopy,
		size:   size,
		nbytes: nbytes,
	}, nil
}

// NewEmptyTensor creates a GPU tensor with the given shape but no data.
// The buffer contents are undefined until written to by a compute operation.
func NewEmptyTensor(dev *Device, shape []int) (*Tensor, error) {
	size := 1
	for _, dim := range shape {
		if dim <= 0 {
			return nil, fmt.Errorf("compute: invalid dimension %d in shape %v", dim, shape)
		}
		size *= dim
	}

	nbytes := uint64(size * 4)
	buf, err := dev.createBuffer("tensor", nbytes,
		wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err != nil {
		return nil, fmt.Errorf("compute: create empty tensor buffer: %w", err)
	}

	shapeCopy := make([]int, len(shape))
	copy(shapeCopy, shape)

	return &Tensor{
		dev:    dev,
		buf:    buf,
		shape:  shapeCopy,
		size:   size,
		nbytes: nbytes,
	}, nil
}

// NewUint32Tensor creates a GPU buffer containing uint32 data (e.g. token IDs).
// This is stored as a Tensor with nbytes = len(data)*4 but size tracks element count.
// The buffer holds raw u32 values, not f32; only use with shaders expecting u32.
func NewUint32Tensor(dev *Device, shape []int, data []uint32) (*Tensor, error) {
	size := 1
	for _, dim := range shape {
		if dim <= 0 {
			return nil, fmt.Errorf("compute: invalid dimension %d in shape %v", dim, shape)
		}
		size *= dim
	}

	if len(data) != size {
		return nil, fmt.Errorf("compute: data length %d doesn't match shape %v (need %d)", len(data), shape, size)
	}

	nbytes := uint64(size * 4)
	buf, err := dev.createBuffer("tensor_u32", nbytes,
		wgpu.BufferUsageStorage|wgpu.BufferUsageCopySrc|wgpu.BufferUsageCopyDst)
	if err != nil {
		return nil, fmt.Errorf("compute: create u32 tensor buffer: %w", err)
	}

	if err := dev.writeBuffer(buf, 0, uint32SliceToBytes(data)); err != nil {
		buf.Release()
		return nil, fmt.Errorf("compute: upload u32 tensor data: %w", err)
	}

	shapeCopy := make([]int, len(shape))
	copy(shapeCopy, shape)

	return &Tensor{
		dev:    dev,
		buf:    buf,
		shape:  shapeCopy,
		size:   size,
		nbytes: nbytes,
	}, nil
}

// Shape returns the tensor's dimensions.
func (t *Tensor) Shape() []int {
	out := make([]int, len(t.shape))
	copy(out, t.shape)
	return out
}

// Size returns the total number of float32 elements.
func (t *Tensor) Size() int {
	return t.size
}

// ToCPU reads the tensor data back from the GPU.
// This involves a staging buffer copy and is synchronous.
func (t *Tensor) ToCPU() ([]float32, error) {
	// Create a staging buffer with MapRead usage for Vulkan-compatible readback.
	staging, err := t.dev.createBuffer("staging", t.nbytes,
		wgpu.BufferUsageMapRead|wgpu.BufferUsageCopyDst)
	if err != nil {
		return nil, fmt.Errorf("compute: create staging buffer: %w", err)
	}
	defer staging.Release()

	// Encode the copy command.
	encoder, err := t.dev.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, fmt.Errorf("compute: create command encoder: %w", err)
	}

	encoder.CopyBufferToBuffer(t.buf, 0, staging, 0, t.nbytes)

	cmdBuf, err := encoder.Finish()
	if err != nil {
		return nil, fmt.Errorf("compute: finish command encoder: %w", err)
	}

	if err := t.dev.queue.Submit(cmdBuf); err != nil {
		return nil, fmt.Errorf("compute: submit copy: %w", err)
	}

	// Read from the staging buffer (forces GPU sync).
	data := make([]byte, t.nbytes)
	if err := t.dev.queue.ReadBuffer(staging, 0, data); err != nil {
		return nil, fmt.Errorf("compute: read staging buffer: %w", err)
	}

	return bytesToFloat32(data), nil
}

// Reshape returns a new view of the tensor with a different shape.
// The total number of elements must remain the same.
// Note: this does NOT copy data, it shares the same GPU buffer.
func (t *Tensor) Reshape(newShape []int) (*Tensor, error) {
	newSize := 1
	for _, dim := range newShape {
		if dim <= 0 {
			return nil, fmt.Errorf("compute: invalid dimension %d in shape %v", dim, newShape)
		}
		newSize *= dim
	}

	if newSize != t.size {
		return nil, fmt.Errorf("compute: reshape size mismatch: %d vs %d", newSize, t.size)
	}

	shapeCopy := make([]int, len(newShape))
	copy(shapeCopy, newShape)

	return &Tensor{
		dev:    t.dev,
		buf:    t.buf, // shared buffer, no copy
		shape:  shapeCopy,
		size:   t.size,
		nbytes: t.nbytes,
	}, nil
}

// Release frees the GPU buffer. After this call the tensor must not be used.
func (t *Tensor) Release() {
	if t.buf != nil {
		t.buf.Release()
		t.buf = nil
	}
}

// float32ToBytes converts a float32 slice to a byte slice.
func float32ToBytes(data []float32) []byte {
	buf := make([]byte, len(data)*4)
	for i, v := range data {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// bytesToFloat32 converts a byte slice to a float32 slice.
func bytesToFloat32(data []byte) []float32 {
	result := make([]float32, len(data)/4)
	for i := range result {
		result[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return result
}

// uint32ToBytes converts a uint32 to a 4-byte slice.
func uint32ToBytes(v uint32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, v)
	return buf
}

// float32Bytes converts a float32 to a 4-byte slice.
func float32Bytes(v float32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(v))
	return buf
}

// uint32SliceToBytes converts a uint32 slice to a byte slice.
func uint32SliceToBytes(data []uint32) []byte {
	buf := make([]byte, len(data)*4)
	for i, v := range data {
		binary.LittleEndian.PutUint32(buf[i*4:], v)
	}
	return buf
}
