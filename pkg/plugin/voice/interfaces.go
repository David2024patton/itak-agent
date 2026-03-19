package voice

import "context"

// TelephonyAdapter provides a uniform interface over different voice providers.
// Each implementation handles the provider-specific webhook format and call
// control commands, translating them to/from the standard CallState.
type TelephonyAdapter interface {
	// Name returns the provider identifier (e.g. "twilio", "telnyx").
	Name() string

	// HandleWebhook processes an incoming HTTP webhook from the provider.
	// Returns the parsed call event and a response body (TwiML, NCCO, etc.).
	HandleWebhook(ctx context.Context, body []byte, params map[string]string) (*CallEvent, []byte, error)

	// Speak sends a text-to-speech message on an active call.
	Speak(ctx context.Context, callID string, text string) error

	// Gather prompts the caller and waits for DTMF or speech input.
	Gather(ctx context.Context, callID string, prompt string, opts GatherOpts) error

	// Transfer forwards the call to another phone number.
	Transfer(ctx context.Context, callID string, toNumber string) error

	// Hangup ends the call.
	Hangup(ctx context.Context, callID string) error

	// MakeCall initiates an outbound call.
	MakeCall(ctx context.Context, from, to string) (string, error)
}

// CallEvent is the normalized event parsed from any provider's webhook.
type CallEvent struct {
	Type         string // "initiated", "answered", "dtmf", "speech", "hangup"
	CallID       string // provider-specific call ID
	From         string // caller number (E.164)
	To           string // called number (E.164)
	Digits       string // DTMF digits if type=dtmf
	SpeechResult string // transcript if type=speech
	Direction    string // "inbound" or "outbound"
	RawPayload   map[string]interface{}
}

// GatherOpts configures how to collect caller input.
type GatherOpts struct {
	InputType  string // "dtmf", "speech", "dtmf speech"
	NumDigits  int    // max DTMF digits to collect
	Timeout    int    // seconds to wait
	Language   string // BCP-47 language code for speech
}

// STTAdapter converts speech audio to text.
type STTAdapter interface {
	Name() string
	// TranscribeAudio takes raw audio bytes and returns the transcript.
	TranscribeAudio(ctx context.Context, audio []byte, format string) (string, error)
}

// TTSAdapter converts text to speech audio.
type TTSAdapter interface {
	Name() string
	// Synthesize takes text and returns audio bytes.
	Synthesize(ctx context.Context, text string, format string) ([]byte, error)
}
