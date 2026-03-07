// Rotary Position Embedding (RoPE)
// Applies sin/cos rotation to pairs of elements in each position.
// Input shape: [seq_len, head_dim] (head_dim must be even)
// Each thread handles one (position, pair) rotation.
//
// For position p and dimension pair d (0-indexed):
//   freq = 1.0 / (theta ^ (2*d / head_dim))
//   angle = p * freq
//   out[p, 2*d]   = x[p, 2*d]   * cos(angle) - x[p, 2*d+1] * sin(angle)
//   out[p, 2*d+1] = x[p, 2*d]   * sin(angle) + x[p, 2*d+1] * cos(angle)

@group(0) @binding(0) var<storage, read> input : array<f32>;
@group(0) @binding(1) var<storage, read_write> output : array<f32>;

struct Params {
  seq_len: u32,
  head_dim: u32,
  theta: f32,
}
@group(0) @binding(2) var<uniform> params : Params;

@compute @workgroup_size(256)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let idx = gid.x;
  let half_dim = params.head_dim / 2u;
  let total_pairs = params.seq_len * half_dim;

  if (idx >= total_pairs) { return; }

  // Decode position and pair index from flat thread index.
  let pos = idx / half_dim;
  let d = idx % half_dim;

  // Compute rotation angle.
  let freq_exp = 2.0 * f32(d) / f32(params.head_dim);
  let freq = 1.0 / pow(params.theta, freq_exp);
  let angle = f32(pos) * freq;

  let cos_a = cos(angle);
  let sin_a = sin(angle);

  // Read the pair of values.
  let base = pos * params.head_dim + d * 2u;
  let x0 = input[base];
  let x1 = input[base + 1u];

  // Apply rotation.
  output[base]      = x0 * cos_a - x1 * sin_a;
  output[base + 1u] = x0 * sin_a + x1 * cos_a;
}
