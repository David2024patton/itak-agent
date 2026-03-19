package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// TwilioAdapter implements TelephonyAdapter for Twilio.
// Twilio sends POST form-encoded webhooks and expects TwiML XML responses.
//
// Required config keys: account_sid, auth_token, phone_number
//
// Webhook POST params: CallSid, From, To, CallStatus, Digits, SpeechResult
// Response format: TwiML XML (<Response><Say>, <Gather>, <Dial>, <Hangup>)
// API base: https://api.twilio.com/2010-04-01/Accounts/{AccountSid}
type TwilioAdapter struct {
	accountSID  string
	authToken   string
	phoneNumber string
	apiBase     string
}

func NewTwilioAdapter(cfg map[string]string) *TwilioAdapter {
	sid := cfg["account_sid"]
	return &TwilioAdapter{
		accountSID:  sid,
		authToken:   cfg["auth_token"],
		phoneNumber: cfg["phone_number"],
		apiBase:     fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s", sid),
	}
}

func (t *TwilioAdapter) Name() string { return "twilio" }

func (t *TwilioAdapter) HandleWebhook(ctx context.Context, body []byte, params map[string]string) (*CallEvent, []byte, error) {
	// Twilio sends form-encoded POST. Params are already parsed.
	callSid := params["CallSid"]
	callStatus := params["CallStatus"]

	event := &CallEvent{
		CallID:    callSid,
		From:      params["From"],
		To:        params["To"],
		Digits:    params["Digits"],
		Direction: params["Direction"],
		RawPayload: map[string]interface{}{
			"CallStatus":  callStatus,
			"FromCity":    params["FromCity"],
			"FromState":   params["FromState"],
			"FromCountry": params["FromCountry"],
		},
	}

	// Map Twilio status to normalized event type.
	switch callStatus {
	case "ringing":
		event.Type = "initiated"
	case "in-progress":
		event.Type = "answered"
	case "completed", "busy", "failed", "no-answer":
		event.Type = "hangup"
	}

	// If Digits present, it's a DTMF event.
	if params["Digits"] != "" {
		event.Type = "dtmf"
	}
	// If SpeechResult present, it's a speech event.
	if params["SpeechResult"] != "" {
		event.Type = "speech"
		event.SpeechResult = params["SpeechResult"]
	}

	// Return nil response body -- the main plugin generates TwiML as needed.
	return event, nil, nil
}

func (t *TwilioAdapter) Speak(ctx context.Context, callID string, text string) error {
	// For Twilio, speaking during an active call requires the Calls API update.
	// POST /Calls/{CallSid}.json with Twiml=<Response><Say>...</Say></Response>
	twiml := fmt.Sprintf(`<Response><Say voice="alice">%s</Say></Response>`, xmlEscape(text))

	reqURL := fmt.Sprintf("%s/Calls/%s.json", t.apiBase, callID)
	data := url.Values{"Twiml": {twiml}}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio speak: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twilio speak: %d %s", resp.StatusCode, string(b))
	}
	return nil
}

