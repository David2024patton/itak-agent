package model

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
)

// GGUF magic number: "GGUF" in ASCII.
const ggufMagic = 0x46554747 // "GGUF" little-endian

// GGML type enum values (from ggml.h).
const (
	ggmlTypeF32  = 0
	ggmlTypeF16  = 1
	ggmlTypeQ4_0 = 2
	ggmlTypeQ4_1 = 3
	ggmlTypeQ8_0 = 8
	ggmlTypeQ8_1 = 9
)

// GGUF metadata value type enum.
const (
	ggufTypeUint8   = 0
	ggufTypeInt8    = 1
	ggufTypeUint16  = 2
	ggufTypeInt16   = 3
	ggufTypeUint32  = 4
	ggufTypeInt32   = 5
	ggufTypeFloat32 = 6
	ggufTypeBool    = 7
	ggufTypeString  = 8
	ggufTypeArray   = 9
	ggufTypeUint64  = 10
	ggufTypeInt64   = 11
	ggufTypeFloat64 = 12
)

// GGUFFile represents a parsed GGUF model file.
type GGUFFile struct {
	path     string
	file     *os.File
	version  uint32
	metadata map[string]interface{}
	tensors  map[string]*ggufTensorEntry

	// Byte offset where tensor data begins (after header + metadata + tensor infos + alignment padding).
	dataOffset int64
	alignment  int64 // data alignment (default 32)
}

// ggufTensorEntry holds parsed info for one tensor from the tensor info section.
type ggufTensorEntry struct {
	Name     string
	NDims    uint32
	Dims     []uint64
	GGMLType uint32
	Offset   uint64 // relative to data section start
	NumElems int
	ByteSize uint64
}

// OpenGGUF opens and parses a GGUF file (version 2 or 3).
//
// GGUF binary layout:
//   - [4 bytes] magic "GGUF"
//   - [4 bytes] version (uint32 LE)
//   - [8 bytes] n_tensors (uint64 LE)
//   - [8 bytes] n_kv (uint64 LE)
//   - [variable] metadata key-value pairs
//   - [variable] tensor info array
//   - [padding to alignment]
//   - [variable] tensor data
func OpenGGUF(path string) (*GGUFFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("model: open gguf %q: %w", path, err)
	}

	r := &ggufReader{f: f}

	// Read and verify magic.
	magic := r.u32()
	if r.err != nil {
		f.Close()
		return nil, fmt.Errorf("model: read magic: %w", r.err)
	}
	if magic != ggufMagic {
		f.Close()
		return nil, fmt.Errorf("model: not a GGUF file (magic=0x%08X)", magic)
	}

	// Read version.
	version := r.u32()
	if version < 2 || version > 3 {
		f.Close()
		return nil, fmt.Errorf("model: unsupported GGUF version %d", version)
	}

	// Read counts.
	nTensors := r.u64()
	nKV := r.u64()
	if r.err != nil {
		f.Close()
		return nil, fmt.Errorf("model: read header: %w", r.err)
	}

	gf := &GGUFFile{
		path:      path,
		file:      f,
		version:   version,
		metadata:  make(map[string]interface{}),
		tensors:   make(map[string]*ggufTensorEntry),
		alignment: 32, // default
	}

	// Parse metadata key-value pairs.
	for i := uint64(0); i < nKV; i++ {
		key := r.str()
		valType := r.u32()
		val := r.readValue(valType)
		if r.err != nil {
			f.Close()
			return nil, fmt.Errorf("model: parse metadata kv %d (%q): %w", i, key, r.err)
		}
		gf.metadata[key] = val

		// Check for alignment override.
		if key == "general.alignment" {
			if a, ok := val.(uint32); ok && a > 0 {
				gf.alignment = int64(a)
			}
		}
	}

	// Parse tensor info array.
	for i := uint64(0); i < nTensors; i++ {
		name := r.str()
		nDims := r.u32()
		dims := make([]uint64, nDims)
		for d := uint32(0); d < nDims; d++ {
			dims[d] = r.u64()
		}
		ggmlType := r.u32()
		offset := r.u64()
		if r.err != nil {
			f.Close()
			return nil, fmt.Errorf("model: parse tensor info %d (%q): %w", i, name, r.err)
		}

		numElems := 1
		for _, d := range dims {
			numElems *= int(d)
		}

		entry := &ggufTensorEntry{
			Name:     name,
			NDims:    nDims,
			Dims:     dims,
			GGMLType: ggmlType,
			Offset:   offset,
			NumElems: numElems,
			ByteSize: ggmlTypeByteSize(ggmlType, numElems),
		}
		gf.tensors[name] = entry
	}

	// Data section starts after the tensor info, aligned to gf.alignment.
	curPos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("model: seek position: %w", err)
	}
	gf.dataOffset = alignOffset(curPos, gf.alignment)

	return gf, nil
}

