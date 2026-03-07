package model

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
)

// SafeTensorsFile represents a parsed SafeTensors model file.
type SafeTensorsFile struct {
	path       string
	file       *os.File
	metadata   map[string]string
	tensors    map[string]*safetensorEntry
	dataOffset int64 // byte offset where tensor data begins
}

// safetensorEntry holds parsed info for one tensor from the JSON header.
type safetensorEntry struct {
	Name        string
	Dtype       DType
	Shape       []int
	OffsetBegin uint64 // relative to data section start
	OffsetEnd   uint64
}

// safetensorsHeaderEntry mirrors the JSON structure for each tensor.
type safetensorsHeaderEntry struct {
	Dtype       string   `json:"dtype"`
	Shape       []int    `json:"shape"`
	DataOffsets [2]int64 `json:"data_offsets"`
}

// OpenSafeTensors opens and parses a SafeTensors file.
//
// SafeTensors binary layout:
//   - [8 bytes] header_len (uint64 LE)
//   - [header_len bytes] JSON header (UTF-8)
//   - [remaining bytes] tensor data
func OpenSafeTensors(path string) (*SafeTensorsFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("model: open safetensors %q: %w", path, err)
	}

	// Read 8-byte header length.
	var headerLen uint64
	if err := binary.Read(f, binary.LittleEndian, &headerLen); err != nil {
		f.Close()
		return nil, fmt.Errorf("model: read header length: %w", err)
	}

	// Sanity check: header shouldn't be gigabytes.
	if headerLen > 100*1024*1024 {
		f.Close()
		return nil, fmt.Errorf("model: header too large: %d bytes", headerLen)
	}

	// Read JSON header.
	headerBytes := make([]byte, headerLen)
	if _, err := io.ReadFull(f, headerBytes); err != nil {
		f.Close()
		return nil, fmt.Errorf("model: read header: %w", err)
	}

	// Parse JSON into raw map first.
	var rawHeader map[string]json.RawMessage
	if err := json.Unmarshal(headerBytes, &rawHeader); err != nil {
		f.Close()
		return nil, fmt.Errorf("model: parse header JSON: %w", err)
	}

	sf := &SafeTensorsFile{
		path:       path,
		file:       f,
		metadata:   make(map[string]string),
		tensors:    make(map[string]*safetensorEntry),
		dataOffset: 8 + int64(headerLen), // 8 bytes for header_len + header itself
	}

	// Process each key in the header.
	for key, raw := range rawHeader {
		if key == "__metadata__" {
			// Parse metadata as string-to-string map.
			var meta map[string]string
			if err := json.Unmarshal(raw, &meta); err == nil {
				sf.metadata = meta
			}
			continue
		}

		// Parse tensor entry.
		var entry safetensorsHeaderEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			f.Close()
			return nil, fmt.Errorf("model: parse tensor %q: %w", key, err)
		}

		dtype, err := parseSafetensorsDtype(entry.Dtype)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("model: tensor %q: %w", key, err)
		}

		sf.tensors[key] = &safetensorEntry{
			Name:        key,
			Dtype:       dtype,
			Shape:       entry.Shape,
			OffsetBegin: uint64(entry.DataOffsets[0]),
			OffsetEnd:   uint64(entry.DataOffsets[1]),
		}
	}

	return sf, nil
}

// parseSafetensorsDtype converts string dtype name to DType enum.
func parseSafetensorsDtype(s string) (DType, error) {
	switch s {
	case "F32":
		return DTypeF32, nil
	case "F16":
		return DTypeF16, nil
	case "BF16":
		return DTypeBF16, nil
	case "I32":
		return DTypeI32, nil
	case "I16":
		return DTypeI16, nil
	case "I8":
		return DTypeI8, nil
	case "U8":
		return DTypeU8, nil
	default:
		return 0, fmt.Errorf("unsupported dtype %q", s)
	}
}

// Metadata returns model-level metadata.
func (sf *SafeTensorsFile) Metadata() map[string]string {
	out := make(map[string]string, len(sf.metadata))
	for k, v := range sf.metadata {
		out[k] = v
	}
	return out
}

// TensorInfos returns descriptors for all tensors.
func (sf *SafeTensorsFile) TensorInfos() []TensorInfo {
	infos := make([]TensorInfo, 0, len(sf.tensors))
	for _, e := range sf.tensors {
		size := 1
		for _, d := range e.Shape {
			size *= d
		}
		infos = append(infos, TensorInfo{
			Name:   e.Name,
			Shape:  e.Shape,
			Dtype:  e.Dtype,
			Size:   size,
			Offset: e.OffsetBegin,
			Nbytes: e.OffsetEnd - e.OffsetBegin,
		})
	}

	// Sort by name for deterministic ordering.
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// TensorNames returns sorted tensor names.
func (sf *SafeTensorsFile) TensorNames() []string {
	names := make([]string, 0, len(sf.tensors))
	for name := range sf.tensors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReadTensorData reads raw tensor bytes and converts to float32.
func (sf *SafeTensorsFile) ReadTensorData(name string) ([]float32, error) {
	entry, ok := sf.tensors[name]
	if !ok {
		return nil, fmt.Errorf("model: tensor %q not found", name)
	}

	nbytes := entry.OffsetEnd - entry.OffsetBegin
	data := make([]byte, nbytes)

	absOffset := sf.dataOffset + int64(entry.OffsetBegin)
	if _, err := sf.file.ReadAt(data, absOffset); err != nil {
		return nil, fmt.Errorf("model: read tensor %q data: %w", name, err)
	}

	switch entry.Dtype {
	case DTypeF32:
		return F32SliceFromBytes(data), nil
	case DTypeF16:
		return F16SliceToF32(data), nil
	case DTypeBF16:
		return BF16SliceToF32(data), nil
	default:
		return nil, fmt.Errorf("model: unsupported dtype %s for tensor %q", entry.Dtype, name)
	}
}

// Close releases the underlying file handle.
func (sf *SafeTensorsFile) Close() error {
	if sf.file != nil {
		return sf.file.Close()
	}
	return nil
}

// Verify SafeTensorsFile implements ModelFile at compile time.
var _ ModelFile = (*SafeTensorsFile)(nil)
