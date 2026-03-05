package llama

import (
	"unsafe"

	"github.com/David2024patton/GOAgent/pkg/torch/utils"
	"github.com/jupiterrider/ffi"
)

// MtmdContext is an opaque handle to a multi-modal context.
type MtmdContext uintptr

// MtmdBitmap is an opaque handle to an image/audio bitmap.
type MtmdBitmap uintptr

// MtmdImageTokens is an opaque handle to image tokens.
type MtmdImageTokens uintptr

// MtmdInputChunk is an opaque handle to an input chunk.
type MtmdInputChunk uintptr

// MtmdInputChunks is an opaque handle to a collection of input chunks.
type MtmdInputChunks uintptr

// MtmdInputChunkType describes whether a chunk is text, image, or audio.
type MtmdInputChunkType int32

const (
	MtmdInputChunkTypeText  MtmdInputChunkType = 0
	MtmdInputChunkTypeImage MtmdInputChunkType = 1
	MtmdInputChunkTypeAudio MtmdInputChunkType = 2
)

// MtmdInputText matches the C struct mtmd_input_text.
type MtmdInputText struct {
	Text         *byte
	AddSpecial   uint8
	ParseSpecial uint8
}

// MtmdContextParams matches the C struct mtmd_context_params.
type MtmdContextParams struct {
	UseGPU         uint8
	PrintTimings   uint8
	NThreads       int32
	ImageMarker    uintptr // deprecated, use MediaMarker
	MediaMarker    uintptr
	FlashAttnType  int32
	Warmup         uint8
	ImageMinTokens int32
	ImageMaxTokens int32
	CbEval         uintptr
	CbEvalUserData uintptr
}

var (
	// ffiTypeMtmdContextParams represents the C struct mtmd_context_params
	ffiTypeMtmdContextParams = ffi.NewType(
		&ffi.TypeUint8,   // use_gpu
		&ffi.TypeUint8,   // print_timings
		&ffi.TypeSint32,  // n_threads
		&ffi.TypePointer, // image_marker
		&ffi.TypePointer, // media_marker
		&ffi.TypeSint32,  // flash_attn_type
		&ffi.TypeUint8,   // warmup
		&ffi.TypeSint32,  // image_min_tokens
		&ffi.TypeSint32,  // image_max_tokens
		&ffi.TypePointer, // cb_eval
		&ffi.TypePointer, // cb_eval_user_data
	)

	// ffiTypeMtmdInputText represents the C struct mtmd_input_text
	ffiTypeMtmdInputText = ffi.NewType(
		&ffi.TypePointer, // text
		&ffi.TypeUint8,   // add_special
		&ffi.TypeUint8,   // parse_special
	)
)

var (
	// MTMD_API const char * mtmd_default_marker(void);
	mtmdDefaultMarkerFunc ffi.Fun

	// MTMD_API struct mtmd_context_params mtmd_context_params_default(void);
	mtmdContextParamsDefaultFunc ffi.Fun

	// MTMD_API mtmd_context * mtmd_init_from_file(const char * mmproj_fname,
	//                                            const struct llama_model * text_model,
	//                                            const struct mtmd_context_params ctx_params);
	mtmdInitFromFileFunc ffi.Fun

	// MTMD_API void mtmd_free(mtmd_context * ctx);
	mtmdFreeFunc ffi.Fun

	// MTMD_API bool mtmd_support_vision(mtmd_context * ctx);
	mtmdSupportVisionFunc ffi.Fun

	// MTMD_API bool mtmd_support_audio(mtmd_context * ctx);
	mtmdSupportAudioFunc ffi.Fun

	// MTMD_API bool mtmd_decode_use_non_causal(mtmd_context * ctx);
	mtmdDecodeUseNonCausalFunc ffi.Fun

	// MTMD_API bool mtmd_decode_use_mrope(mtmd_context * ctx);
	mtmdDecodeUseMropeFunc ffi.Fun

	// MTMD_API mtmd_bitmap * mtmd_bitmap_init(uint32_t nx, uint32_t ny, const unsigned char * data);
	mtmdBitmapInitFunc ffi.Fun

	// MTMD_API void mtmd_bitmap_free(mtmd_bitmap * bitmap);
	mtmdBitmapFreeFunc ffi.Fun

	// MTMD_API uint32_t mtmd_bitmap_get_nx(const mtmd_bitmap * bitmap);
	mtmdBitmapGetNxFunc ffi.Fun

	// MTMD_API uint32_t mtmd_bitmap_get_ny(const mtmd_bitmap * bitmap);
	mtmdBitmapGetNyFunc ffi.Fun

	// MTMD_API mtmd_input_chunks * mtmd_input_chunks_init(void);
	mtmdInputChunksInitFunc ffi.Fun

	// MTMD_API size_t mtmd_input_chunks_size(const mtmd_input_chunks * chunks);
	mtmdInputChunksSizeFunc ffi.Fun

	// MTMD_API const mtmd_input_chunk * mtmd_input_chunks_get(const mtmd_input_chunks * chunks, size_t idx);
	mtmdInputChunksGetFunc ffi.Fun

	// MTMD_API void mtmd_input_chunks_free(mtmd_input_chunks * chunks);
	mtmdInputChunksFreeFunc ffi.Fun

	// MTMD_API enum mtmd_input_chunk_type mtmd_input_chunk_get_type(const mtmd_input_chunk * chunk);
	mtmdInputChunkGetTypeFunc ffi.Fun

	// MTMD_API const llama_token * mtmd_input_chunk_get_tokens_text(const mtmd_input_chunk * chunk, size_t * n_tokens_output);
	mtmdInputChunkGetTokensTextFunc ffi.Fun

	// MTMD_API size_t mtmd_input_chunk_get_n_tokens(const mtmd_input_chunk * chunk);
	mtmdInputChunkGetNTokensFunc ffi.Fun

	// MTMD_API int32_t mtmd_tokenize(mtmd_context * ctx,
	//                                mtmd_input_chunks * output,
	//                                const mtmd_input_text * text,
	//                                const mtmd_bitmap ** bitmaps,
	//                                size_t n_bitmaps);
	mtmdTokenizeFunc ffi.Fun

	// MTMD_API int32_t mtmd_encode_chunk(mtmd_context * ctx,
	//                                    const mtmd_input_chunk * chunk);
	mtmdEncodeChunkFunc ffi.Fun

	// MTMD_API float * mtmd_get_output_embd(mtmd_context * ctx);
	mtmdGetOutputEmbdFunc ffi.Fun
)

