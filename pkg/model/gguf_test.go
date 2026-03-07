package model

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// ggufTestWriter builds a minimal GGUF file for testing.
type ggufTestWriter struct {
	buf []byte
}

func (w *ggufTestWriter) writeU32(v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	w.buf = append(w.buf, b...)
}

func (w *ggufTestWriter) writeU64(v uint64) {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	w.buf = append(w.buf, b...)
}

func (w *ggufTestWriter) writeF32(v float32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
	w.buf = append(w.buf, b...)
}

func (w *ggufTestWriter) writeStr(s string) {
	w.writeU64(uint64(len(s)))
	w.buf = append(w.buf, []byte(s)...)
}

func (w *ggufTestWriter) writeKVString(key, val string) {
	w.writeStr(key)
	w.writeU32(ggufTypeString)
	w.writeStr(val)
}

func (w *ggufTestWriter) writeKVUint32(key string, val uint32) {
	w.writeStr(key)
	w.writeU32(ggufTypeUint32)
	w.writeU32(val)
}

func (w *ggufTestWriter) writeTensorInfo(name string, dims []uint64, ggmlType uint32, offset uint64) {
	w.writeStr(name)
	w.writeU32(uint32(len(dims)))
	for _, d := range dims {
		w.writeU64(d)
	}
	w.writeU32(ggmlType)
	w.writeU64(offset)
}

// writeGGUFTestFile creates a minimal GGUF file with specified tensors and returns the path.
func writeGGUFTestFile(t *testing.T, tensors []ggufTestTensor, kvs map[string]interface{}) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.gguf")

	w := &ggufTestWriter{}

	// Header.
	w.writeU32(ggufMagic) // magic
	w.writeU32(3)         // version 3

	w.writeU64(uint64(len(tensors))) // n_tensors
	w.writeU64(uint64(len(kvs)))     // n_kv

	// Metadata KV pairs.
	for key, val := range kvs {
		switch v := val.(type) {
		case string:
			w.writeKVString(key, v)
		case uint32:
			w.writeKVUint32(key, v)
		}
	}

	// Calculate tensor data offsets.
	// Tensor info section needs to be written before we know the data section offset,
	// so we use relative offsets from the data section start.
	dataOffsets := make([]uint64, len(tensors))
	offset := uint64(0)
	for i, tensor := range tensors {
		dataOffsets[i] = offset
		offset += uint64(len(tensor.data))
	}

	// Tensor info.
	for i, tensor := range tensors {
		w.writeTensorInfo(tensor.name, tensor.dims, tensor.ggmlType, dataOffsets[i])
	}

	// Align to 32 bytes (default alignment).
	headerSize := len(w.buf)
	alignment := 32
	padding := (alignment - (headerSize % alignment)) % alignment
	for range padding {
		w.buf = append(w.buf, 0)
	}

	// Tensor data.
	for _, tensor := range tensors {
		w.buf = append(w.buf, tensor.data...)
	}

	if err := os.WriteFile(path, w.buf, 0644); err != nil {
		t.Fatalf("write test gguf: %v", err)
	}

	return path
}

type ggufTestTensor struct {
	name     string
	dims     []uint64
	ggmlType uint32
	data     []byte
}

// f32Bytes encodes float32 values to little-endian bytes.
func f32Bytes(values ...float32) []byte {
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

func TestGGUFParseHeader(t *testing.T) {
	data := f32Bytes(1.0, 2.0, 3.0, 4.0, 5.0, 6.0)

	path := writeGGUFTestFile(t, []ggufTestTensor{
		{
			name:     "test.weight",
			dims:     []uint64{2, 3},
			ggmlType: ggmlTypeF32,
			data:     data,
		},
	}, map[string]interface{}{
		"general.architecture": "test",
		"general.name":         "TestModel",
	})

	gf, err := OpenGGUF(path)
	if err != nil {
		t.Fatalf("OpenGGUF: %v", err)
	}
	defer gf.Close()

	// Verify version.
	if gf.version != 3 {
		t.Errorf("version: got %d, want 3", gf.version)
	}

	// Verify metadata.
	meta := gf.Metadata()
	if meta["general.architecture"] != "test" {
		t.Errorf("architecture: got %q", meta["general.architecture"])
	}
	if meta["general.name"] != "TestModel" {
		t.Errorf("name: got %q", meta["general.name"])
	}

	// Verify tensor listing.
	names := gf.TensorNames()
	if len(names) != 1 || names[0] != "test.weight" {
		t.Errorf("tensor names: got %v", names)
	}

	infos := gf.TensorInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 tensor info, got %d", len(infos))
	}
	info := infos[0]
	if info.Dtype != DTypeF32 {
		t.Errorf("dtype: got %s, want F32", info.Dtype)
	}
	if info.Size != 6 {
		t.Errorf("size: got %d, want 6", info.Size)
	}

	t.Logf("GGUF header: PASSED (version=%d, %d metadata keys, %d tensors)", gf.version, len(meta), len(names))
}

func TestGGUFF32RoundTrip(t *testing.T) {
	want := []float32{1.0, 2.5, -3.14, 0, 42.0, 0.001}
	data := f32Bytes(want...)

	path := writeGGUFTestFile(t, []ggufTestTensor{
		{
			name:     "layer.weight",
			dims:     []uint64{2, 3},
			ggmlType: ggmlTypeF32,
			data:     data,
		},
	}, map[string]interface{}{
		"general.architecture": "test",
	})

	gf, err := OpenGGUF(path)
	if err != nil {
		t.Fatalf("OpenGGUF: %v", err)
	}
	defer gf.Close()

	got, err := gf.ReadTensorData("layer.weight")
	if err != nil {
		t.Fatalf("ReadTensorData: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d", len(got), len(want))
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("[%d]: got %f, want %f", i, v, want[i])
		}
	}
	t.Logf("GGUF F32 round-trip: PASSED (%d elements)", len(got))
}

