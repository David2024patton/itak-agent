// Package model provides parsers for LLM model file formats (SafeTensors, GGUF).
//
// It defines a common ModelFile interface that abstracts over different formats,
// allowing the inference engine to load weights without caring about the source format.
//
// Usage:
//
//	model, err := model.OpenSafeTensors("model.safetensors")
//	tensor, err := model.LoadTensor("model.layers.0.attention.wq.weight", dev)
package model

import (
	"encoding/binary"
	"fmt"
	"math"
)

// DType represents a tensor data type.
type DType int

const (
	DTypeF32  DType = iota // 32-bit float
	DTypeF16               // IEEE 754 half-precision float
	DTypeBF16              // Brain floating point (bfloat16)
	DTypeQ4_0              // 4-bit quantized (GGML)
	DTypeQ4_1              // 4-bit quantized with min (GGML)
	DTypeQ8_0              // 8-bit quantized (GGML)
	DTypeI32               // 32-bit integer
	DTypeI16               // 16-bit integer
	DTypeI8                // 8-bit integer
	DTypeU8                // unsigned 8-bit integer
)

// String returns the human-readable name of the dtype.
func (d DType) String() string {
	switch d {
	case DTypeF32:
		return "F32"
	case DTypeF16:
		return "F16"
	case DTypeBF16:
		return "BF16"
	case DTypeQ4_0:
		return "Q4_0"
	case DTypeQ4_1:
		return "Q4_1"
	case DTypeQ8_0:
		return "Q8_0"
	case DTypeI32:
		return "I32"
	case DTypeI16:
		return "I16"
	case DTypeI8:
		return "I8"
	case DTypeU8:
		return "U8"
	default:
		return fmt.Sprintf("DType(%d)", d)
	}
}

// TensorInfo describes a tensor stored in a model file.
type TensorInfo struct {
	Name   string
	Shape  []int
	Dtype  DType
	Size   int    // total number of elements
	Offset uint64 // byte offset into data section
	Nbytes uint64 // raw byte count in the file
}

// ModelFile is the common interface for model file formats.
type ModelFile interface {
	// Metadata returns model-level key-value metadata.
	Metadata() map[string]string

	// TensorInfos returns descriptors for all tensors in the file.
	TensorInfos() []TensorInfo

	// TensorNames returns the names of all tensors in the file.
	TensorNames() []string

	// ReadTensorData reads raw tensor bytes and converts to float32.
	// This performs any necessary dequantization (Q4_0, Q8_0, etc.)
	// or type conversion (f16 -> f32, bf16 -> f32).
	ReadTensorData(name string) ([]float32, error)

	// Close releases any resources (file handles, mmaps).
	Close() error
}

// ---------------------------------------------------------------------------
// Float16 / BFloat16 conversion utilities
// ---------------------------------------------------------------------------

// F16ToF32 converts an IEEE 754 half-precision float (uint16) to float32.
func F16ToF32(h uint16) float32 {
	sign := uint32(h>>15) & 1
	exp := uint32(h>>10) & 0x1F
	frac := uint32(h) & 0x3FF

	switch {
	case exp == 0:
		if frac == 0 {
			// Zero
			return math.Float32frombits(sign << 31)
		}
		// Subnormal: convert to normalized f32
		exp = 127 - 15 + 1
		for frac&0x400 == 0 {
			frac <<= 1
			exp--
		}
		frac &= 0x3FF
		return math.Float32frombits((sign << 31) | (exp << 23) | (frac << 13))
	case exp == 0x1F:
		// Inf / NaN
		return math.Float32frombits((sign << 31) | (0xFF << 23) | (frac << 13))
	default:
		// Normal: re-bias exponent from f16 bias (15) to f32 bias (127)
		exp32 := exp - 15 + 127
		return math.Float32frombits((sign << 31) | (exp32 << 23) | (frac << 13))
	}
}

// BF16ToF32 converts a bfloat16 (uint16) to float32.
// BF16 is simply the upper 16 bits of a float32, so we left-shift by 16.
func BF16ToF32(b uint16) float32 {
	return math.Float32frombits(uint32(b) << 16)
}

// F16SliceToF32 converts a byte slice of packed f16 values to float32.
func F16SliceToF32(data []byte) []float32 {
	n := len(data) / 2
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		h := binary.LittleEndian.Uint16(data[i*2:])
		out[i] = F16ToF32(h)
	}
	return out
}

// BF16SliceToF32 converts a byte slice of packed bf16 values to float32.
func BF16SliceToF32(data []byte) []float32 {
	n := len(data) / 2
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		b := binary.LittleEndian.Uint16(data[i*2:])
		out[i] = BF16ToF32(b)
	}
	return out
}

// F32SliceFromBytes reinterprets a byte slice as float32 (little-endian).
func F32SliceFromBytes(data []byte) []float32 {
	n := len(data) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return out
}