// mtmdAvailable tracks whether mtmd.dll was loaded successfully.
var mtmdAvailable bool

// MtmdAvailable returns whether multi-modal support (mtmd.dll) is loaded.
func MtmdAvailable() bool {
	return mtmdAvailable
}

func loadMtmdFuncs(lib ffi.Lib) error {
	var err error

	if mtmdDefaultMarkerFunc, err = lib.Prep("mtmd_default_marker", &ffi.TypePointer); err != nil {
		return loadError("mtmd_default_marker", err)
	}

	if mtmdContextParamsDefaultFunc, err = lib.Prep("mtmd_context_params_default", &ffiTypeMtmdContextParams); err != nil {
		return loadError("mtmd_context_params_default", err)
	}

	if mtmdInitFromFileFunc, err = lib.Prep("mtmd_init_from_file", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffiTypeMtmdContextParams); err != nil {
		return loadError("mtmd_init_from_file", err)
	}

	if mtmdFreeFunc, err = lib.Prep("mtmd_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_free", err)
	}

	if mtmdSupportVisionFunc, err = lib.Prep("mtmd_support_vision", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("mtmd_support_vision", err)
	}

	if mtmdSupportAudioFunc, err = lib.Prep("mtmd_support_audio", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("mtmd_support_audio", err)
	}

	if mtmdDecodeUseNonCausalFunc, err = lib.Prep("mtmd_decode_use_non_causal", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("mtmd_decode_use_non_causal", err)
	}

	if mtmdDecodeUseMropeFunc, err = lib.Prep("mtmd_decode_use_mrope", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("mtmd_decode_use_mrope", err)
	}

	if mtmdBitmapInitFunc, err = lib.Prep("mtmd_bitmap_init", &ffi.TypePointer, &ffi.TypeUint32, &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_init", err)
	}

	if mtmdBitmapFreeFunc, err = lib.Prep("mtmd_bitmap_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_free", err)
	}

	if mtmdBitmapGetNxFunc, err = lib.Prep("mtmd_bitmap_get_nx", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_get_nx", err)
	}

	if mtmdBitmapGetNyFunc, err = lib.Prep("mtmd_bitmap_get_ny", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_get_ny", err)
	}

	if mtmdInputChunksInitFunc, err = lib.Prep("mtmd_input_chunks_init", &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunks_init", err)
	}

	if mtmdInputChunksSizeFunc, err = lib.Prep("mtmd_input_chunks_size", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunks_size", err)
	}

	if mtmdInputChunksGetFunc, err = lib.Prep("mtmd_input_chunks_get", &ffi.TypePointer, &ffi.TypePointer, &ffiTypeSize); err != nil {
		return loadError("mtmd_input_chunks_get", err)
	}

	if mtmdInputChunksFreeFunc, err = lib.Prep("mtmd_input_chunks_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunks_free", err)
	}

	if mtmdInputChunkGetTypeFunc, err = lib.Prep("mtmd_input_chunk_get_type", &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_type", err)
	}

	if mtmdInputChunkGetTokensTextFunc, err = lib.Prep("mtmd_input_chunk_get_tokens_text", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_tokens_text", err)
	}

	if mtmdInputChunkGetNTokensFunc, err = lib.Prep("mtmd_input_chunk_get_n_tokens", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_input_chunk_get_n_tokens", err)
	}

	if mtmdTokenizeFunc, err = lib.Prep("mtmd_tokenize", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffiTypeSize); err != nil {
		return loadError("mtmd_tokenize", err)
	}

	if mtmdEncodeChunkFunc, err = lib.Prep("mtmd_encode_chunk", &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_encode_chunk", err)
	}

	if mtmdGetOutputEmbdFunc, err = lib.Prep("mtmd_get_output_embd", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_get_output_embd", err)
	}

	mtmdAvailable = true
	return nil
}

