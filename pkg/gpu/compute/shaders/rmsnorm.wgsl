// RMS Normalization: out[i] = (x[i] / rms) * weight[i]
// where rms = sqrt(mean(x^2) + eps)
// Each workgroup processes one row.

@group(0) @binding(0) var<storage, read> input : array<f32>;
@group(0) @binding(1) var<storage, read> weight : array<f32>;
@group(0) @binding(2) var<storage, read_write> output : array<f32>;

struct Params {
  rows: u32,
  cols: u32,
  eps: f32,
}
@group(0) @binding(3) var<uniform> params : Params;

var<workgroup> shared_sum : array<f32, 256>;

@compute @workgroup_size(256)
fn main(@builtin(global_invocation_id) gid : vec3<u32>,
        @builtin(local_invocation_id) lid : vec3<u32>,
        @builtin(workgroup_id) wid : vec3<u32>) {
  let row = wid.x;
  let tid = lid.x;
  let cols = params.cols;

  if (row >= params.rows) { return; }

  let rowStart = row * cols;

  // Compute sum of squares for this row.
  var localSumSq : f32 = 0.0;
  var i = tid;
  while (i < cols) {
    let val = input[rowStart + i];
    localSumSq = localSumSq + val * val;
    i = i + 256u;
  }
  shared_sum[tid] = localSumSq;
  workgroupBarrier();

  // Tree reduction for sum of squares.
  var stride = 128u;
  while (stride > 0u) {
    if (tid < stride) {
      shared_sum[tid] = shared_sum[tid] + shared_sum[tid + stride];
    }
    workgroupBarrier();
    stride = stride / 2u;
  }

  let rms = sqrt(shared_sum[0] / f32(cols) + params.eps);
  let invRms = 1.0 / rms;
  workgroupBarrier();

  // Normalize and apply weight.
  i = tid;
  while (i < cols) {
    output[rowStart + i] = input[rowStart + i] * invRms * weight[i];
    i = i + 256u;
  }
}
