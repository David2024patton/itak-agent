// Linear layer: out = x @ weight^T + bias
// Fused matmul with weight transpose and bias addition in a single kernel.
// x: [M, K], weight: [N, K] (standard PyTorch layout), bias: [N] or unused
// output: [M, N]
//
// Each thread computes one element of the output matrix.
// Weight transpose is implicit: weight[j, k] is accessed as weight[j * K + k].

@group(0) @binding(0) var<storage, read> x : array<f32>;
@group(0) @binding(1) var<storage, read> weight : array<f32>;
@group(0) @binding(2) var<storage, read> bias : array<f32>;
@group(0) @binding(3) var<storage, read_write> output : array<f32>;

struct Params {
  M: u32,       // batch size (rows of x)
  N: u32,       // out_features (rows of weight)
  K: u32,       // in_features (cols of x, cols of weight)
  has_bias: u32, // 1 if bias should be added, 0 otherwise
}
@group(0) @binding(4) var<uniform> params : Params;

@compute @workgroup_size(16, 16)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let row = gid.y;  // batch index
  let col = gid.x;  // output feature index

  if (row >= params.M || col >= params.N) { return; }

  // Compute dot product: x[row, :] dot weight[col, :]
  // This is equivalent to (x @ weight^T)[row, col]
  var sum : f32 = 0.0;
  for (var k = 0u; k < params.K; k = k + 1u) {
    sum = sum + x[row * params.K + k] * weight[col * params.K + k];
  }

  // Add bias if present.
  if (params.has_bias == 1u) {
    sum = sum + bias[col];
  }

  output[row * params.N + col] = sum;
}
