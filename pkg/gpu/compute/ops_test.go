package compute

import (
	"fmt"
	"math"
	"testing"
)

// cpuMatMul computes C = A * B on the CPU for reference.
func cpuMatMul(a []float32, m, k int, b []float32, n int) []float32 {
	c := make([]float32, m*n)
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			var sum float32
			for l := 0; l < k; l++ {
				sum += a[i*k+l] * b[l*n+j]
			}
			c[i*n+j] = sum
		}
	}
	return c
}

// cpuSoftmax computes row-wise softmax on the CPU.
func cpuSoftmax(data []float32, rows, cols int) []float32 {
	out := make([]float32, len(data))
	for r := 0; r < rows; r++ {
		rowStart := r * cols
		maxVal := data[rowStart]
		for c := 1; c < cols; c++ {
			if data[rowStart+c] > maxVal {
				maxVal = data[rowStart+c]
			}
		}
		var sum float32
		for c := 0; c < cols; c++ {
			out[rowStart+c] = float32(math.Exp(float64(data[rowStart+c] - maxVal)))
			sum += out[rowStart+c]
		}
		for c := 0; c < cols; c++ {
			out[rowStart+c] /= sum
		}
	}
	return out
}

// cpuRMSNorm computes RMS normalization on the CPU.
func cpuRMSNorm(data []float32, weight []float32, rows, cols int, eps float32) []float32 {
	out := make([]float32, len(data))
	for r := 0; r < rows; r++ {
		rowStart := r * cols
		var sumSq float32
		for c := 0; c < cols; c++ {
			v := data[rowStart+c]
			sumSq += v * v
		}
		rms := float32(math.Sqrt(float64(sumSq/float32(cols) + eps)))
		invRms := 1.0 / rms
		for c := 0; c < cols; c++ {
			out[rowStart+c] = data[rowStart+c] * invRms * weight[c]
		}
	}
	return out
}

// assertClose checks that two float32 slices are approximately equal.
func assertClose(t *testing.T, label string, got, want []float32, tol float32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: length mismatch: got %d, want %d", label, len(got), len(want))
	}
	maxDiff := float32(0)
	for i := range got {
		diff := float32(math.Abs(float64(got[i] - want[i])))
		if diff > maxDiff {
			maxDiff = diff
		}
		if diff > tol {
			t.Errorf("%s[%d]: got %.6f, want %.6f (diff=%.6f, tol=%.6f)", label, i, got[i], want[i], diff, tol)
			if i > 5 {
				t.Fatalf("%s: too many mismatches, stopping", label)
			}
		}
	}
	if maxDiff <= tol {
		t.Logf("%s: PASSED (max diff=%.6f within tol=%.6f)", label, maxDiff, tol)
	}
}

// newTestDevice creates a fresh GPU device for each test.
// This avoids Vulkan fence pool state contamination between tests.
func newTestDevice(t *testing.T) *Device {
	t.Helper()
	dev, err := NewDevice()
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	t.Cleanup(func() { dev.Release() })
	return dev
}

func TestDeviceInit(t *testing.T) {
	dev := newTestDevice(t)
	if dev.AdapterName == "" {
		t.Error("AdapterName is empty")
	}
	if dev.Backend == "" {
		t.Error("Backend is empty")
	}
	t.Logf("Adapter: %s", dev.AdapterName)
	t.Logf("Backend: %s", dev.Backend)
	t.Logf("Type:    %s", dev.DeviceType)
}

func TestTensorRoundTrip(t *testing.T) {
	dev := newTestDevice(t)

	data := []float32{1, 2, 3, 4, 5, 6}
	tensor, err := NewTensor(dev, []int{2, 3}, data)
	if err != nil {
		t.Fatalf("NewTensor: %v", err)
	}
	defer tensor.Release()

	got, err := tensor.ToCPU()
	if err != nil {
		t.Fatalf("ToCPU: %v", err)
	}

	assertClose(t, "roundtrip", got, data, 0)
}

