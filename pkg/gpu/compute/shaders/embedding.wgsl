// Token Embedding Lookup
// For each token index, copies one row from the embedding weight table.
// token_ids: array<u32> of length num_tokens
// weights: array<f32> of shape [vocab_size, embed_dim] (row-major)
// output: array<f32> of shape [num_tokens, embed_dim]
//
// Each thread copies one element: output[token * embed_dim + col] = weights[token_ids[token] * embed_dim + col]

@group(0) @binding(0) var<storage, read> token_ids : array<u32>;
@group(0) @binding(1) var<storage, read> weights : array<f32>;
@group(0) @binding(2) var<storage, read_write> output : array<f32>;

struct Params {
  num_tokens: u32,
  embed_dim: u32,
}
@group(0) @binding(3) var<uniform> params : Params;

@compute @workgroup_size(256)
fn main(@builtin(global_invocation_id) gid : vec3<u32>) {
  let idx = gid.x;
  let total = params.num_tokens * params.embed_dim;

  if (idx >= total) { return; }

  // Decode which token and which dimension.
  let token = idx / params.embed_dim;
  let col = idx % params.embed_dim;

  // Look up the token ID and index into the weight table.
  let token_id = token_ids[token];
  output[idx] = weights[token_id * params.embed_dim + col];
}
