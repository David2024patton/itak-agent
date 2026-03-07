package model

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeSafeTensorsFile creates a minimal SafeTensors file for testing.
// It writes a header with the given tensors and raw f32 data.
func writeSafeTensorsFile(t *testing.T, tensors map[string]safetensorsTestTensor) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.safetensors")

	// Calculate data section layout.
	type layout struct {
		name  string
		begin int64
		end   int64
		data  []byte
	}
	var layouts []layout
	offset := int64(0)

	for name, tensor := range tensors {
		raw := tensor.toBytes()
		layouts = append(layouts, layout{
			name:  name,
			begin: offset,
			end:   offset + int64(len(raw)),
			data:  raw,
		})
		offset += int64(len(raw))
	}

	// Build JSON header.
	header := make(map[string]interface{})
	for _, l := range layouts {
		tensor := tensors[l.name]
		header[l.name] = safetensorsHeaderEntry{
			Dtype:       tensor.dtype,
			Shape:       tensor.shape,
			DataOffsets: [2]int64{l.begin, l.end},
		}
	}
	header["__metadata__"] = map[string]string{
		"format": "pt",
		"test":   "true",
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}

	// Write file: [8-byte header_len] [header JSON] [tensor data]
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer f.Close()

	headerLen := uint64(len(headerBytes))
	if err := binary.Write(f, binary.LittleEndian, headerLen); err != nil {
		t.Fatalf("write header len: %v", err)
	}
	if _, err := f.Write(headerBytes); err != nil {
		t.Fatalf("write header: %v", err)
	}
	for _, l := range layouts {
		if _, err := f.Write(l.data); err != nil {
			t.Fatalf("write tensor data: %v", err)
		}
	}

	return path
}

type safetensorsTestTensor struct {
	dtype string
	shape []int
	f32   []float32 // for F32 tensors
	f16   []uint16  // for F16 tensors
}

func (s safetensorsTestTensor) toBytes() []byte {
	switch s.dtype {
	case "F32":
		buf := make([]byte, len(s.f32)*4)
		for i, v := range s.f32 {
			binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
		}
		return buf
	case "F16":
		buf := make([]byte, len(s.f16)*2)
		for i, v := range s.f16 {
			binary.LittleEndian.PutUint16(buf[i*2:], v)
		}
		return buf
	default:
		panic("unsupported test dtype: " + s.dtype)
	}
}

// f32ToF16 converts a float32 to IEEE 754 half-precision (for test data).
func f32ToF16(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := (bits >> 31) & 1
	exp := (bits >> 23) & 0xFF
	frac := bits & 0x7FFFFF

	switch {
	case exp == 0xFF:
		// Inf/NaN
		return uint16((sign << 15) | (0x1F << 10) | (frac >> 13))
	case exp > 127+15:
		// Overflow -> Inf
		return uint16((sign << 15) | (0x1F << 10))
	case exp < 127-14:
		// Underflow -> zero
		return uint16(sign << 15)
	default:
		exp16 := exp - 127 + 15
		return uint16((sign << 15) | (exp16 << 10) | (frac >> 13))
	}
}

func TestSafeTensorsF32RoundTrip(t *testing.T) {
	data := []float32{1.0, 2.5, -3.14, 0, 42.0, 0.001}

	path := writeSafeTensorsFile(t, map[string]safetensorsTestTensor{
		"weights": {
			dtype: "F32",
			shape: []int{2, 3},
			f32:   data,
		},
	})

	sf, err := OpenSafeTensors(path)
	if err != nil {
		t.Fatalf("OpenSafeTensors: %v", err)
	}
	defer sf.Close()

	// Check tensor names.
	names := sf.TensorNames()
	if len(names) != 1 || names[0] != "weights" {
		t.Fatalf("unexpected names: %v", names)
	}

	// Check tensor info.
	infos := sf.TensorInfos()
	if len(infos) != 1 {
		t.Fatalf("expected 1 tensor, got %d", len(infos))
	}
	info := infos[0]
	if info.Name != "weights" {
		t.Errorf("name: got %q, want %q", info.Name, "weights")
	}
	if info.Dtype != DTypeF32 {
		t.Errorf("dtype: got %s, want F32", info.Dtype)
	}
	if info.Size != 6 {
		t.Errorf("size: got %d, want 6", info.Size)
	}

	// Read and verify data.
	got, err := sf.ReadTensorData("weights")
	if err != nil {
		t.Fatalf("ReadTensorData: %v", err)
	}

	if len(got) != len(data) {
		t.Fatalf("length: got %d, want %d", len(got), len(data))
	}
	for i, v := range got {
		if v != data[i] {
			t.Errorf("[%d]: got %f, want %f", i, v, data[i])
		}
	}
	t.Logf("F32 round-trip: PASSED (%d elements)", len(got))
}

