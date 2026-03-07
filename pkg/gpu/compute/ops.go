package compute

import (
	"embed"
	"fmt"
	"math"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

//go:embed shaders/*.wgsl
var shaderFS embed.FS

// Cached shader sources loaded once at init.
var (
	matmulShaderSrc      string
	elementwiseShaderSrc string
	softmaxShaderSrc     string
	rmsnormShaderSrc     string
	siluShaderSrc        string
	ropeShaderSrc        string
	embeddingShaderSrc   string
	linearShaderSrc      string
)

func init() {
	mustLoad := func(name string) string {
		data, err := shaderFS.ReadFile("shaders/" + name)
		if err != nil {
			panic(fmt.Sprintf("compute: missing embedded shader %s: %v", name, err))
		}
		return string(data)
	}

	matmulShaderSrc = mustLoad("matmul.wgsl")
	elementwiseShaderSrc = mustLoad("elementwise.wgsl")
	softmaxShaderSrc = mustLoad("softmax.wgsl")
	rmsnormShaderSrc = mustLoad("rmsnorm.wgsl")
	siluShaderSrc = mustLoad("silu.wgsl")
	ropeShaderSrc = mustLoad("rope.wgsl")
	embeddingShaderSrc = mustLoad("embedding.wgsl")
	linearShaderSrc = mustLoad("linear.wgsl")
}

// minUniformSize is the minimum uniform buffer size required by Vulkan.
// Uniform buffers must be at least 16 bytes to satisfy alignment requirements.
const minUniformSize = 16

// padTo16 pads a byte slice to at least 16 bytes with zeros.
func padTo16(data []byte) []byte {
	if len(data) >= minUniformSize {
		return data
	}
	padded := make([]byte, minUniformSize)
	copy(padded, data)
	return padded
}

// ---------------------------------------------------------------------------
// MatMul: C = A * B
// A is [M, K], B is [K, N], result is [M, N]
// ---------------------------------------------------------------------------

// MatMul performs matrix multiplication C = A * B on the GPU.
// A must be [M, K] and B must be [K, N]. Returns a [M, N] tensor.
func MatMul(dev *Device, a, b *Tensor) (*Tensor, error) {
	if len(a.shape) != 2 || len(b.shape) != 2 {
		return nil, fmt.Errorf("compute.MatMul: requires 2D tensors, got %v and %v", a.shape, b.shape)
	}

	m, k := a.shape[0], a.shape[1]
	k2, n := b.shape[0], b.shape[1]
	if k != k2 {
		return nil, fmt.Errorf("compute.MatMul: inner dimensions mismatch: %d vs %d", k, k2)
	}

	out, err := NewEmptyTensor(dev, []int{m, n})
	if err != nil {
		return nil, err
	}

	// Params buffer: {m, n, k} as uint32, padded to 16 bytes.
	paramsData := make([]byte, 12)
	copy(paramsData[0:4], uint32ToBytes(uint32(m)))
	copy(paramsData[4:8], uint32ToBytes(uint32(n)))
	copy(paramsData[8:12], uint32ToBytes(uint32(k)))

	if err := dispatchCompute(dev, "matmul", matmulShaderSrc, "main",
		[]*wgpu.Buffer{a.buf, b.buf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{a.nbytes, b.nbytes, out.nbytes},
		padTo16(paramsData),
		// Workgroups: ceil(M/16), ceil(N/16), 1
		uint32(math.Ceil(float64(m)/16.0)),
		uint32(math.Ceil(float64(n)/16.0)),
		1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.MatMul: %w", err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Add: out = a + b (elementwise)
// ---------------------------------------------------------------------------

// Add performs elementwise addition. Tensors must have the same total size.
func Add(dev *Device, a, b *Tensor) (*Tensor, error) {
	return elementwiseOp(dev, a, b, 0, "add")
}

// ---------------------------------------------------------------------------
// Mul: out = a * b (elementwise)
// ---------------------------------------------------------------------------

// Mul performs elementwise multiplication. Tensors must have the same total size.
func Mul(dev *Device, a, b *Tensor) (*Tensor, error) {
	return elementwiseOp(dev, a, b, 1, "mul")
}

// ---------------------------------------------------------------------------
// Scale: out = a * scalar
// ---------------------------------------------------------------------------

// Scale multiplies every element of a tensor by a scalar value.
func Scale(dev *Device, a *Tensor, scalar float32) (*Tensor, error) {
	// Create a 1-element tensor for the scalar.
	scalarTensor, err := NewTensor(dev, []int{1}, []float32{scalar})
	if err != nil {
		return nil, err
	}
	defer scalarTensor.Release()

	return elementwiseOp(dev, a, scalarTensor, 2, "scale")
}

// elementwiseOp dispatches an elementwise shader with the given op code.
func elementwiseOp(dev *Device, a, b *Tensor, op uint32, label string) (*Tensor, error) {
	if op != 2 && a.size != b.size {
		return nil, fmt.Errorf("compute.%s: size mismatch: %d vs %d", label, a.size, b.size)
	}

	out, err := NewEmptyTensor(dev, a.Shape())
	if err != nil {
		return nil, err
	}

	// Params: {len, op}, padded to 16 bytes.
	paramsData := make([]byte, 8)
	copy(paramsData[0:4], uint32ToBytes(uint32(a.size)))
	copy(paramsData[4:8], uint32ToBytes(op))

	if err := dispatchCompute(dev, "elementwise_"+label, elementwiseShaderSrc, "main",
		[]*wgpu.Buffer{a.buf, b.buf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{a.nbytes, b.nbytes, out.nbytes},
		padTo16(paramsData),
		// Workgroups: ceil(len/256)
		uint32(math.Ceil(float64(a.size)/256.0)),
		1, 1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.%s: %w", label, err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Softmax: row-wise softmax
// ---------------------------------------------------------------------------

// Softmax computes row-wise softmax on a 2D tensor [rows, cols].
func Softmax(dev *Device, a *Tensor) (*Tensor, error) {
	if len(a.shape) != 2 {
		return nil, fmt.Errorf("compute.Softmax: requires 2D tensor, got shape %v", a.shape)
	}

	rows, cols := a.shape[0], a.shape[1]

	out, err := NewEmptyTensor(dev, []int{rows, cols})
	if err != nil {
		return nil, err
	}

	// Params: {rows, cols}, padded to 16 bytes.
	paramsData := make([]byte, 8)
	copy(paramsData[0:4], uint32ToBytes(uint32(rows)))
	copy(paramsData[4:8], uint32ToBytes(uint32(cols)))

	if err := dispatchCompute(dev, "softmax", softmaxShaderSrc, "main",
		[]*wgpu.Buffer{a.buf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{a.nbytes, out.nbytes},
		padTo16(paramsData),
		// Workgroups: one per row, 1, 1
		uint32(rows), 1, 1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.Softmax: %w", err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// RMSNorm: out = (x / rms(x)) * weight
// ---------------------------------------------------------------------------

// RMSNorm computes RMS normalization on a 2D tensor [rows, cols].
// weight must be a 1D tensor of length cols.
func RMSNorm(dev *Device, a, weight *Tensor, eps float32) (*Tensor, error) {
	if len(a.shape) != 2 {
		return nil, fmt.Errorf("compute.RMSNorm: requires 2D input tensor, got shape %v", a.shape)
	}

	rows, cols := a.shape[0], a.shape[1]

	if weight.size != cols {
		return nil, fmt.Errorf("compute.RMSNorm: weight size %d doesn't match cols %d", weight.size, cols)
	}

	out, err := NewEmptyTensor(dev, []int{rows, cols})
	if err != nil {
		return nil, err
	}

	// Params: {rows, cols, eps}, padded to 16 bytes.
	paramsData := make([]byte, 12)
	copy(paramsData[0:4], uint32ToBytes(uint32(rows)))
	copy(paramsData[4:8], uint32ToBytes(uint32(cols)))
	copy(paramsData[8:12], float32Bytes(eps))

	if err := dispatchCompute(dev, "rmsnorm", rmsnormShaderSrc, "main",
		[]*wgpu.Buffer{a.buf, weight.buf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{a.nbytes, weight.nbytes, out.nbytes},
		padTo16(paramsData),
		// Workgroups: one per row
		uint32(rows), 1, 1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.RMSNorm: %w", err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// SiLU: out = x * sigmoid(x)
// ---------------------------------------------------------------------------

// SiLU applies the SiLU (Swish) activation function elementwise.
// SiLU(x) = x * sigmoid(x) = x / (1 + exp(-x))
func SiLU(dev *Device, a *Tensor) (*Tensor, error) {
	return activationOp(dev, a, 0, "silu")
}

// ---------------------------------------------------------------------------
// GELU: out = 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715*x^3)))
// ---------------------------------------------------------------------------

// GELU applies the GELU activation function elementwise (tanh approximation).
func GELU(dev *Device, a *Tensor) (*Tensor, error) {
	return activationOp(dev, a, 1, "gelu")
}

// activationOp dispatches a SiLU or GELU shader with the given op code.
func activationOp(dev *Device, a *Tensor, op uint32, label string) (*Tensor, error) {
	out, err := NewEmptyTensor(dev, a.Shape())
	if err != nil {
		return nil, err
	}

	// Params: {len, op}, padded to 16 bytes.
	paramsData := make([]byte, 8)
	copy(paramsData[0:4], uint32ToBytes(uint32(a.size)))
	copy(paramsData[4:8], uint32ToBytes(op))

	if err := dispatchCompute(dev, "activation_"+label, siluShaderSrc, "main",
		[]*wgpu.Buffer{a.buf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{a.nbytes, out.nbytes},
		padTo16(paramsData),
		uint32(math.Ceil(float64(a.size)/256.0)),
		1, 1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.%s: %w", label, err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// RoPE: Rotary Position Embeddings
// ---------------------------------------------------------------------------

// RoPE applies Rotary Position Embeddings to a [seq_len, head_dim] tensor.
// head_dim must be even. theta defaults to 10000.0 in standard Llama models.
func RoPE(dev *Device, a *Tensor, theta float32) (*Tensor, error) {
	if len(a.shape) != 2 {
		return nil, fmt.Errorf("compute.RoPE: requires 2D tensor, got shape %v", a.shape)
	}

	seqLen, headDim := a.shape[0], a.shape[1]
	if headDim%2 != 0 {
		return nil, fmt.Errorf("compute.RoPE: head_dim must be even, got %d", headDim)
	}

	out, err := NewEmptyTensor(dev, []int{seqLen, headDim})
	if err != nil {
		return nil, err
	}

	// Params: {seq_len, head_dim, theta}, padded to 16 bytes.
	paramsData := make([]byte, 12)
	copy(paramsData[0:4], uint32ToBytes(uint32(seqLen)))
	copy(paramsData[4:8], uint32ToBytes(uint32(headDim)))
	copy(paramsData[8:12], float32Bytes(theta))

	totalPairs := seqLen * (headDim / 2)

	if err := dispatchCompute(dev, "rope", ropeShaderSrc, "main",
		[]*wgpu.Buffer{a.buf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{a.nbytes, out.nbytes},
		padTo16(paramsData),
		uint32(math.Ceil(float64(totalPairs)/256.0)),
		1, 1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.RoPE: %w", err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Embedding: token ID lookup
// ---------------------------------------------------------------------------

// Embedding looks up token embeddings from a weight table.
// tokenIDs is a [num_tokens] u32 tensor (created via NewUint32Tensor).
// weights is a [vocab_size, embed_dim] f32 tensor.
// Returns a [num_tokens, embed_dim] tensor.
func Embedding(dev *Device, tokenIDs, weights *Tensor) (*Tensor, error) {
	if len(tokenIDs.shape) != 1 {
		return nil, fmt.Errorf("compute.Embedding: tokenIDs must be 1D, got shape %v", tokenIDs.shape)
	}
	if len(weights.shape) != 2 {
		return nil, fmt.Errorf("compute.Embedding: weights must be 2D, got shape %v", weights.shape)
	}

	numTokens := tokenIDs.shape[0]
	embedDim := weights.shape[1]

	out, err := NewEmptyTensor(dev, []int{numTokens, embedDim})
	if err != nil {
		return nil, err
	}

	// Params: {num_tokens, embed_dim}, padded to 16 bytes.
	paramsData := make([]byte, 8)
	copy(paramsData[0:4], uint32ToBytes(uint32(numTokens)))
	copy(paramsData[4:8], uint32ToBytes(uint32(embedDim)))

	totalElements := numTokens * embedDim

	if err := dispatchCompute(dev, "embedding", embeddingShaderSrc, "main",
		[]*wgpu.Buffer{tokenIDs.buf, weights.buf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{tokenIDs.nbytes, weights.nbytes, out.nbytes},
		padTo16(paramsData),
		uint32(math.Ceil(float64(totalElements)/256.0)),
		1, 1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.Embedding: %w", err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Linear: out = x @ weight^T + bias
// ---------------------------------------------------------------------------

// Linear applies a fused linear transformation: out = x @ weight^T + bias.
// x is [batch, in_features], weight is [out_features, in_features].
// bias is optional (pass nil to skip). If provided, bias is [out_features].
// Returns [batch, out_features].
//
// Uses a fused WGSL shader that handles weight transpose and bias addition
// in a single GPU dispatch, avoiding the Vulkan fence pool exhaustion that
// occurs with compound multi-dispatch operations.
func Linear(dev *Device, x, weight, bias *Tensor) (*Tensor, error) {
	if len(x.shape) != 2 || len(weight.shape) != 2 {
		return nil, fmt.Errorf("compute.Linear: requires 2D tensors, got x=%v weight=%v", x.shape, weight.shape)
	}

	batch, inFeatures := x.shape[0], x.shape[1]
	outFeatures, wIn := weight.shape[0], weight.shape[1]
	if inFeatures != wIn {
		return nil, fmt.Errorf("compute.Linear: in_features mismatch: x has %d, weight has %d", inFeatures, wIn)
	}

	if bias != nil && bias.size != outFeatures {
		return nil, fmt.Errorf("compute.Linear: bias size %d doesn't match out_features %d", bias.size, outFeatures)
	}

	out, err := NewEmptyTensor(dev, []int{batch, outFeatures})
	if err != nil {
		return nil, err
	}

	// Params: {M, N, K, has_bias}
	hasBias := uint32(0)
	if bias != nil {
		hasBias = 1
	}
	paramsData := make([]byte, 16)
	copy(paramsData[0:4], uint32ToBytes(uint32(batch)))
	copy(paramsData[4:8], uint32ToBytes(uint32(outFeatures)))
	copy(paramsData[8:12], uint32ToBytes(uint32(inFeatures)))
	copy(paramsData[12:16], uint32ToBytes(hasBias))

	// If no bias, create a dummy 1-element buffer (shader won't read it).
	var biasBuf *wgpu.Buffer
	var biasNbytes uint64
	if bias != nil {
		biasBuf = bias.buf
		biasNbytes = bias.nbytes
	} else {
		// Create a minimal dummy buffer for the bind group.
		dummyBias, dErr := NewTensor(dev, []int{1}, []float32{0})
		if dErr != nil {
			out.Release()
			return nil, fmt.Errorf("compute.Linear: create dummy bias: %w", dErr)
		}
		defer dummyBias.Release()
		biasBuf = dummyBias.buf
		biasNbytes = dummyBias.nbytes
	}

	if err := dispatchCompute(dev, "linear", linearShaderSrc, "main",
		[]*wgpu.Buffer{x.buf, weight.buf, biasBuf, out.buf},
		[]gputypes.BufferBindingType{
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeReadOnlyStorage,
			gputypes.BufferBindingTypeStorage,
		},
		[]uint64{x.nbytes, weight.nbytes, biasNbytes, out.nbytes},
		paramsData, // already 16 bytes, no padding needed
		uint32(math.Ceil(float64(outFeatures)/16.0)),
		uint32(math.Ceil(float64(batch)/16.0)),
		1,
	); err != nil {
		out.Release()
		return nil, fmt.Errorf("compute.Linear: %w", err)
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// Generic dispatch helper
// ---------------------------------------------------------------------------

// dispatchCompute is the shared dispatch logic for all compute operations.
// It creates fresh bind group layout and pipeline each call to avoid
// lifecycle issues with cached layouts being released.
func dispatchCompute(
	dev *Device,
	label string,
	shaderSrc string,
	entryPoint string,
	buffers []*wgpu.Buffer,
	bindingTypes []gputypes.BufferBindingType,
	bufferSizes []uint64,
	paramsData []byte,
	wgX, wgY, wgZ uint32,
) error {
	paramsSize := uint64(len(paramsData))

	// Build bind group layout entries: storage buffers + uniform params.
	entries := make([]wgpu.BindGroupLayoutEntry, len(buffers)+1)
	for i, bt := range bindingTypes {
		entries[i] = wgpu.BindGroupLayoutEntry{
			Binding:    uint32(i),
			Visibility: wgpu.ShaderStageCompute,
			Buffer:     &gputypes.BufferBindingLayout{Type: bt},
		}
	}
	// Params uniform at the last binding.
	paramBinding := uint32(len(buffers))
	entries[len(buffers)] = wgpu.BindGroupLayoutEntry{
		Binding:    paramBinding,
		Visibility: wgpu.ShaderStageCompute,
		Buffer:     &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform},
	}

	bgl, err := dev.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label:   label + "_bgl",
		Entries: entries,
	})
	if err != nil {
		return fmt.Errorf("create bind group layout: %w", err)
	}
	defer bgl.Release()

	pipelineLayout, err := dev.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            label + "_pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{bgl},
	})
	if err != nil {
		return fmt.Errorf("create pipeline layout: %w", err)
	}
	defer pipelineLayout.Release()

	// Compile shader module.
	module, err := dev.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: label,
		WGSL:  shaderSrc,
	})
	if err != nil {
		return fmt.Errorf("compile shader %q: %w", label, err)
	}
	defer module.Release()

	pipeline, err := dev.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      label,
		Layout:     pipelineLayout,
		Module:     module,
		EntryPoint: entryPoint,
	})
	if err != nil {
		return fmt.Errorf("create pipeline %q: %w", label, err)
	}
	defer pipeline.Release()

	// Create params uniform buffer.
	paramsBuf, err := dev.createBuffer(label+"_params", paramsSize,
		wgpu.BufferUsageUniform|wgpu.BufferUsageCopyDst)
	if err != nil {
		return fmt.Errorf("create params buffer: %w", err)
	}
	defer paramsBuf.Release()

	if err := dev.writeBuffer(paramsBuf, 0, paramsData); err != nil {
		return fmt.Errorf("write params: %w", err)
	}

	// Create bind group entries.
	bgEntries := make([]wgpu.BindGroupEntry, len(buffers)+1)
	for i, buf := range buffers {
		bgEntries[i] = wgpu.BindGroupEntry{
			Binding: uint32(i),
			Buffer:  buf,
			Size:    bufferSizes[i],
		}
	}
	bgEntries[len(buffers)] = wgpu.BindGroupEntry{
		Binding: paramBinding,
		Buffer:  paramsBuf,
		Size:    paramsSize,
	}

	bindGroup, err := dev.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   label + "_bg",
		Layout:  bgl,
		Entries: bgEntries,
	})
	if err != nil {
		return fmt.Errorf("create bind group: %w", err)
	}
	defer bindGroup.Release()

	// Encode and dispatch.
	encoder, err := dev.device.CreateCommandEncoder(nil)
	if err != nil {
		return fmt.Errorf("create command encoder: %w", err)
	}

	computePass, err := encoder.BeginComputePass(nil)
	if err != nil {
		return fmt.Errorf("begin compute pass: %w", err)
	}

	computePass.SetPipeline(pipeline)
	computePass.SetBindGroup(0, bindGroup, nil)
	computePass.Dispatch(wgX, wgY, wgZ)

	if err := computePass.End(); err != nil {
		return fmt.Errorf("end compute pass: %w", err)
	}

	cmdBuf, err := encoder.Finish()
	if err != nil {
		return fmt.Errorf("finish command encoder: %w", err)
	}

	if err := dev.queue.Submit(cmdBuf); err != nil {
		return fmt.Errorf("submit: %w", err)
	}

	return nil
}
