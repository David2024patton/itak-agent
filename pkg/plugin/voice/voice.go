package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/plugin"
)

// VoicePlugin handles incoming phone calls via telephony webhooks,
// routes caller speech through the iTaK orchestrator, and responds
// with TTS audio. Supports both IVR menu and conversational modes.
type VoicePlugin struct {
	cfg       Config
	handler   plugin.MessageHandler
	telephony TelephonyAdapter
	stt       STTAdapter
	tts       TTSAdapter
	server    *http.Server
	calls     map[string]*CallState // active calls by callID
	mu        sync.RWMutex
}

// New creates a voice plugin with the given config and message handler.
func New(cfg Config, handler plugin.MessageHandler) *VoicePlugin {
	if cfg.Port == 0 {
		cfg.Port = 47300
	}
	if cfg.GreetingMessage == "" {
		cfg.GreetingMessage = "Hello, how can I help you today?"
	}

	vp := &VoicePlugin{
		cfg:     cfg,
		handler: handler,
		calls:   make(map[string]*CallState),
	}

	// Initialize telephony adapter based on provider selection.
	vp.telephony = newTelephonyAdapter(cfg.TelephonyProvider, cfg.TelephonyConfig)
	vp.stt = newSTTAdapter(cfg.STTProvider, cfg.STTConfig)
	vp.tts = newTTSAdapter(cfg.TTSProvider, cfg.TTSConfig)

	return vp
}

func (v *VoicePlugin) Name() string { return "voice" }

func (v *VoicePlugin) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/voice/webhook", v.handleWebhook)
	mux.HandleFunc("/voice/status", v.handleStatus)
	mux.HandleFunc("/voice/calls", v.handleCallList)

	v.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", v.cfg.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("[voice] Voice agent listening on port %d (provider: %s, stt: %s, tts: %s)",
			v.cfg.Port, v.cfg.TelephonyProvider, v.cfg.STTProvider, v.cfg.TTSProvider)
		if err := v.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[voice] Server error: %v", err)
		}
	}()

	return nil
}

func (v *VoicePlugin) Stop() error {
	if v.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return v.server.Shutdown(ctx)
	}
	return nil
}