func TestMatMul(t *testing.T) {
	dev := newTestDevice(t)

	// 4x3 * 3x5 = 4x5
	m, k, n := 4, 3, 5
	aData := make([]float32, m*k)
	bData := make([]float32, k*n)
	for i := range aData {
		aData[i] = float32(i+1) * 0.1
	}
	for i := range bData {
		bData[i] = float32(i+1) * 0.05
	}

	a, err := NewTensor(dev, []int{m, k}, aData)
	if err != nil {
		t.Fatalf("NewTensor A: %v", err)
	}
	defer a.Release()

	b, err := NewTensor(dev, []int{k, n}, bData)
	if err != nil {
		t.Fatalf("NewTensor B: %v", err)
	}
	defer b.Release()

	c, err := MatMul(dev, a, b)
	if err != nil {
		t.Fatalf("MatMul: %v", err)
	}
	defer c.Release()

	got, err := c.ToCPU()
	if err != nil {
		t.Fatalf("ToCPU: %v", err)
	}

	want := cpuMatMul(aData, m, k, bData, n)
	assertClose(t, "MatMul", got, want, 0.001)
}

func TestMatMulLarge(t *testing.T) {
	dev := newTestDevice(t)

	// 128x64 * 64x96 = 128x96
	m, k, n := 128, 64, 96
	aData := make([]float32, m*k)
	bData := make([]float32, k*n)
	for i := range aData {
		aData[i] = float32(i%7) * 0.3
	}
	for i := range bData {
		bData[i] = float32(i%11) * 0.2
	}

	a, err := NewTensor(dev, []int{m, k}, aData)
	if err != nil {
		t.Fatalf("NewTensor A: %v", err)
	}
	defer a.Release()

	b, err := NewTensor(dev, []int{k, n}, bData)
	if err != nil {
		t.Fatalf("NewTensor B: %v", err)
	}
	defer b.Release()

	c, err := MatMul(dev, a, b)
	if err != nil {
		t.Fatalf("MatMul: %v", err)
	}
	defer c.Release()

	got, err := c.ToCPU()
	if err != nil {
		t.Fatalf("ToCPU: %v", err)
	}

	want := cpuMatMul(aData, m, k, bData, n)
	assertClose(t, "MatMulLarge", got, want, 0.05)
}

func TestAdd(t *testing.T) {
	dev := newTestDevice(t)

	a, _ := NewTensor(dev, []int{4}, []float32{1, 2, 3, 4})
	defer a.Release()
	b, _ := NewTensor(dev, []int{4}, []float32{10, 20, 30, 40})
	defer b.Release()

	c, err := Add(dev, a, b)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()
	assertClose(t, "Add", got, []float32{11, 22, 33, 44}, 0.0001)
}