// -----------------------------------------------------------------------
// Go wrappers for mtmd C API
// -----------------------------------------------------------------------

// MtmdDefaultMarker returns the default media marker string ("<__media__>").
func MtmdDefaultMarker() string {
	var ptr *byte
	mtmdDefaultMarkerFunc.Call(unsafe.Pointer(&ptr))
	if ptr == nil {
		return "<__media__>"
	}
	return utils.BytePtrToString(ptr)
}

// MtmdContextParamsDefault returns default parameters for multi-modal context.
func MtmdContextParamsDefault() MtmdContextParams {
	var p MtmdContextParams
	mtmdContextParamsDefaultFunc.Call(unsafe.Pointer(&p))
	return p
}

// MtmdInitFromFile creates a multi-modal context from an mmproj GGUF file
// and an already-loaded text model.
func MtmdInitFromFile(mmprojPath string, textModel Model, params MtmdContextParams) (MtmdContext, error) {
	var ctx MtmdContext
	file := &[]byte(mmprojPath + "\x00")[0]
	mtmdInitFromFileFunc.Call(unsafe.Pointer(&ctx), unsafe.Pointer(&file), unsafe.Pointer(&textModel), unsafe.Pointer(&params))
	if ctx == 0 {
		return 0, loadError("mtmd_init_from_file", nil)
	}
	return ctx, nil
}

// MtmdFree releases multi-modal context resources.
func MtmdFree(ctx MtmdContext) {
	if ctx == 0 {
		return
	}
	mtmdFreeFunc.Call(nil, unsafe.Pointer(&ctx))
}

// MtmdSupportVision returns true if the context supports vision input.
func MtmdSupportVision(ctx MtmdContext) bool {
	if ctx == 0 {
		return false
	}
	var result ffi.Arg
	mtmdSupportVisionFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return result.Bool()
}

// MtmdSupportAudio returns true if the context supports audio input.
func MtmdSupportAudio(ctx MtmdContext) bool {
	if ctx == 0 {
		return false
	}
	var result ffi.Arg
	mtmdSupportAudioFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return result.Bool()
}

// MtmdDecodeUseNonCausal returns whether non-causal mask is needed before decode.
func MtmdDecodeUseNonCausal(ctx MtmdContext) bool {
	if ctx == 0 {
		return false
	}
	var result ffi.Arg
	mtmdDecodeUseNonCausalFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return result.Bool()
}

// MtmdDecodeUseMrope returns whether the model uses M-RoPE.
func MtmdDecodeUseMrope(ctx MtmdContext) bool {
	if ctx == 0 {
		return false
	}
	var result ffi.Arg
	mtmdDecodeUseMropeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return result.Bool()
}

// MtmdBitmapInit creates a bitmap from raw RGB pixel data.
// data must be nx * ny * 3 bytes (RGBRGBRGB...).
func MtmdBitmapInit(nx, ny uint32, data []byte) MtmdBitmap {
	var bitmap MtmdBitmap
	dataPtr := unsafe.Pointer(&data[0])
	mtmdBitmapInitFunc.Call(unsafe.Pointer(&bitmap), &nx, &ny, unsafe.Pointer(&dataPtr))
	return bitmap
}

// MtmdBitmapFree releases bitmap resources.
func MtmdBitmapFree(bitmap MtmdBitmap) {
	if bitmap == 0 {
		return
	}
	mtmdBitmapFreeFunc.Call(nil, unsafe.Pointer(&bitmap))
}

// MtmdBitmapGetNx returns the width of the bitmap.
func MtmdBitmapGetNx(bitmap MtmdBitmap) uint32 {
	if bitmap == 0 {
		return 0
	}
	var nx ffi.Arg
	mtmdBitmapGetNxFunc.Call(unsafe.Pointer(&nx), unsafe.Pointer(&bitmap))
	return uint32(nx)
}

