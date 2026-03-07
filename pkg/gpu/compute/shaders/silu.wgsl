// Activation functions: SiLU and GELU
// Op dispatch via params.op:
//   0 = SiLU: x * sigmoid(x) = x / (1 + exp(-x))
//   1 = GELU: 0.5 * x * (1 + tanh(sqrt(2/pi) * (x + 0.044715*x^3)))

@group(0) @binding(0) var<storage, read> input : array<f32>;
@group(0) @binding(1) var<storage, read_write> output : array<f32>;

struct Params {
  len: u32,
  op: u32,  // 0 = SiLU, 1 = GELU
}
@group(0) @binding(2) var<uniform> params : Params;

@compute @workgroup_size(256)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let idx = gid.x;
  if (idx >= params.len) { return; }

  let x = input[idx];

  switch (params.op) {
    case 0u: {
      // SiLU: x * sigmoid(x)
      let sigmoid_x = 1.0 / (1.0 + exp(-x));
      output[idx] = x * sigmoid_x;
    }
    case 1u: {
      // GELU (tanh approximation)
      let c = 0.7978845608; // sqrt(2/pi)
      let x3 = x * x * x;
      let inner = c * (x + 0.044715 * x3);
      output[idx] = 0.5 * x * (1.0 + tanh(inner));
    }
    default: {
      output[idx] = x;
    }
  }
}