// handleWebhook processes incoming telephony webhooks from any provider.
func (v *VoicePlugin) handleWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse query params and form values into a flat map.
	r.ParseForm()
	params := make(map[string]string)
	for k, vals := range r.Form {
		if len(vals) > 0 {
			params[k] = vals[0]
		}
	}

	if v.telephony == nil {
		http.Error(w, "no telephony provider configured", http.StatusServiceUnavailable)
		return
	}

	event, responseBody, err := v.telephony.HandleWebhook(r.Context(), body, params)
	if err != nil {
		log.Printf("[voice] Webhook error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Process the call event.
	v.processCallEvent(r.Context(), event)

	// If the adapter produced a direct response (TwiML, NCCO), send it.
	if responseBody != nil {
		// Determine content type based on provider.
		ct := "application/json"
		if v.cfg.TelephonyProvider == "twilio" || v.cfg.TelephonyProvider == "plivo" || v.cfg.TelephonyProvider == "bandwidth" {
			ct = "application/xml"
		}
		w.Header().Set("Content-Type", ct)
		w.Write(responseBody)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// processCallEvent handles call lifecycle events.
func (v *VoicePlugin) processCallEvent(ctx context.Context, event *CallEvent) {
	if event == nil {
		return
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	switch event.Type {
	case "initiated", "answered":
		// Track new call and greet the caller.
		state := &CallState{
			CallID:       event.CallID,
			CallerNumber: event.From,
			CalledNumber: event.To,
			Provider:     v.cfg.TelephonyProvider,
			Status:       event.Type,
			Metadata:     make(map[string]string),
		}
		v.calls[event.CallID] = state

		if event.Type == "answered" {
			// Greet the caller.
			go v.greetCaller(ctx, event.CallID)
		}

	case "dtmf":
		// Handle IVR menu selection.
		if v.cfg.MenuEnabled {
			go v.handleMenuSelection(ctx, event.CallID, event.Digits)
		}

	case "speech":
		// Handle conversational input.
		if event.SpeechResult != "" {
			go v.handleSpeechInput(ctx, event.CallID, event.SpeechResult)
		}

	case "hangup":
		delete(v.calls, event.CallID)
		log.Printf("[voice] Call %s ended (from: %s)", event.CallID, event.From)
	}
}

// greetCaller plays the greeting message and sets up the next input mode.
func (v *VoicePlugin) greetCaller(ctx context.Context, callID string) {
	greeting := v.cfg.GreetingMessage

	if v.cfg.MenuEnabled && len(v.cfg.MenuOptions) > 0 {
		// Build IVR menu prompt.
		greeting += " Please select from the following options: "
		for _, opt := range v.cfg.MenuOptions {
			greeting += fmt.Sprintf("Press %s for %s. ", opt.Digit, opt.Label)
		}

		if v.telephony != nil {
			v.telephony.Gather(ctx, callID, greeting, GatherOpts{
				InputType: "dtmf",
				NumDigits: 1,
				Timeout:   10,
			})
		}
	} else {
		// Conversational mode: speak greeting, then listen for speech.
		if v.telephony != nil {
			v.telephony.Speak(ctx, callID, greeting)
			v.telephony.Gather(ctx, callID, "", GatherOpts{
				InputType: "speech",
				Timeout:   15,
				Language:  "en-US",
			})
		}
	}
}

// handleMenuSelection routes the call based on DTMF input.
func (v *VoicePlugin) handleMenuSelection(ctx context.Context, callID, digits string) {
	for _, opt := range v.cfg.MenuOptions {
		if opt.Digit == digits {
			if opt.TransferTo != "" {
				// Transfer to human.
				v.telephony.Transfer(ctx, callID, opt.TransferTo)
				return
			}

			// Route to agent.
			response, err := v.handler(ctx, plugin.InboundMessage{
				Text:    opt.Prompt,
				Channel: "voice",
				UserID:  callID,
				Metadata: map[string]string{
					"agent":  opt.AgentName,
					"digits": digits,
				},
			})
			if err != nil {
				v.telephony.Speak(ctx, callID, "I'm sorry, I encountered an error. Please try again.")
				return
			}
			v.telephony.Speak(ctx, callID, response)
			return
		}
	}

	// Invalid selection.
	v.telephony.Speak(ctx, callID, "Sorry, that's not a valid option. Please try again.")
	v.greetCaller(ctx, callID)
}

// handleSpeechInput sends the caller's spoken words to the agent.
func (v *VoicePlugin) handleSpeechInput(ctx context.Context, callID, speech string) {
	v.mu.RLock()
	state := v.calls[callID]
	v.mu.RUnlock()

	if state != nil {
		state.Transcript = append(state.Transcript, TranscriptEntry{
			Role: "caller",
			Text: speech,
			Time: time.Now().Format(time.RFC3339),
		})
	}

	// Send to the orchestrator.
	response, err := v.handler(ctx, plugin.InboundMessage{
		Text:    speech,
		Channel: "voice",
		UserID:  callID,
		Metadata: map[string]string{
			"caller_number": state.CallerNumber,
			"called_number": state.CalledNumber,
		},
	})
	if err != nil {
		log.Printf("[voice] Agent error for call %s: %v", callID, err)
		v.telephony.Speak(ctx, callID, "I'm sorry, I had trouble understanding. Could you repeat that?")
		return
	}

	if state != nil {
		state.Transcript = append(state.Transcript, TranscriptEntry{
			Role: "agent",
			Text: response,
			Time: time.Now().Format(time.RFC3339),
		})
	}

	// Speak the response and listen for more.
	v.telephony.Speak(ctx, callID, response)
	v.telephony.Gather(ctx, callID, "", GatherOpts{
		InputType: "speech",
		Timeout:   15,
		Language:  "en-US",
	})
}

// handleStatus returns the voice plugin status.
func (v *VoicePlugin) handleStatus(w http.ResponseWriter, r *http.Request) {
	v.mu.RLock()
	activeCount := len(v.calls)
	v.mu.RUnlock()

	status := map[string]interface{}{
		"status":       "running",
		"provider":     v.cfg.TelephonyProvider,
		"stt":          v.cfg.STTProvider,
		"tts":          v.cfg.TTSProvider,
		"active_calls": activeCount,
		"menu_enabled": v.cfg.MenuEnabled,
		"port":         v.cfg.Port,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleCallList returns active calls.
func (v *VoicePlugin) handleCallList(w http.ResponseWriter, r *http.Request) {
	v.mu.RLock()
	calls := make([]CallState, 0, len(v.calls))
	for _, c := range v.calls {
		calls = append(calls, *c)
	}
	v.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"calls": calls})
}

// ── Adapter Factories ──────────────────────────────────────────────

func newTelephonyAdapter(provider string, cfg map[string]string) TelephonyAdapter {
	switch provider {
	case "twilio":
		return NewTwilioAdapter(cfg)
	case "telnyx":
		return NewTelnyxAdapter(cfg)
	case "vonage":
		return NewVonageAdapter(cfg)
	case "plivo":
		return NewPlivoAdapter(cfg)
	case "bandwidth":
		return NewBandwidthAdapter(cfg)
	default:
		log.Printf("[voice] Unknown telephony provider %q, using stub", provider)
		return nil
	}
}

func newSTTAdapter(provider string, cfg map[string]string) STTAdapter {
	switch provider {
	case "deepgram":
		return NewDeepgramSTT(cfg)
	case "assemblyai":
		return NewAssemblyAISTT(cfg)
	case "whisper":
		return NewWhisperSTT(cfg)
	default:
		return nil
	}
}

func newTTSAdapter(provider string, cfg map[string]string) TTSAdapter {
	switch provider {
	case "elevenlabs":
		return NewElevenLabsTTS(cfg)
	case "local":
		return NewLocalTTS(cfg)
	default:
		return nil
	}
}