func (t *TwilioAdapter) Gather(ctx context.Context, callID string, prompt string, opts GatherOpts) error {
	// Build TwiML for gathering input.
	input := "dtmf"
	if opts.InputType != "" {
		input = opts.InputType
	}
	timeout := 10
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	numDigits := ""
	if opts.NumDigits > 0 {
		numDigits = fmt.Sprintf(` numDigits="%d"`, opts.NumDigits)
	}
	language := ""
	if opts.Language != "" {
		language = fmt.Sprintf(` language="%s"`, opts.Language)
	}

	var sayTag string
	if prompt != "" {
		sayTag = fmt.Sprintf(`<Say voice="alice">%s</Say>`, xmlEscape(prompt))
	}

	twiml := fmt.Sprintf(`<Response><Gather input="%s" timeout="%d"%s%s action="/voice/webhook">%s</Gather></Response>`,
		input, timeout, numDigits, language, sayTag)

	reqURL := fmt.Sprintf("%s/Calls/%s.json", t.apiBase, callID)
	data := url.Values{"Twiml": {twiml}}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio gather: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (t *TwilioAdapter) Transfer(ctx context.Context, callID string, toNumber string) error {
	twiml := fmt.Sprintf(`<Response><Dial>%s</Dial></Response>`, xmlEscape(toNumber))

	reqURL := fmt.Sprintf("%s/Calls/%s.json", t.apiBase, callID)
	data := url.Values{"Twiml": {twiml}}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio transfer: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (t *TwilioAdapter) Hangup(ctx context.Context, callID string) error {
	reqURL := fmt.Sprintf("%s/Calls/%s.json", t.apiBase, callID)
	data := url.Values{"Status": {"completed"}}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("twilio hangup: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (t *TwilioAdapter) MakeCall(ctx context.Context, from, to string) (string, error) {
	reqURL := fmt.Sprintf("%s/Calls.json", t.apiBase)
	data := url.Values{
		"From": {from},
		"To":   {to},
		"Twiml": {`<Response><Say voice="alice">Hello, this is a call from your AI assistant.</Say></Response>`},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("twilio make call: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	sid, _ := result["sid"].(string)
	return sid, nil
}

// xmlEscape replaces XML-unsafe characters.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// ── Telnyx Adapter ─────────────────────────────────────────────────

// TelnyxAdapter implements TelephonyAdapter for Telnyx.
// Telnyx sends JSON webhooks and accepts REST command calls.
//
// Required config keys: api_key, connection_id, phone_number
//
// Webhook JSON: { event_type, payload: { call_control_id, from, to, ... } }
// Command API: POST https://api.telnyx.com/v2/calls/{call_control_id}/actions/{action}
// Auth: Bearer token
type TelnyxAdapter struct {
	apiKey       string
	connectionID string
	phoneNumber  string
}

func NewTelnyxAdapter(cfg map[string]string) *TelnyxAdapter {
	return &TelnyxAdapter{
		apiKey:       cfg["api_key"],
		connectionID: cfg["connection_id"],
		phoneNumber:  cfg["phone_number"],
	}
}

func (t *TelnyxAdapter) Name() string { return "telnyx" }

func (t *TelnyxAdapter) HandleWebhook(ctx context.Context, body []byte, params map[string]string) (*CallEvent, []byte, error) {
	var webhook struct {
		EventType string `json:"event_type"`
		Payload   struct {
			CallControlID string `json:"call_control_id"`
			From          string `json:"from"`
			To            string `json:"to"`
			Direction     string `json:"direction"`
			ClientState   string `json:"client_state"`
			State         string `json:"state"`
			// Gather results
			Digits string `json:"digits"`
			// Transcription results
			TranscriptionData struct {
				Transcript string `json:"transcript"`
			} `json:"transcription_data"`
		} `json:"payload"`
	}

	if err := json.Unmarshal(body, &webhook); err != nil {
		return nil, nil, fmt.Errorf("telnyx parse: %w", err)
	}

	event := &CallEvent{
		CallID:    webhook.Payload.CallControlID,
		From:      webhook.Payload.From,
		To:        webhook.Payload.To,
		Direction: webhook.Payload.Direction,
	}

	switch webhook.EventType {
	case "call.initiated":
		event.Type = "initiated"
		// Auto-answer inbound calls.
		if webhook.Payload.Direction == "incoming" {
			go t.answerCall(ctx, webhook.Payload.CallControlID)
		}
	case "call.answered":
		event.Type = "answered"
	case "call.hangup":
		event.Type = "hangup"
	case "call.gather.ended":
		if webhook.Payload.Digits != "" {
			event.Type = "dtmf"
			event.Digits = webhook.Payload.Digits
		}
	case "call.transcription":
		event.Type = "speech"
		event.SpeechResult = webhook.Payload.TranscriptionData.Transcript
	}

	return event, nil, nil
}

func (t *TelnyxAdapter) answerCall(ctx context.Context, callControlID string) {
	t.sendCommand(ctx, callControlID, "answer", nil)
}

func (t *TelnyxAdapter) Speak(ctx context.Context, callID string, text string) error {
	return t.sendCommand(ctx, callID, "speak", map[string]interface{}{
		"payload":  text,
		"voice":    "female",
		"language": "en-US",
	})
}

func (t *TelnyxAdapter) Gather(ctx context.Context, callID string, prompt string, opts GatherOpts) error {
	payload := map[string]interface{}{
		"maximum_digits":  opts.NumDigits,
		"timeout_millis":  opts.Timeout * 1000,
	}
	if prompt != "" {
		payload["initial_prompt"] = map[string]interface{}{
			"payload":  prompt,
			"voice":    "female",
			"language": "en-US",
		}
	}
	if opts.InputType == "speech" {
		// Use transcription instead of gather for speech.
		return t.sendCommand(ctx, callID, "transcription_start", map[string]interface{}{
			"language": opts.Language,
		})
	}
	return t.sendCommand(ctx, callID, "gather_using_speak", payload)
}

func (t *TelnyxAdapter) Transfer(ctx context.Context, callID string, toNumber string) error {
	return t.sendCommand(ctx, callID, "transfer", map[string]interface{}{
		"to": toNumber,
	})
}

func (t *TelnyxAdapter) Hangup(ctx context.Context, callID string) error {
	return t.sendCommand(ctx, callID, "hangup", nil)
}

func (t *TelnyxAdapter) MakeCall(ctx context.Context, from, to string) (string, error) {
	payload := map[string]interface{}{
		"to":            to,
		"from":          from,
		"connection_id": t.connectionID,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.telnyx.com/v2/calls",
		strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("telnyx make call: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			CallControlID string `json:"call_control_id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Data.CallControlID, nil
}

func (t *TelnyxAdapter) sendCommand(ctx context.Context, callControlID, action string, payload map[string]interface{}) error {
	reqURL := fmt.Sprintf("https://api.telnyx.com/v2/calls/%s/actions/%s", callControlID, action)

	var bodyReader io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		bodyReader = strings.NewReader(string(b))
	} else {
		bodyReader = strings.NewReader("{}")
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("telnyx %s: %w", action, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telnyx %s: %d %s", action, resp.StatusCode, string(b))
	}
	return nil
}

// ── Vonage Adapter ─────────────────────────────────────────────────

// VonageAdapter implements TelephonyAdapter for Vonage (Nexmo).
// Vonage sends JSON webhooks and expects NCCO (Nexmo Call Control Object) JSON responses.
//
// Required config keys: api_key, api_secret, application_id, private_key, phone_number
//
// Webhook JSON: { conversation_uuid, from, to, uuid }
// Response: NCCO JSON array [{action:"talk",text:"..."},{action:"input",type:["dtmf"]}]
// API: POST https://api.nexmo.com/v1/calls
type VonageAdapter struct {
	apiKey        string
	apiSecret     string
	applicationID string
	phoneNumber   string
}

func NewVonageAdapter(cfg map[string]string) *VonageAdapter {
	return &VonageAdapter{
		apiKey:        cfg["api_key"],
		apiSecret:     cfg["api_secret"],
		applicationID: cfg["application_id"],
		phoneNumber:   cfg["phone_number"],
	}
}

func (v *VonageAdapter) Name() string { return "vonage" }

func (v *VonageAdapter) HandleWebhook(ctx context.Context, body []byte, params map[string]string) (*CallEvent, []byte, error) {
	var webhook map[string]interface{}
	if err := json.Unmarshal(body, &webhook); err != nil {
		return nil, nil, fmt.Errorf("vonage parse: %w", err)
	}

	event := &CallEvent{
		CallID:     getString(webhook, "uuid"),
		From:       getString(webhook, "from"),
		To:         getString(webhook, "to"),
		Direction:  getString(webhook, "direction"),
		RawPayload: webhook,
	}

	status := getString(webhook, "status")
	switch status {
	case "started", "ringing":
		event.Type = "initiated"
	case "answered":
		event.Type = "answered"
	case "completed":
		event.Type = "hangup"
	}

	// DTMF input from Vonage.
	if dtmf, ok := webhook["dtmf"].(map[string]interface{}); ok {
		event.Type = "dtmf"
		event.Digits = getString(dtmf, "digits")
	}

	// Speech input from Vonage.
	if speech, ok := webhook["speech"].(map[string]interface{}); ok {
		if results, ok := speech["results"].([]interface{}); ok && len(results) > 0 {
			if first, ok := results[0].(map[string]interface{}); ok {
				event.Type = "speech"
				event.SpeechResult = getString(first, "text")
			}
		}
	}

	return event, nil, nil
}

func (v *VonageAdapter) Speak(ctx context.Context, callID string, text string) error {
	payload := map[string]interface{}{
		"text":  text,
		"loop":  1,
	}
	return v.callAction(ctx, callID, "talk", payload)
}

func (v *VonageAdapter) Gather(ctx context.Context, callID string, prompt string, opts GatherOpts) error {
	inputType := []string{"dtmf"}
	if opts.InputType == "speech" {
		inputType = []string{"speech"}
	} else if opts.InputType == "dtmf speech" {
		inputType = []string{"dtmf", "speech"}
	}

	payload := map[string]interface{}{
		"type":       inputType,
		"eventUrl":   []string{"/voice/webhook"},
	}
	if opts.NumDigits > 0 {
		payload["dtmf"] = map[string]interface{}{"maxDigits": opts.NumDigits}
	}
	if opts.Timeout > 0 {
		payload["speech"] = map[string]interface{}{"endOnSilence": opts.Timeout}
	}

	// Vonage uses NCCO actions in sequence.
	ncco := []map[string]interface{}{}
	if prompt != "" {
		ncco = append(ncco, map[string]interface{}{
			"action": "talk",
			"text":   prompt,
		})
	}
	ncco = append(ncco, map[string]interface{}{
		"action": "input",
		"type":   inputType,
	})

	_, err := json.Marshal(ncco)
	return err
}

func (v *VonageAdapter) Transfer(ctx context.Context, callID string, toNumber string) error {
	payload := map[string]interface{}{
		"action": "transfer",
		"destination": map[string]interface{}{
			"type":   "ncco",
			"ncco":   []map[string]interface{}{{"action": "connect", "endpoint": []map[string]interface{}{{"type": "phone", "number": toNumber}}}},
		},
	}
	return v.callAction(ctx, callID, "transfer", payload)
}

func (v *VonageAdapter) Hangup(ctx context.Context, callID string) error {
	reqURL := fmt.Sprintf("https://api.nexmo.com/v1/calls/%s", callID)
	payload := map[string]interface{}{"action": "hangup"}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(v.apiKey, v.apiSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("vonage hangup: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (v *VonageAdapter) MakeCall(ctx context.Context, from, to string) (string, error) {
	payload := map[string]interface{}{
		"to":   []map[string]interface{}{{"type": "phone", "number": to}},
		"from": map[string]interface{}{"type": "phone", "number": from},
		"ncco": []map[string]interface{}{{"action": "talk", "text": "Hello from your AI assistant."}},
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.nexmo.com/v1/calls", strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(v.apiKey, v.apiSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("vonage make call: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return getString(result, "uuid"), nil
}

func (v *VonageAdapter) callAction(ctx context.Context, callID, action string, payload map[string]interface{}) error {
	reqURL := fmt.Sprintf("https://api.nexmo.com/v1/calls/%s/%s", callID, action)
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(v.apiKey, v.apiSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("vonage %s: %w", action, err)
	}
	defer resp.Body.Close()
	return nil
}

// ── Plivo Adapter ──────────────────────────────────────────────────

// PlivoAdapter implements TelephonyAdapter for Plivo.
// Plivo sends POST form-encoded webhooks and expects Plivo XML responses.
//
// Required config keys: auth_id, auth_token, phone_number
//
// Webhook POST params: CallUUID, From, To, Direction, Digits
// Response: Plivo XML (<Response><Speak>, <GetDigits>, <Dial>, <Hangup>)
// API: https://api.plivo.com/v1/Account/{AuthID}/Call/
type PlivoAdapter struct {
	authID      string
	authToken   string
	phoneNumber string
}

func NewPlivoAdapter(cfg map[string]string) *PlivoAdapter {
	return &PlivoAdapter{
		authID:      cfg["auth_id"],
		authToken:   cfg["auth_token"],
		phoneNumber: cfg["phone_number"],
	}
}

func (p *PlivoAdapter) Name() string { return "plivo" }

func (p *PlivoAdapter) HandleWebhook(ctx context.Context, body []byte, params map[string]string) (*CallEvent, []byte, error) {
	event := &CallEvent{
		CallID:    params["CallUUID"],
		From:      params["From"],
		To:        params["To"],
		Direction: params["Direction"],
		Digits:    params["Digits"],
	}

	callStatus := params["CallStatus"]
	switch callStatus {
	case "ringing":
		event.Type = "initiated"
	case "in-progress":
		event.Type = "answered"
	case "completed", "busy", "failed", "no-answer":
		event.Type = "hangup"
	}

	if params["Digits"] != "" {
		event.Type = "dtmf"
	}
	if params["SpeechResult"] != "" {
		event.Type = "speech"
		event.SpeechResult = params["SpeechResult"]
	}

	return event, nil, nil
}

func (p *PlivoAdapter) Speak(ctx context.Context, callID string, text string) error {
	reqURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/%s/Speak/", p.authID, callID)
	payload := map[string]interface{}{
		"text":  text,
		"voice": "WOMAN",
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.authID, p.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("plivo speak: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (p *PlivoAdapter) Gather(ctx context.Context, callID string, prompt string, opts GatherOpts) error {
	// Plivo uses GetDigits in XML response; for active calls, use the DTMF API.
	reqURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/%s/DTMF/", p.authID, callID)
	payload := map[string]interface{}{
		"digits": "*", // Signal to gather
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.authID, p.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("plivo gather: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (p *PlivoAdapter) Transfer(ctx context.Context, callID string, toNumber string) error {
	reqURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/%s/", p.authID, callID)
	payload := map[string]interface{}{
		"legs":    "aleg",
		"aleg_url": fmt.Sprintf("/voice/webhook?transfer_to=%s", toNumber),
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.authID, p.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("plivo transfer: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (p *PlivoAdapter) Hangup(ctx context.Context, callID string) error {
	reqURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/%s/", p.authID, callID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.authID, p.authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("plivo hangup: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (p *PlivoAdapter) MakeCall(ctx context.Context, from, to string) (string, error) {
	reqURL := fmt.Sprintf("https://api.plivo.com/v1/Account/%s/Call/", p.authID)
	payload := map[string]interface{}{
		"from":       from,
		"to":         to,
		"answer_url": "/voice/webhook",
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(p.authID, p.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("plivo make call: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	uuid, _ := result["request_uuid"].(string)
	return uuid, nil
}

// ── Bandwidth Adapter ──────────────────────────────────────────────

// BandwidthAdapter implements TelephonyAdapter for Bandwidth.
// Bandwidth sends JSON webhooks and expects BXML (Bandwidth XML) responses.
//
// Required config keys: account_id, username, password, application_id, phone_number
//
// Webhook JSON: { eventType, from, to, callId, ... }
// Response: BXML (<Response><SpeakSentence>, <Gather>, <Transfer>, <Hangup>)
// API: https://voice.bandwidth.com/api/v2/accounts/{accountId}/calls
type BandwidthAdapter struct {
	accountID     string
	username      string
	password      string
	applicationID string
	phoneNumber   string
}

func NewBandwidthAdapter(cfg map[string]string) *BandwidthAdapter {
	return &BandwidthAdapter{
		accountID:     cfg["account_id"],
		username:      cfg["username"],
		password:      cfg["password"],
		applicationID: cfg["application_id"],
		phoneNumber:   cfg["phone_number"],
	}
}

func (bw *BandwidthAdapter) Name() string { return "bandwidth" }

func (bw *BandwidthAdapter) HandleWebhook(ctx context.Context, body []byte, params map[string]string) (*CallEvent, []byte, error) {
	var webhook map[string]interface{}
	if err := json.Unmarshal(body, &webhook); err != nil {
		return nil, nil, fmt.Errorf("bandwidth parse: %w", err)
	}

	event := &CallEvent{
		CallID:     getString(webhook, "callId"),
		From:       getString(webhook, "from"),
		To:         getString(webhook, "to"),
		Direction:  getString(webhook, "direction"),
		RawPayload: webhook,
	}

	eventType := getString(webhook, "eventType")
	switch eventType {
	case "initiate":
		event.Type = "initiated"
	case "answer":
		event.Type = "answered"
	case "disconnect":
		event.Type = "hangup"
	case "gather":
		event.Type = "dtmf"
		event.Digits = getString(webhook, "digits")
	case "transcription":
		event.Type = "speech"
		event.SpeechResult = getString(webhook, "transcript")
	}

	return event, nil, nil
}

func (bw *BandwidthAdapter) Speak(ctx context.Context, callID string, text string) error {
	bxml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Response><SpeakSentence>%s</SpeakSentence></Response>`,
		xmlEscape(text))

	reqURL := fmt.Sprintf("https://voice.bandwidth.com/api/v2/accounts/%s/calls/%s/bxml", bw.accountID, callID)
	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(bxml))
	if err != nil {
		return err
	}
	req.SetBasicAuth(bw.username, bw.password)
	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("bandwidth speak: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (bw *BandwidthAdapter) Gather(ctx context.Context, callID string, prompt string, opts GatherOpts) error {
	var speakTag string
	if prompt != "" {
		speakTag = fmt.Sprintf("<SpeakSentence>%s</SpeakSentence>", xmlEscape(prompt))
	}
	timeout := 10
	if opts.Timeout > 0 {
		timeout = opts.Timeout
	}
	maxDigits := 1
	if opts.NumDigits > 0 {
		maxDigits = opts.NumDigits
	}

	bxml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Response><Gather gatherUrl="/voice/webhook" maxDigits="%d" firstDigitTimeout="%d">%s</Gather></Response>`,
		maxDigits, timeout, speakTag)

	reqURL := fmt.Sprintf("https://voice.bandwidth.com/api/v2/accounts/%s/calls/%s/bxml", bw.accountID, callID)
	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(bxml))
	if err != nil {
		return err
	}
	req.SetBasicAuth(bw.username, bw.password)
	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("bandwidth gather: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (bw *BandwidthAdapter) Transfer(ctx context.Context, callID string, toNumber string) error {
	bxml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Response><Transfer transferCallerId="%s"><PhoneNumber>%s</PhoneNumber></Transfer></Response>`,
		bw.phoneNumber, toNumber)

	reqURL := fmt.Sprintf("https://voice.bandwidth.com/api/v2/accounts/%s/calls/%s/bxml", bw.accountID, callID)
	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(bxml))
	if err != nil {
		return err
	}
	req.SetBasicAuth(bw.username, bw.password)
	req.Header.Set("Content-Type", "application/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("bandwidth transfer: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (bw *BandwidthAdapter) Hangup(ctx context.Context, callID string) error {
	reqURL := fmt.Sprintf("https://voice.bandwidth.com/api/v2/accounts/%s/calls/%s", bw.accountID, callID)
	payload := map[string]interface{}{"state": "completed"}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.SetBasicAuth(bw.username, bw.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("bandwidth hangup: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (bw *BandwidthAdapter) MakeCall(ctx context.Context, from, to string) (string, error) {
	reqURL := fmt.Sprintf("https://voice.bandwidth.com/api/v2/accounts/%s/calls", bw.accountID)
	payload := map[string]interface{}{
		"from":          from,
		"to":            to,
		"applicationId": bw.applicationID,
		"answerUrl":     "/voice/webhook",
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, strings.NewReader(string(b)))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(bw.username, bw.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("bandwidth make call: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return getString(result, "callId"), nil
}

// getString safely extracts a string from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Ensure unused imports are consumed.
var _ = base64.StdEncoding
var _ = log.Println