// Metadata returns model-level metadata as string key-value pairs.
// Non-string values are formatted with %v.
func (gf *GGUFFile) Metadata() map[string]string {
	out := make(map[string]string, len(gf.metadata))
	for k, v := range gf.metadata {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}

// MetadataRaw returns the raw typed metadata map.
func (gf *GGUFFile) MetadataRaw() map[string]interface{} {
	out := make(map[string]interface{}, len(gf.metadata))
	for k, v := range gf.metadata {
		out[k] = v
	}
	return out
}

// TensorInfos returns descriptors for all tensors.
func (gf *GGUFFile) TensorInfos() []TensorInfo {
	infos := make([]TensorInfo, 0, len(gf.tensors))
	for _, e := range gf.tensors {
		shape := make([]int, len(e.Dims))
		for i, d := range e.Dims {
			shape[i] = int(d)
		}

		infos = append(infos, TensorInfo{
			Name:   e.Name,
			Shape:  shape,
			Dtype:  ggmlTypeToDType(e.GGMLType),
			Size:   e.NumElems,
			Offset: e.Offset,
			Nbytes: e.ByteSize,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// TensorNames returns sorted tensor names.
func (gf *GGUFFile) TensorNames() []string {
	names := make([]string, 0, len(gf.tensors))
	for name := range gf.tensors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReadTensorData reads raw tensor bytes and dequantizes to float32.
func (gf *GGUFFile) ReadTensorData(name string) ([]float32, error) {
	entry, ok := gf.tensors[name]
	if !ok {
		return nil, fmt.Errorf("model: tensor %q not found", name)
	}

	data := make([]byte, entry.ByteSize)
	absOffset := gf.dataOffset + int64(entry.Offset)
	if _, err := gf.file.ReadAt(data, absOffset); err != nil {
		return nil, fmt.Errorf("model: read tensor %q data: %w", name, err)
	}

	switch entry.GGMLType {
	case ggmlTypeF32:
		return F32SliceFromBytes(data), nil
	case ggmlTypeF16:
		return F16SliceToF32(data), nil
	case ggmlTypeQ4_0:
		return dequantQ4_0(data, entry.NumElems)
	case ggmlTypeQ8_0:
		return dequantQ8_0(data, entry.NumElems)
	default:
		return nil, fmt.Errorf("model: unsupported ggml type %d for tensor %q", entry.GGMLType, name)
	}
}

// Close releases the file handle.
func (gf *GGUFFile) Close() error {
	if gf.file != nil {
		return gf.file.Close()
	}
	return nil
}

// Verify GGUFFile implements ModelFile at compile time.
var _ ModelFile = (*GGUFFile)(nil)

// ---------------------------------------------------------------------------
// Dequantization
// ---------------------------------------------------------------------------

// dequantQ8_0 dequantizes Q8_0 data to float32.
// Q8_0 block: 1 f16 scale + 32 int8 quantized values = 34 bytes per block.
func dequantQ8_0(data []byte, numElems int) ([]float32, error) {
	const blockSize = 32
	const blockBytes = 2 + blockSize // 2 bytes f16 scale + 32 bytes int8 data
	nBlocks := (numElems + blockSize - 1) / blockSize

	if len(data) < nBlocks*blockBytes {
		return nil, fmt.Errorf("model: Q8_0 data too short: have %d bytes, need %d", len(data), nBlocks*blockBytes)
	}

	out := make([]float32, numElems)
	for b := 0; b < nBlocks; b++ {
		blockData := data[b*blockBytes:]
		scale := F16ToF32(binary.LittleEndian.Uint16(blockData[0:2]))

		for i := 0; i < blockSize; i++ {
			idx := b*blockSize + i
			if idx >= numElems {
				break
			}
			qi := int8(blockData[2+i])
			out[idx] = scale * float32(qi)
		}
	}

	return out, nil
}

// dequantQ4_0 dequantizes Q4_0 data to float32.
// Q4_0 block: 1 f16 scale + 16 bytes (32 nibbles, each a 4-bit signed int) = 18 bytes per block.
// Nibble encoding: value = nibble - 8 (unsigned nibble 0-15 maps to signed -8..+7).
func dequantQ4_0(data []byte, numElems int) ([]float32, error) {
	const blockSize = 32
	const blockBytes = 2 + blockSize/2 // 2 bytes f16 scale + 16 bytes nibble data
	nBlocks := (numElems + blockSize - 1) / blockSize

	if len(data) < nBlocks*blockBytes {
		return nil, fmt.Errorf("model: Q4_0 data too short: have %d bytes, need %d", len(data), nBlocks*blockBytes)
	}

	out := make([]float32, numElems)
	for b := 0; b < nBlocks; b++ {
		blockData := data[b*blockBytes:]
		scale := F16ToF32(binary.LittleEndian.Uint16(blockData[0:2]))

		for i := 0; i < blockSize/2; i++ {
			nibbleByte := blockData[2+i]

			// Low nibble -> element 2*i
			lo := int(nibbleByte&0x0F) - 8
			idx := b*blockSize + i
			if idx < numElems {
				out[idx] = scale * float32(lo)
			}

			// High nibble -> element 2*i + blockSize/2
			hi := int(nibbleByte>>4) - 8
			idx2 := b*blockSize + i + blockSize/2
			if idx2 < numElems {
				out[idx2] = scale * float32(hi)
			}
		}
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// GGUF binary reader helpers
// ---------------------------------------------------------------------------

type ggufReader struct {
	f   *os.File
	err error
}

func (r *ggufReader) u8() uint8 {
	if r.err != nil {
		return 0
	}
	var v uint8
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) i8() int8 {
	if r.err != nil {
		return 0
	}
	var v int8
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) u16() uint16 {
	if r.err != nil {
		return 0
	}
	var v uint16
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) i16() int16 {
	if r.err != nil {
		return 0
	}
	var v int16
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) u32() uint32 {
	if r.err != nil {
		return 0
	}
	var v uint32
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) i32() int32 {
	if r.err != nil {
		return 0
	}
	var v int32
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) u64() uint64 {
	if r.err != nil {
		return 0
	}
	var v uint64
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) i64() int64 {
	if r.err != nil {
		return 0
	}
	var v int64
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) f32() float32 {
	if r.err != nil {
		return 0
	}
	var v float32
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) f64() float64 {
	if r.err != nil {
		return 0
	}
	var v float64
	r.err = binary.Read(r.f, binary.LittleEndian, &v)
	return v
}

func (r *ggufReader) str() string {
	if r.err != nil {
		return ""
	}
	length := r.u64()
	if r.err != nil {
		return ""
	}
	if length > 10*1024*1024 {
		r.err = fmt.Errorf("string too long: %d bytes", length)
		return ""
	}
	buf := make([]byte, length)
	_, r.err = io.ReadFull(r.f, buf)
	return string(buf)
}

func (r *ggufReader) readValue(valType uint32) interface{} {
	if r.err != nil {
		return nil
	}
	switch valType {
	case ggufTypeUint8:
		return r.u8()
	case ggufTypeInt8:
		return r.i8()
	case ggufTypeUint16:
		return r.u16()
	case ggufTypeInt16:
		return r.i16()
	case ggufTypeUint32:
		return r.u32()
	case ggufTypeInt32:
		return r.i32()
	case ggufTypeFloat32:
		return r.f32()
	case ggufTypeBool:
		v := r.u8()
		return v != 0
	case ggufTypeString:
		return r.str()
	case ggufTypeArray:
		elemType := r.u32()
		count := r.u64()
		if r.err != nil {
			return nil
		}
		arr := make([]interface{}, count)
		for i := uint64(0); i < count; i++ {
			arr[i] = r.readValue(elemType)
			if r.err != nil {
				return nil
			}
		}
		return arr
	case ggufTypeUint64:
		return r.u64()
	case ggufTypeInt64:
		return r.i64()
	case ggufTypeFloat64:
		return r.f64()
	default:
		r.err = fmt.Errorf("unknown gguf type %d", valType)
		return nil
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ggmlTypeByteSize returns the byte count needed for numElems of the given ggml type.
func ggmlTypeByteSize(ggmlType uint32, numElems int) uint64 {
	switch ggmlType {
	case ggmlTypeF32:
		return uint64(numElems * 4)
	case ggmlTypeF16:
		return uint64(numElems * 2)
	case ggmlTypeQ8_0:
		nBlocks := (numElems + 31) / 32
		return uint64(nBlocks * 34) // 2 (scale) + 32 (data) per block
	case ggmlTypeQ4_0:
		nBlocks := (numElems + 31) / 32
		return uint64(nBlocks * 18) // 2 (scale) + 16 (nibbles) per block
	case ggmlTypeQ4_1:
		nBlocks := (numElems + 31) / 32
		return uint64(nBlocks * 20) // 2 (scale) + 2 (min) + 16 (nibbles) per block
	default:
		// Fallback: assume 4 bytes per element.
		return uint64(numElems * 4)
	}
}

// ggmlTypeToDType maps ggml type enum to our DType.
func ggmlTypeToDType(ggmlType uint32) DType {
	switch ggmlType {
	case ggmlTypeF32:
		return DTypeF32
	case ggmlTypeF16:
		return DTypeF16
	case ggmlTypeQ4_0:
		return DTypeQ4_0
	case ggmlTypeQ4_1:
		return DTypeQ4_1
	case ggmlTypeQ8_0:
		return DTypeQ8_0
	default:
		return DTypeF32
	}
}

// alignOffset rounds up offset to the next multiple of alignment.
func alignOffset(offset, alignment int64) int64 {
	if alignment <= 1 {
		return offset
	}
	rem := offset % alignment
	if rem == 0 {
		return offset
	}
	return offset + (alignment - rem)
}

// f16Bits returns the uint16 representation of a float value in f16 format.
// Used for writing test data.
func f16Bits(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := (bits >> 31) & 1
	exp := (bits >> 23) & 0xFF
	frac := bits & 0x7FFFFF

	switch {
	case exp == 0xFF:
		return uint16((sign << 15) | (0x1F << 10) | (frac >> 13))
	case exp > 127+15:
		return uint16((sign << 15) | (0x1F << 10))
	case exp < 127-14:
		return uint16(sign << 15)
	default:
		exp16 := exp - 127 + 15
		return uint16((sign << 15) | (exp16 << 10) | (frac >> 13))
	}
}
