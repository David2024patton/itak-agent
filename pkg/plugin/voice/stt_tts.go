package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ── Deepgram STT ───────────────────────────────────────────────────

// DeepgramSTT uses the Deepgram Nova API for speech-to-text.
//
// Batch:     POST https://api.deepgram.com/v1/listen?model=nova-3
// Streaming: wss://api.deepgram.com/v1/listen (WebSocket, send raw audio frames)
// Auth:      Authorization: Token <API_KEY>
type DeepgramSTT struct {
	apiKey string
	model  string
}

func NewDeepgramSTT(cfg map[string]string) *DeepgramSTT {
	model := cfg["model"]
	if model == "" {
		model = "nova-3"
	}
	return &DeepgramSTT{
		apiKey: cfg["api_key"],
		model:  model,
	}
}

func (d *DeepgramSTT) Name() string { return "deepgram" }

func (d *DeepgramSTT) TranscribeAudio(ctx context.Context, audio []byte, format string) (string, error) {
	contentType := "audio/wav"
	switch format {
	case "mp3":
		contentType = "audio/mp3"
	case "ogg":
		contentType = "audio/ogg"
	case "flac":
		contentType = "audio/flac"
	case "mulaw", "ulaw":
		contentType = "audio/mulaw"
	case "pcm":
		contentType = "audio/l16"
	}

	reqURL := fmt.Sprintf("https://api.deepgram.com/v1/listen?model=%s&punctuate=true&smart_format=true", d.model)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(audio))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+d.apiKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("deepgram: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("deepgram: %d %s", resp.StatusCode, string(b))
	}

	var result struct {
		Results struct {
			Channels []struct {
				Alternatives []struct {
					Transcript string  `json:"transcript"`
					Confidence float64 `json:"confidence"`
				} `json:"alternatives"`
			} `json:"channels"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("deepgram decode: %w", err)
	}

	if len(result.Results.Channels) > 0 && len(result.Results.Channels[0].Alternatives) > 0 {
		return result.Results.Channels[0].Alternatives[0].Transcript, nil
	}
	return "", nil
}

// ── AssemblyAI STT ─────────────────────────────────────────────────

// AssemblyAISTT uses the AssemblyAI API for speech-to-text.
//
// Batch:     POST https://api.assemblyai.com/v2/transcript (submit URL, poll for result)
// Upload:    POST https://api.assemblyai.com/v2/upload (upload audio, get URL back)
// Streaming: wss://streaming.assemblyai.com (real-time WebSocket)
// EU:        api.eu.assemblyai.com / streaming.eu.assemblyai.com
// Auth:      authorization: <API_KEY> header
type AssemblyAISTT struct {
	apiKey string
	region string // "us" (default) or "eu"
}

func NewAssemblyAISTT(cfg map[string]string) *AssemblyAISTT {
	region := cfg["region"]
	if region == "" {
		region = "us"
	}
	return &AssemblyAISTT{
		apiKey: cfg["api_key"],
		region: region,
	}
}

func (a *AssemblyAISTT) Name() string { return "assemblyai" }

func (a *AssemblyAISTT) baseURL() string {
	if a.region == "eu" {
		return "https://api.eu.assemblyai.com"
	}
	return "https://api.assemblyai.com"
}

func (a *AssemblyAISTT) TranscribeAudio(ctx context.Context, audio []byte, format string) (string, error) {
	// Step 1: Upload audio.
	uploadURL := a.baseURL() + "/v2/upload"
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewReader(audio))
	if err != nil {
		return "", err
	}
	req.Header.Set("authorization", a.apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("assemblyai upload: %w", err)
	}
	defer resp.Body.Close()

	var uploadResult struct {
		UploadURL string `json:"upload_url"`
	}
	json.NewDecoder(resp.Body).Decode(&uploadResult)

	if uploadResult.UploadURL == "" {
		return "", fmt.Errorf("assemblyai: no upload_url returned")
	}

	// Step 2: Create transcript job.
	transcriptURL := a.baseURL() + "/v2/transcript"
	payload := map[string]interface{}{
		"audio_url": uploadResult.UploadURL,
	}
	b, _ := json.Marshal(payload)

	req2, err := http.NewRequestWithContext(ctx, "POST", transcriptURL, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req2.Header.Set("authorization", a.apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return "", fmt.Errorf("assemblyai transcript: %w", err)
	}
	defer resp2.Body.Close()

	var job struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Text   string `json:"text"`
	}
	json.NewDecoder(resp2.Body).Decode(&job)

	// Step 3: Poll for completion (simple polling).
	for job.Status != "completed" && job.Status != "error" {
		pollURL := fmt.Sprintf("%s/v2/transcript/%s", a.baseURL(), job.ID)
		req3, err := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
		if err != nil {
			return "", err
		}
		req3.Header.Set("authorization", a.apiKey)

		resp3, err := http.DefaultClient.Do(req3)
		if err != nil {
			return "", err
		}
		json.NewDecoder(resp3.Body).Decode(&job)
		resp3.Body.Close()

		if job.Status == "error" {
			return "", fmt.Errorf("assemblyai: transcription failed")
		}
	}

	return job.Text, nil
}

// ── Whisper STT (local) ────────────────────────────────────────────

// WhisperSTT uses a locally hosted Whisper-compatible API for STT.
// Compatible with OpenAI Whisper API format at /v1/audio/transcriptions.
type WhisperSTT struct {
	endpoint string
}

func NewWhisperSTT(cfg map[string]string) *WhisperSTT {
	endpoint := cfg["endpoint"]
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	return &WhisperSTT{endpoint: endpoint}
}

func (w *WhisperSTT) Name() string { return "whisper" }

func (w *WhisperSTT) TranscribeAudio(ctx context.Context, audio []byte, format string) (string, error) {
	// Build multipart form with the audio file.
	boundary := "---whisper-boundary---"
	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	body.WriteString(fmt.Sprintf("Content-Disposition: form-data; name=\"file\"; filename=\"audio.%s\"\r\n", format))
	body.WriteString(fmt.Sprintf("Content-Type: audio/%s\r\n\r\n", format))
	body.Write(audio)
	body.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
	body.WriteString("Content-Disposition: form-data; name=\"model\"\r\n\r\n")
	body.WriteString("whisper-1")
	body.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))

	reqURL := strings.TrimRight(w.endpoint, "/") + "/v1/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("whisper: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Text string `json:"text"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Text, nil
}

// ── ElevenLabs TTS ─────────────────────────────────────────────────

// ElevenLabsTTS uses the ElevenLabs API for high-quality text-to-speech.
//
// Endpoint:  POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}
// Streaming: POST https://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream
// Auth:      xi-api-key: <API_KEY>
// Formats:   mp3_44100_128, ulaw_8000 (for Twilio), pcm_16000
type ElevenLabsTTS struct {
	apiKey   string
	voiceID  string
	modelID  string
}

func NewElevenLabsTTS(cfg map[string]string) *ElevenLabsTTS {
	voiceID := cfg["voice_id"]
	if voiceID == "" {
		voiceID = "21m00Tcm4TlvDq8ikWAM" // Rachel (default)
	}
	modelID := cfg["model_id"]
	if modelID == "" {
		modelID = "eleven_multilingual_v2"
	}
	return &ElevenLabsTTS{
		apiKey:  cfg["api_key"],
		voiceID: voiceID,
		modelID: modelID,
	}
}

func (e *ElevenLabsTTS) Name() string { return "elevenlabs" }

func (e *ElevenLabsTTS) Synthesize(ctx context.Context, text string, format string) ([]byte, error) {
	// Map desired format to ElevenLabs output_format query param.
	outputFormat := "mp3_44100_128"
	switch format {
	case "ulaw", "mulaw":
		outputFormat = "ulaw_8000" // Optimized for Twilio
	case "pcm":
		outputFormat = "pcm_16000"
	}

	reqURL := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s?output_format=%s",
		e.voiceID, outputFormat)

	payload := map[string]interface{}{
		"text":     text,
		"model_id": e.modelID,
		"voice_settings": map[string]interface{}{
			"stability":        0.5,
			"similarity_boost": 0.5,
		},
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("xi-api-key", e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elevenlabs: %d %s", resp.StatusCode, string(errBody))
	}

	return io.ReadAll(resp.Body)
}

// ── Local TTS ──────────────────────────────────────────────────────

// LocalTTS uses a locally hosted TTS server (Qwen-TTS, Piper, etc.)
// Compatible with OpenAI TTS API format at /v1/audio/speech.
type LocalTTS struct {
	endpoint string
	voice    string
}

func NewLocalTTS(cfg map[string]string) *LocalTTS {
	endpoint := cfg["endpoint"]
	if endpoint == "" {
		endpoint = "http://localhost:8001"
	}
	voice := cfg["voice"]
	if voice == "" {
		voice = "alloy"
	}
	return &LocalTTS{endpoint: endpoint, voice: voice}
}

func (l *LocalTTS) Name() string { return "local" }

func (l *LocalTTS) Synthesize(ctx context.Context, text string, format string) ([]byte, error) {
	respFormat := "mp3"
	if format != "" {
		respFormat = format
	}

	payload := map[string]interface{}{
		"model":           "tts-1",
		"input":           text,
		"voice":           l.voice,
		"response_format": respFormat,
	}
	b, _ := json.Marshal(payload)

	reqURL := strings.TrimRight(l.endpoint, "/") + "/v1/audio/speech"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local tts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("local tts: %d %s", resp.StatusCode, string(errBody))
	}

	return io.ReadAll(resp.Body)
}
