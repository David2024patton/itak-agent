// Elementwise operations: Add, Mul, Scale
// Each thread processes one element.

@group(0) @binding(0) var<storage, read> a : array<f32>;
@group(0) @binding(1) var<storage, read> b : array<f32>;
@group(0) @binding(2) var<storage, read_write> out : array<f32>;

struct Params {
  len: u32,
  op: u32,    // 0 = add, 1 = mul, 2 = scale (b[0] is scalar)
}
@group(0) @binding(3) var<uniform> params : Params;

@compute @workgroup_size(256)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let idx = gid.x;
  if (idx >= params.len) { return; }

  switch (params.op) {
    case 0u: {
      // Add
      out[idx] = a[idx] + b[idx];
    }
    case 1u: {
      // Multiply
      out[idx] = a[idx] * b[idx];
    }
    case 2u: {
      // Scale: multiply all elements of a by b[0]
      out[idx] = a[idx] * b[0];
    }
    default: {
      out[idx] = a[idx];
    }
  }
}