func TestGGUFQ8_0Dequant(t *testing.T) {
	// Create a Q8_0 block manually:
	// 32 elements, scale = 2.0, quantized values = [-4, -3, ..., +27]
	const blockSize = 32
	scale := float32(0.125) // small scale so values are representable

	// Build raw block: [f16 scale] [32 x int8]
	block := make([]byte, 34)
	binary.LittleEndian.PutUint16(block[0:2], f16Bits(scale))

	want := make([]float32, blockSize)
	for i := 0; i < blockSize; i++ {
		qi := int8(i - 16) // -16 to +15
		block[2+i] = byte(qi)
		want[i] = scale * float32(qi)
	}

	path := writeGGUFTestFile(t, []ggufTestTensor{
		{
			name:     "q8.weight",
			dims:     []uint64{32},
			ggmlType: ggmlTypeQ8_0,
			data:     block,
		},
	}, map[string]interface{}{
		"general.architecture": "test",
	})

	gf, err := OpenGGUF(path)
	if err != nil {
		t.Fatalf("OpenGGUF: %v", err)
	}
	defer gf.Close()

	got, err := gf.ReadTensorData("q8.weight")
	if err != nil {
		t.Fatalf("ReadTensorData: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d", len(got), len(want))
	}

	maxDiff := float32(0)
	for i, v := range got {
		diff := float32(math.Abs(float64(v - want[i])))
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	if maxDiff > 0.01 {
		t.Errorf("Q8_0 max diff too large: %f", maxDiff)
		for i := range got {
			if math.Abs(float64(got[i]-want[i])) > 0.001 {
				t.Errorf("  [%d]: got %f, want %f", i, got[i], want[i])
			}
		}
	}
	t.Logf("GGUF Q8_0 dequant: PASSED (max diff=%.6f, %d elements)", maxDiff, len(got))
}

func TestGGUFQ4_0Dequant(t *testing.T) {
	// Create a Q4_0 block manually:
	// 32 elements, scale = 0.5
	// Nibble encoding: value = nibble - 8 (unsigned 0-15 -> signed -8..+7)
	const blockSize = 32
	scale := float32(0.5)

	// Build raw block: [f16 scale] [16 bytes of packed nibbles]
	block := make([]byte, 18)
	binary.LittleEndian.PutUint16(block[0:2], f16Bits(scale))

	// Fill nibbles with known pattern.
	// lo nibble -> element [0..15], hi nibble -> element [16..31]
	want := make([]float32, blockSize)
	for i := 0; i < 16; i++ {
		loNibble := uint8(i % 16)        // 0-15
		hiNibble := uint8((15 - i) % 16) // 15-0
		block[2+i] = (hiNibble << 4) | loNibble

		// Low nibble maps to element i
		want[i] = scale * float32(int(loNibble)-8)
		// High nibble maps to element i+16
		want[i+16] = scale * float32(int(hiNibble)-8)
	}

	path := writeGGUFTestFile(t, []ggufTestTensor{
		{
			name:     "q4.weight",
			dims:     []uint64{32},
			ggmlType: ggmlTypeQ4_0,
			data:     block,
		},
	}, map[string]interface{}{
		"general.architecture": "test",
	})

	gf, err := OpenGGUF(path)
	if err != nil {
		t.Fatalf("OpenGGUF: %v", err)
	}
	defer gf.Close()

	got, err := gf.ReadTensorData("q4.weight")
	if err != nil {
		t.Fatalf("ReadTensorData: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("length: got %d, want %d", len(got), len(want))
	}

	maxDiff := float32(0)
	for i, v := range got {
		diff := float32(math.Abs(float64(v - want[i])))
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	if maxDiff > 0.01 {
		t.Errorf("Q4_0 max diff too large: %f", maxDiff)
		for i := range got {
			if math.Abs(float64(got[i]-want[i])) > 0.001 {
				t.Errorf("  [%d]: got %f, want %f", i, got[i], want[i])
			}
		}
	}
	t.Logf("GGUF Q4_0 dequant: PASSED (max diff=%.6f, %d elements)", maxDiff, len(got))
}

func TestGGUFMultipleTensors(t *testing.T) {
	w1Data := f32Bytes(1, 2, 3, 4)
	w2Data := f32Bytes(10, 20, 30, 40, 50, 60)

	path := writeGGUFTestFile(t, []ggufTestTensor{
		{name: "layer.0.weight", dims: []uint64{2, 2}, ggmlType: ggmlTypeF32, data: w1Data},
		{name: "layer.1.weight", dims: []uint64{2, 3}, ggmlType: ggmlTypeF32, data: w2Data},
	}, map[string]interface{}{
		"general.architecture": "llama",
	})

	gf, err := OpenGGUF(path)
	if err != nil {
		t.Fatalf("OpenGGUF: %v", err)
	}
	defer gf.Close()

	names := gf.TensorNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 tensors, got %d", len(names))
	}

	// Read first tensor.
	got1, err := gf.ReadTensorData("layer.0.weight")
	if err != nil {
		t.Fatalf("ReadTensorData layer.0: %v", err)
	}
	if got1[0] != 1 || got1[3] != 4 {
		t.Errorf("layer.0 data mismatch: %v", got1)
	}

	// Read second tensor.
	got2, err := gf.ReadTensorData("layer.1.weight")
	if err != nil {
		t.Fatalf("ReadTensorData layer.1: %v", err)
	}
	if got2[0] != 10 || got2[5] != 60 {
		t.Errorf("layer.1 data mismatch: %v", got2)
	}

	t.Logf("GGUF multiple tensors: PASSED (2 tensors verified)")
}
