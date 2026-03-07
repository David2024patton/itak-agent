# Changelog

All notable changes to gputypes will be documented in this file.

## [v0.2.0] - 2026-01-29

### Changed

- **webgpu.h compliance**: All enum values now use explicit hex constants matching the official WebGPU C header specification
  - TextureFormat: 97 formats with spec-compliant values (0x00000000 - 0x00000060)
  - BufferUsage: Explicit bit flags matching WebGPU spec
  - TextureUsage: Explicit bit flags matching WebGPU spec
  - LoadOp, StoreOp: Spec-compliant values
  - BlendFactor, BlendOperation: Spec-compliant values
  - AddressMode, FilterMode: Spec-compliant values
  - VertexFormat: 31 formats with spec-compliant values
  - PresentMode, CompositeAlphaMode: Spec-compliant values

### Migration

Values changed from iota-based to explicit webgpu.h values. This ensures:
- Binary compatibility with wgpu-native and other WebGPU implementations
- Correct serialization/deserialization of GPU descriptors
- Interoperability with C/Rust WebGPU code

If you were relying on specific numeric values, they may have changed. Use the named constants.

## [v0.1.0] - 2026-01-29

### Added

- Initial release with WebGPU type definitions
- **Texture types**: TextureFormat (97 formats including BC, ETC2, ASTC), TextureUsage, TextureDimension, TextureViewDimension, TextureAspect, TextureSampleType
- **Buffer types**: BufferUsage, BufferBindingType, BufferDescriptor, IndexFormat
- **Sampler types**: AddressMode, FilterMode, MipmapFilterMode, CompareFunction, SamplerDescriptor
- **Render types**: LoadOp, StoreOp, BlendState, BlendFactor, BlendOperation, ColorWriteMask, RenderPassDescriptor
- **Shader types**: ShaderStage, ShaderSource (WGSL, SPIR-V, GLSL)
- **Vertex types**: VertexFormat (31 formats), VertexStepMode, VertexBufferLayout
- **Binding types**: BindGroupLayoutEntry, BindGroupEntry, BufferBindingLayout, TextureBindingLayout
- **Adapter types**: DeviceType, Backend, PowerPreference, AdapterInfo
- **Surface types**: PresentMode, CompositeAlphaMode, SurfaceConfiguration
- **Limits & Features**: Full WebGPU limits struct, feature flags
- **Geometry types**: Extent3D, Origin3D, Color
