// Matrix multiplication: C = A * B
// Workgroup size: 16x16
// Each thread computes one element of C.

@group(0) @binding(0) var<storage, read> a : array<f32>;
@group(0) @binding(1) var<storage, read> b : array<f32>;
@group(0) @binding(2) var<storage, read_write> c : array<f32>;

struct Params {
  m: u32,  // rows of A and C
  n: u32,  // cols of B and C
  k: u32,  // cols of A / rows of B
}
@group(0) @binding(3) var<uniform> params : Params;

@compute @workgroup_size(16, 16)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let row = gid.x;
  let col = gid.y;
  let m = params.m;
  let n = params.n;
  let k = params.k;

  if (row >= m || col >= n) { return; }

  var sum : f32 = 0.0;
  for (var i : u32 = 0u; i < k; i = i + 1u) {
    sum = sum + a[row * k + i] * b[i * n + col];
  }
  c[row * n + col] = sum;
}
