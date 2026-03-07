// Row-wise Softmax: out[i] = exp(a[i] - max) / sum(exp(a[j] - max))
// Two-pass approach for numerical stability:
//   Pass 1: find row max
//   Pass 2: compute exp and normalize
// Each workgroup processes one row.

@group(0) @binding(0) var<storage, read> input : array<f32>;
@group(0) @binding(1) var<storage, read_write> output : array<f32>;

struct Params {
  rows: u32,
  cols: u32,
}
@group(0) @binding(2) var<uniform> params : Params;

// Shared memory for reductions within a workgroup.
var<workgroup> shared_max : array<f32, 256>;
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

  // Pass 1: Find the max value in this row (parallel reduction).
  var localMax : f32 = -3.402823e+38;  // -FLT_MAX
  var i = tid;
  while (i < cols) {
    let val = input[rowStart + i];
    if (val > localMax) {
      localMax = val;
    }
    i = i + 256u;
  }
  shared_max[tid] = localMax;
  workgroupBarrier();

  // Tree reduction for max.
  var stride = 128u;
  while (stride > 0u) {
    if (tid < stride) {
      if (shared_max[tid + stride] > shared_max[tid]) {
        shared_max[tid] = shared_max[tid + stride];
      }
    }
    workgroupBarrier();
    stride = stride / 2u;
  }

  let rowMax = shared_max[0];
  workgroupBarrier();

  // Pass 2: Compute exp(x - max) and sum.
  var localSum : f32 = 0.0;
  i = tid;
  while (i < cols) {
    let val = exp(input[rowStart + i] - rowMax);
    output[rowStart + i] = val;  // Store exp temporarily.
    localSum = localSum + val;
    i = i + 256u;
  }
  shared_sum[tid] = localSum;
  workgroupBarrier();

  // Tree reduction for sum.
  stride = 128u;
  while (stride > 0u) {
    if (tid < stride) {
      shared_sum[tid] = shared_sum[tid] + shared_sum[tid + stride];
    }
    workgroupBarrier();
    stride = stride / 2u;
  }

  let rowSum = shared_sum[0];
  workgroupBarrier();

  // Pass 3: Normalize.
  let invSum = 1.0 / rowSum;
  i = tid;
  while (i < cols) {
    output[rowStart + i] = output[rowStart + i] * invSum;
    i = i + 256u;
  }
}