// MtmdBitmapGetNy returns the height of the bitmap.
func MtmdBitmapGetNy(bitmap MtmdBitmap) uint32 {
	if bitmap == 0 {
		return 0
	}
	var ny ffi.Arg
	mtmdBitmapGetNyFunc.Call(unsafe.Pointer(&ny), unsafe.Pointer(&bitmap))
	return uint32(ny)
}

// MtmdInputChunksInit creates a new empty input chunks container.
func MtmdInputChunksInit() MtmdInputChunks {
	var chunks MtmdInputChunks
	mtmdInputChunksInitFunc.Call(unsafe.Pointer(&chunks))
	return chunks
}

// MtmdInputChunksSize returns the number of chunks in the container.
func MtmdInputChunksSize(chunks MtmdInputChunks) uint64 {
	if chunks == 0 {
		return 0
	}
	var size ffi.Arg
	mtmdInputChunksSizeFunc.Call(unsafe.Pointer(&size), unsafe.Pointer(&chunks))
	return uint64(size)
}

// MtmdInputChunksGet returns the chunk at the given index.
func MtmdInputChunksGet(chunks MtmdInputChunks, idx uint64) MtmdInputChunk {
	if chunks == 0 {
		return 0
	}
	var chunk MtmdInputChunk
	mtmdInputChunksGetFunc.Call(unsafe.Pointer(&chunk), unsafe.Pointer(&chunks), &idx)
	return chunk
}

// MtmdInputChunksFree releases all chunks and the container.
func MtmdInputChunksFree(chunks MtmdInputChunks) {
	if chunks == 0 {
		return
	}
	mtmdInputChunksFreeFunc.Call(nil, unsafe.Pointer(&chunks))
}

// MtmdInputChunkGetType returns the type of a chunk (text, image, or audio).
func MtmdInputChunkGetType(chunk MtmdInputChunk) MtmdInputChunkType {
	if chunk == 0 {
		return MtmdInputChunkTypeText
	}
	var result ffi.Arg
	mtmdInputChunkGetTypeFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&chunk))
	return MtmdInputChunkType(int32(result))
}

// MtmdInputChunkGetTokensText returns the text tokens and count for a text chunk.
func MtmdInputChunkGetTokensText(chunk MtmdInputChunk) ([]Token, uint64) {
	if chunk == 0 {
		return nil, 0
	}
	var tokensPtr uintptr
	var nTokens uint64
	mtmdInputChunkGetTokensTextFunc.Call(unsafe.Pointer(&tokensPtr), unsafe.Pointer(&chunk), unsafe.Pointer(&nTokens))

	if tokensPtr == 0 || nTokens == 0 {
		return nil, 0
	}

	tokens := unsafe.Slice((*Token)(unsafe.Pointer(tokensPtr)), nTokens)
	result := make([]Token, nTokens)
	copy(result, tokens)
	return result, nTokens
}

// MtmdInputChunkGetNTokens returns the total token count for a chunk.
func MtmdInputChunkGetNTokens(chunk MtmdInputChunk) uint64 {
	if chunk == 0 {
		return 0
	}
	var n ffi.Arg
	mtmdInputChunkGetNTokensFunc.Call(unsafe.Pointer(&n), unsafe.Pointer(&chunk))
	return uint64(n)
}

// MtmdTokenize tokenizes input text with embedded media markers and bitmaps.
// The text must contain the media marker (default: "<__media__>") for each bitmap.
// Returns 0 on success, 1 if bitmap count doesn't match markers, 2 on preprocessing error.
func MtmdTokenize(ctx MtmdContext, output MtmdInputChunks, text MtmdInputText, bitmaps []MtmdBitmap) int32 {
	if ctx == 0 || output == 0 {
		return -1
	}

	nBitmaps := uint64(len(bitmaps))

	// Build array of pointers to bitmaps (const mtmd_bitmap **)
	var bitmapsPtr unsafe.Pointer
	if nBitmaps > 0 {
		bitmapsPtr = unsafe.Pointer(&bitmaps[0])
	}

	var result ffi.Arg
	mtmdTokenizeFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&output),
		unsafe.Pointer(&text),
		unsafe.Pointer(&bitmapsPtr),
		&nBitmaps,
	)
	return int32(result)
}

// MtmdEncodeChunk encodes a chunk (runs CLIP for image chunks).
// Returns 0 on success.
func MtmdEncodeChunk(ctx MtmdContext, chunk MtmdInputChunk) int32 {
	if ctx == 0 || chunk == 0 {
		return -1
	}
	var result ffi.Arg
	mtmdEncodeChunkFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx), unsafe.Pointer(&chunk))
	return int32(result)
}

// MtmdGetOutputEmbd returns the output embeddings from the last encode pass.
func MtmdGetOutputEmbd(ctx MtmdContext) uintptr {
	if ctx == 0 {
		return 0
	}
	var ptr uintptr
	mtmdGetOutputEmbdFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&ctx))
	return ptr
}
