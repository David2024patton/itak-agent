package voice

// Config holds the full voice plugin configuration.
type Config struct {
	Port              int               `yaml:"port,omitempty" json:"port,omitempty"`
	TelephonyProvider string            `yaml:"telephony_provider" json:"telephony_provider"` // twilio, telnyx, vonage, plivo, bandwidth
	TelephonyConfig   map[string]string `yaml:"telephony_config" json:"telephony_config"`     // provider-specific keys
	STTProvider       string            `yaml:"stt_provider" json:"stt_provider"`              // deepgram, assemblyai, whisper
	STTConfig         map[string]string `yaml:"stt_config" json:"stt_config"`
	TTSProvider       string            `yaml:"tts_provider" json:"tts_provider"` // elevenlabs, local
	TTSConfig         map[string]string `yaml:"tts_config" json:"tts_config"`
	GreetingMessage   string            `yaml:"greeting" json:"greeting"`
	MenuEnabled       bool              `yaml:"menu_enabled" json:"menu_enabled"`
	MenuOptions       []MenuOption      `yaml:"menu_options,omitempty" json:"menu_options,omitempty"`
}

// MenuOption defines one IVR menu entry (press 1 for X, 2 for Y, etc.).
type MenuOption struct {
	Digit       string `yaml:"digit" json:"digit"`             // "1", "2", "0"
	Label       string `yaml:"label" json:"label"`             // "Sales", "Support"
	AgentName   string `yaml:"agent_name" json:"agent_name"`   // focused agent to route to
	Prompt      string `yaml:"prompt" json:"prompt"`           // system prompt for this option
	TransferTo  string `yaml:"transfer_to" json:"transfer_to"` // phone number if transferring
}

// CallState tracks the state of an active call for the agent loop.
type CallState struct {
	CallID        string            // provider-specific call identifier
	CallerNumber  string            // E.164 caller number
	CalledNumber  string            // E.164 number they dialed
	Provider      string            // which telephony provider
	Status        string            // ringing, answered, ended
	Transcript    []TranscriptEntry // rolling transcript
	Metadata      map[string]string // extra provider data
}

// TranscriptEntry is one turn of the call conversation.
type TranscriptEntry struct {
	Role    string `json:"role"`    // "caller" or "agent"
	Text    string `json:"text"`
	Time    string `json:"time"`
}
