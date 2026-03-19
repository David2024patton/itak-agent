package builtins

import (
	"context"
	"fmt"
)

// VoiceCallAction is set by the voice plugin at startup so these tools
// can interact with active phone calls. Nil when voice is disabled.
var VoiceCallAction func(ctx context.Context, action string, args map[string]interface{}) (string, error)

// ── Voice Answer ───────────────────────────────────────────────────

type VoiceAnswerTool struct{}

func (t *VoiceAnswerTool) Name() string        { return "voice_answer" }
func (t *VoiceAnswerTool) Description() string {
	return "Answer an incoming phone call. Use when a new call comes in and you want to greet the caller."
}
func (t *VoiceAnswerTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"call_id":  map[string]interface{}{"type": "string", "description": "The call ID to answer"},
			"greeting": map[string]interface{}{"type": "string", "description": "Greeting message to speak to the caller"},
		},
		"required": []string{"call_id"},
	}
}
func (t *VoiceAnswerTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "answer", args)
}

// ── Voice Speak ────────────────────────────────────────────────────

type VoiceSpeakTool struct{}

func (t *VoiceSpeakTool) Name() string        { return "voice_speak" }
func (t *VoiceSpeakTool) Description() string {
	return "Speak a message to the caller on an active phone call. The text is converted to speech using the configured TTS provider."
}
func (t *VoiceSpeakTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"call_id": map[string]interface{}{"type": "string", "description": "The call ID to speak on"},
			"text":    map[string]interface{}{"type": "string", "description": "Text to speak to the caller"},
		},
		"required": []string{"call_id", "text"},
	}
}
func (t *VoiceSpeakTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "speak", args)
}

// ── Voice Gather ───────────────────────────────────────────────────

type VoiceGatherTool struct{}

func (t *VoiceGatherTool) Name() string        { return "voice_gather" }
func (t *VoiceGatherTool) Description() string {
	return "Prompt the caller and collect input (DTMF digits or speech). Use for IVR menus or asking the caller a question."
}
func (t *VoiceGatherTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"call_id":    map[string]interface{}{"type": "string", "description": "The call ID"},
			"prompt":     map[string]interface{}{"type": "string", "description": "Text to speak as a prompt before gathering input"},
			"input_type": map[string]interface{}{"type": "string", "description": "Type of input: dtmf, speech, or dtmf speech"},
			"num_digits": map[string]interface{}{"type": "number", "description": "Max DTMF digits to collect (default 1)"},
			"timeout":    map[string]interface{}{"type": "number", "description": "Seconds to wait for input (default 10)"},
		},
		"required": []string{"call_id"},
	}
}
func (t *VoiceGatherTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "gather", args)
}

// ── Voice Transfer ─────────────────────────────────────────────────

type VoiceTransferTool struct{}

func (t *VoiceTransferTool) Name() string        { return "voice_transfer" }
func (t *VoiceTransferTool) Description() string {
	return "Transfer the call to a human or external phone number. The caller will be connected to the new number."
}
func (t *VoiceTransferTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"call_id":   map[string]interface{}{"type": "string", "description": "The call ID to transfer"},
			"to_number": map[string]interface{}{"type": "string", "description": "Phone number to transfer to (E.164 format, e.g. +15551234567)"},
		},
		"required": []string{"call_id", "to_number"},
	}
}
func (t *VoiceTransferTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "transfer", args)
}

// ── Voice Hold ─────────────────────────────────────────────────────

type VoiceHoldTool struct{}

func (t *VoiceHoldTool) Name() string        { return "voice_hold" }
func (t *VoiceHoldTool) Description() string {
	return "Put the caller on hold with a message. Useful while looking up information or processing a request."
}
func (t *VoiceHoldTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"call_id": map[string]interface{}{"type": "string", "description": "The call ID to hold"},
			"message": map[string]interface{}{"type": "string", "description": "Message to play (e.g. 'Please hold while I look that up.')"},
		},
		"required": []string{"call_id"},
	}
}
func (t *VoiceHoldTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "hold", args)
}

// ── Voice Hangup ───────────────────────────────────────────────────

type VoiceHangupTool struct{}

func (t *VoiceHangupTool) Name() string        { return "voice_hangup" }
func (t *VoiceHangupTool) Description() string {
	return "End an active phone call. Use when the conversation is complete."
}
func (t *VoiceHangupTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"call_id": map[string]interface{}{"type": "string", "description": "The call ID to hang up"},
		},
		"required": []string{"call_id"},
	}
}
func (t *VoiceHangupTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "hangup", args)
}

// ── Voice Record ───────────────────────────────────────────────────

type VoiceRecordTool struct{}

func (t *VoiceRecordTool) Name() string        { return "voice_record" }
func (t *VoiceRecordTool) Description() string {
	return "Start or stop recording a phone call. Recordings are saved for later review."
}
func (t *VoiceRecordTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"call_id": map[string]interface{}{"type": "string", "description": "The call ID to record"},
			"action":  map[string]interface{}{"type": "string", "description": "start or stop"},
		},
		"required": []string{"call_id", "action"},
	}
}
func (t *VoiceRecordTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "record", args)
}

// ── Voice Call List ────────────────────────────────────────────────

type VoiceCallListTool struct{}

func (t *VoiceCallListTool) Name() string        { return "voice_call_list" }
func (t *VoiceCallListTool) Description() string {
	return "List all active phone calls with their status, caller info, and duration."
}
func (t *VoiceCallListTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}
func (t *VoiceCallListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "[]", nil
	}
	return VoiceCallAction(ctx, "list", args)
}

// ── Voice Make Call ────────────────────────────────────────────────

type VoiceMakeCallTool struct{}

func (t *VoiceMakeCallTool) Name() string        { return "voice_make_call" }
func (t *VoiceMakeCallTool) Description() string {
	return "Make an outbound phone call to a number. The agent can then speak to the recipient."
}
func (t *VoiceMakeCallTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to":   map[string]interface{}{"type": "string", "description": "Phone number to call (E.164 format, e.g. +15551234567)"},
			"from": map[string]interface{}{"type": "string", "description": "Your phone number to call from (E.164 format)"},
		},
		"required": []string{"to"},
	}
}
func (t *VoiceMakeCallTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	if VoiceCallAction == nil {
		return "", fmt.Errorf("voice plugin not active")
	}
	return VoiceCallAction(ctx, "make_call", args)
}