func TestSafeTensorsF16Conversion(t *testing.T) {
	// Create F16 test data by converting known F32 values.
	f32Values := []float32{1.0, 2.0, 0.5, -1.0, 0, 3.5}
	f16Values := make([]uint16, len(f32Values))
	for i, v := range f32Values {
		f16Values[i] = f32ToF16(v)
	}

	path := writeSafeTensorsFile(t, map[string]safetensorsTestTensor{
		"layer.weight": {
			dtype: "F16",
			shape: []int{2, 3},
			f16:   f16Values,
		},
	})

	sf, err := OpenSafeTensors(path)
	if err != nil {
		t.Fatalf("OpenSafeTensors: %v", err)
	}
	defer sf.Close()

	got, err := sf.ReadTensorData("layer.weight")
	if err != nil {
		t.Fatalf("ReadTensorData: %v", err)
	}

	// Verify F16 -> F32 conversion accuracy.
	maxDiff := float32(0)
	for i, v := range got {
		diff := float32(math.Abs(float64(v - f32Values[i])))
		if diff > maxDiff {
			maxDiff = diff
		}
		if diff > 0.01 {
			t.Errorf("[%d]: got %f, want %f (diff=%f)", i, v, f32Values[i], diff)
		}
	}
	t.Logf("F16 conversion: PASSED (max diff=%.6f, %d elements)", maxDiff, len(got))
}

func TestSafeTensorsMetadata(t *testing.T) {
	path := writeSafeTensorsFile(t, map[string]safetensorsTestTensor{
		"w": {dtype: "F32", shape: []int{2}, f32: []float32{1, 2}},
	})

	sf, err := OpenSafeTensors(path)
	if err != nil {
		t.Fatalf("OpenSafeTensors: %v", err)
	}
	defer sf.Close()

	meta := sf.Metadata()
	if meta["format"] != "pt" {
		t.Errorf("metadata format: got %q, want %q", meta["format"], "pt")
	}
	if meta["test"] != "true" {
		t.Errorf("metadata test: got %q, want %q", meta["test"], "true")
	}
	t.Logf("Metadata: PASSED (%d keys)", len(meta))
}

func TestSafeTensorsMultipleTensors(t *testing.T) {
	path := writeSafeTensorsFile(t, map[string]safetensorsTestTensor{
		"embed.weight": {dtype: "F32", shape: []int{4, 2}, f32: []float32{1, 2, 3, 4, 5, 6, 7, 8}},
		"norm.weight":  {dtype: "F32", shape: []int{2}, f32: []float32{0.5, 1.5}},
	})

	sf, err := OpenSafeTensors(path)
	if err != nil {
		t.Fatalf("OpenSafeTensors: %v", err)
	}
	defer sf.Close()

	names := sf.TensorNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 tensors, got %d", len(names))
	}

	// Verify both tensors.
	embed, err := sf.ReadTensorData("embed.weight")
	if err != nil {
		t.Fatalf("ReadTensorData embed: %v", err)
	}
	if len(embed) != 8 || embed[0] != 1 || embed[7] != 8 {
		t.Errorf("embed data mismatch: got %v", embed)
	}

	norm, err := sf.ReadTensorData("norm.weight")
	if err != nil {
		t.Fatalf("ReadTensorData norm: %v", err)
	}
	if len(norm) != 2 || norm[0] != 0.5 || norm[1] != 1.5 {
		t.Errorf("norm data mismatch: got %v", norm)
	}

	t.Logf("Multiple tensors: PASSED (2 tensors verified)")
}

func TestF16Conversion(t *testing.T) {
	// Test specific F16 edge cases.
	tests := []struct {
		name string
		f16  uint16
		want float32
	}{
		{"zero", 0x0000, 0},
		{"one", 0x3C00, 1.0},
		{"neg_one", 0xBC00, -1.0},
		{"half", 0x3800, 0.5},
		{"two", 0x4000, 2.0},
		{"neg_zero", 0x8000, 0}, // -0 is still 0
	}

	for _, tc := range tests {
		got := F16ToF32(tc.f16)
		// Handle -0 vs +0
		if tc.want == 0 && got == 0 {
			continue
		}
		if got != tc.want {
			t.Errorf("F16ToF32(%s=0x%04X): got %f, want %f", tc.name, tc.f16, got, tc.want)
		}
	}
	t.Logf("F16 edge cases: PASSED (%d cases)", len(tests))
}

func TestBF16Conversion(t *testing.T) {
	// BF16 is the upper 16 bits of F32.
	tests := []struct {
		f32  float32
		bf16 uint16
	}{
		{1.0, 0x3F80},
		{-1.0, 0xBF80},
		{2.0, 0x4000},
		{0.5, 0x3F00},
	}

	for _, tc := range tests {
		got := BF16ToF32(tc.bf16)
		if got != tc.f32 {
			t.Errorf("BF16ToF32(0x%04X): got %f, want %f", tc.bf16, got, tc.f32)
		}
	}
	t.Logf("BF16 conversion: PASSED (%d cases)", len(tests))
}