func TestMul(t *testing.T) {
	dev := newTestDevice(t)

	a, _ := NewTensor(dev, []int{4}, []float32{1, 2, 3, 4})
	defer a.Release()
	b, _ := NewTensor(dev, []int{4}, []float32{2, 3, 4, 5})
	defer b.Release()

	c, err := Mul(dev, a, b)
	if err != nil {
		t.Fatalf("Mul: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()
	assertClose(t, "Mul", got, []float32{2, 6, 12, 20}, 0.0001)
}

func TestScale(t *testing.T) {
	dev := newTestDevice(t)

	a, _ := NewTensor(dev, []int{4}, []float32{1, 2, 3, 4})
	defer a.Release()

	c, err := Scale(dev, a, 2.5)
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()
	assertClose(t, "Scale", got, []float32{2.5, 5.0, 7.5, 10.0}, 0.0001)
}

func TestSoftmax(t *testing.T) {
	dev := newTestDevice(t)

	rows, cols := 2, 4
	data := []float32{1, 2, 3, 4, 5, 6, 7, 8}

	a, _ := NewTensor(dev, []int{rows, cols}, data)
	defer a.Release()

	c, err := Softmax(dev, a)
	if err != nil {
		t.Fatalf("Softmax: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()
	want := cpuSoftmax(data, rows, cols)
	assertClose(t, "Softmax", got, want, 0.0001)

	// Verify rows sum to 1.
	for r := 0; r < rows; r++ {
		var sum float32
		for c := 0; c < cols; c++ {
			sum += got[r*cols+c]
		}
		if math.Abs(float64(sum-1.0)) > 0.001 {
			t.Errorf("Softmax row %d sums to %.6f, want 1.0", r, sum)
		}
	}
}

func TestRMSNorm(t *testing.T) {
	dev := newTestDevice(t)

	rows, cols := 2, 4
	data := []float32{1, 2, 3, 4, 5, 6, 7, 8}
	weights := []float32{1, 1, 1, 1}
	eps := float32(1e-5)

	a, _ := NewTensor(dev, []int{rows, cols}, data)
	defer a.Release()
	w, _ := NewTensor(dev, []int{cols}, weights)
	defer w.Release()

	c, err := RMSNorm(dev, a, w, eps)
	if err != nil {
		t.Fatalf("RMSNorm: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()
	want := cpuRMSNorm(data, weights, rows, cols, eps)
	assertClose(t, "RMSNorm", got, want, 0.001)
}

func TestSiLU(t *testing.T) {
	dev := newTestDevice(t)

	data := []float32{-2, -1, 0, 0.5, 1, 2, 3, 5}
	a, _ := NewTensor(dev, []int{8}, data)
	defer a.Release()

	c, err := SiLU(dev, a)
	if err != nil {
		t.Fatalf("SiLU: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()

	// CPU reference: SiLU(x) = x * sigmoid(x) = x / (1 + exp(-x))
	want := make([]float32, len(data))
	for i, x := range data {
		want[i] = float32(float64(x) / (1.0 + math.Exp(-float64(x))))
	}
	assertClose(t, "SiLU", got, want, 0.0001)
}

func TestGELU(t *testing.T) {
	dev := newTestDevice(t)

	data := []float32{-2, -1, 0, 0.5, 1, 2, 3, 5}
	a, _ := NewTensor(dev, []int{8}, data)
	defer a.Release()

	c, err := GELU(dev, a)
	if err != nil {
		t.Fatalf("GELU: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()

	// CPU reference: GELU (tanh approx) = 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715*x^3)))
	want := make([]float32, len(data))
	sqrtTwoPi := math.Sqrt(2.0 / math.Pi)
	for i, x := range data {
		xf := float64(x)
		inner := sqrtTwoPi * (xf + 0.044715*xf*xf*xf)
		want[i] = float32(0.5 * xf * (1.0 + math.Tanh(inner)))
	}
	assertClose(t, "GELU", got, want, 0.001)
}

func TestRoPE(t *testing.T) {
	dev := newTestDevice(t)

	seqLen, headDim := 4, 8
	theta := float32(10000.0)

	// Fill with sequential values.
	data := make([]float32, seqLen*headDim)
	for i := range data {
		data[i] = float32(i+1) * 0.1
	}

	a, _ := NewTensor(dev, []int{seqLen, headDim}, data)
	defer a.Release()

	c, err := RoPE(dev, a, theta)
	if err != nil {
		t.Fatalf("RoPE: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()

	// CPU reference: rotate pairs by (cos, sin) of position-dependent angles.
	want := make([]float32, len(data))
	halfDim := headDim / 2
	for pos := 0; pos < seqLen; pos++ {
		for d := 0; d < halfDim; d++ {
			freqExp := 2.0 * float64(d) / float64(headDim)
			freq := 1.0 / math.Pow(float64(theta), freqExp)
			angle := float64(pos) * freq
			cosA := float32(math.Cos(angle))
			sinA := float32(math.Sin(angle))

			base := pos*headDim + d*2
			x0 := data[base]
			x1 := data[base+1]
			want[base] = x0*cosA - x1*sinA
			want[base+1] = x0*sinA + x1*cosA
		}
	}
	assertClose(t, "RoPE", got, want, 0.001)
}

func TestEmbedding(t *testing.T) {
	dev := newTestDevice(t)

	// Vocab size 5, embed dim 3.
	vocabSize, embedDim := 5, 3
	weightsData := make([]float32, vocabSize*embedDim)
	for i := range weightsData {
		weightsData[i] = float32(i+1) * 0.1
	}
	// weightsData layout:
	//   token 0: [0.1, 0.2, 0.3]
	//   token 1: [0.4, 0.5, 0.6]
	//   token 2: [0.7, 0.8, 0.9]
	//   token 3: [1.0, 1.1, 1.2]
	//   token 4: [1.3, 1.4, 1.5]

	weights, _ := NewTensor(dev, []int{vocabSize, embedDim}, weightsData)
	defer weights.Release()

	tokenIDs, _ := NewUint32Tensor(dev, []int{4}, []uint32{2, 0, 4, 1})
	defer tokenIDs.Release()

	c, err := Embedding(dev, tokenIDs, weights)
	if err != nil {
		t.Fatalf("Embedding: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()

	// Expected: rows for tokens [2, 0, 4, 1]
	want := []float32{
		0.7, 0.8, 0.9, // token 2
		0.1, 0.2, 0.3, // token 0
		1.3, 1.4, 1.5, // token 4
		0.4, 0.5, 0.6, // token 1
	}
	assertClose(t, "Embedding", got, want, 0.0001)
}

func TestLinear(t *testing.T) {
	dev := newTestDevice(t)

	// x: [2, 3], weight: [4, 3] (out_features=4, in_features=3), bias: [4]
	batch, inF, outF := 2, 3, 4
	xData := []float32{1, 2, 3, 4, 5, 6}
	wData := []float32{
		0.1, 0.2, 0.3, // out_feature 0
		0.4, 0.5, 0.6, // out_feature 1
		0.7, 0.8, 0.9, // out_feature 2
		1.0, 1.1, 1.2, // out_feature 3
	}
	biasData := []float32{0.01, 0.02, 0.03, 0.04}

	x, _ := NewTensor(dev, []int{batch, inF}, xData)
	defer x.Release()
	w, _ := NewTensor(dev, []int{outF, inF}, wData)
	defer w.Release()
	bias, _ := NewTensor(dev, []int{outF}, biasData)
	defer bias.Release()

	c, err := Linear(dev, x, w, bias)
	if err != nil {
		t.Fatalf("Linear: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()

	// CPU reference: out = x @ w^T + bias
	// w^T is [3, 4], so x [2,3] @ w^T [3,4] = [2,4]
	want := make([]float32, batch*outF)
	for r := 0; r < batch; r++ {
		for j := 0; j < outF; j++ {
			var sum float32
			for k := 0; k < inF; k++ {
				sum += xData[r*inF+k] * wData[j*inF+k]
			}
			want[r*outF+j] = sum + biasData[j]
		}
	}
	assertClose(t, "Linear", got, want, 0.001)
}

func TestLinearNoBias(t *testing.T) {
	dev := newTestDevice(t)

	batch, inF, outF := 2, 3, 4
	xData := []float32{1, 2, 3, 4, 5, 6}
	wData := []float32{
		0.1, 0.2, 0.3,
		0.4, 0.5, 0.6,
		0.7, 0.8, 0.9,
		1.0, 1.1, 1.2,
	}

	x, _ := NewTensor(dev, []int{batch, inF}, xData)
	defer x.Release()
	w, _ := NewTensor(dev, []int{outF, inF}, wData)
	defer w.Release()

	c, err := Linear(dev, x, w, nil)
	if err != nil {
		t.Fatalf("LinearNoBias: %v", err)
	}
	defer c.Release()

	got, _ := c.ToCPU()

	// CPU: out = x @ w^T (no bias)
	want := make([]float32, batch*outF)
	for r := 0; r < batch; r++ {
		for j := 0; j < outF; j++ {
			var sum float32
			for k := 0; k < inF; k++ {
				sum += xData[r*inF+k] * wData[j*inF+k]
			}
			want[r*outF+j] = sum
		}
	}
	assertClose(t, "LinearNoBias", got, want, 0.001)
}

func BenchmarkMatMul512(b *testing.B) {
	dev, err := NewDevice()
	if err != nil {
		b.Fatal(err)
	}
	defer dev.Release()

	n := 512
	aData := make([]float32, n*n)
	bData := make([]float32, n*n)
	for i := range aData {
		aData[i] = float32(i%100) * 0.01
	}
	for i := range bData {
		bData[i] = float32(i%100) * 0.01
	}

	a, _ := NewTensor(dev, []int{n, n}, aData)
	defer a.Release()
	bt, _ := NewTensor(dev, []int{n, n}, bData)
	defer bt.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, err := MatMul(dev, a, bt)
		if err != nil {
			b.Fatal(err)
		}
		_, err = c.ToCPU()
		if err != nil {
			b.Fatal(err)
		}
		c.Release()
	}

	elapsed := b.Elapsed()
	flops := float64(b.N) * 2.0 * float64(n) * float64(n) * float64(n)
	gflops := flops / elapsed.Seconds() / 1e9
	b.ReportMetric(gflops, "GFLOPS")
	fmt.Printf("  %dx%d MatMul: %.2f GFLOPS\n", n, n, gflops)
}
