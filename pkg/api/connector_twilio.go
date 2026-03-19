package api

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

// ────────────────────────────────────────────────────────────────────
// TwilioConnector: Phone/SMS integration via Twilio REST API.
//
// What: Send/receive SMS, make calls, manage phone numbers via Twilio.
// Why:  Powers the Phone System page with real telephony.
// How:  Uses Twilio's REST API (api.twilio.com/2010-04-01).
//       Auth: HTTP Basic (AccountSID:AuthToken).
//       Credential fields: account_sid, auth_token, from_number.
// ────────────────────────────────────────────────────────────────────

// TwilioConnector implements Connector for Twilio SMS/Voice.
type TwilioConnector struct {
	registry *ConnectorRegistry
}

// NewTwilioConnector creates a Twilio connector.
func NewTwilioConnector(registry *ConnectorRegistry) *TwilioConnector {
	return &TwilioConnector{registry: registry}
}

func (t *TwilioConnector) Name() string     { return "twilio" }
func (t *TwilioConnector) Category() string  { return "phone" }

func (t *TwilioConnector) Actions() []string {
	return []string{
		"send-sms",
		"list-numbers",
		"call-log",
		"make-call",
		"buy-number",
		"search-numbers",
		"list-messages",
	}
}

func (t *TwilioConnector) getCreds() (sid, token, fromNum string, err error) {
	creds, err := t.registry.GetConnectorCreds("twilio")
	if err != nil {
		return "", "", "", fmt.Errorf("twilio credentials not configured: %w", err)
	}
	sid = creds.Fields["account_sid"]
	token = creds.Fields["auth_token"]
	fromNum = creds.Fields["from_number"]
	if sid == "" || token == "" {
		return "", "", "", fmt.Errorf("twilio credentials incomplete: need account_sid and auth_token")
	}
	return sid, token, fromNum, nil
}

func (t *TwilioConnector) client() (*ConnectorHTTPClient, string, error) {
	sid, token, _, err := t.getCreds()
	if err != nil {
		return nil, "", err
	}
	auth := base64.StdEncoding.EncodeToString([]byte(sid + ":" + token))
	c := NewConnectorHTTPClient(
		fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s", sid),
		map[string]string{"Authorization": "Basic " + auth},
	)
	return c, sid, nil
}

func (t *TwilioConnector) TestConnection() error {
	c, _, err := t.client()
	if err != nil {
		return err
	}
	_, err = c.DoRequest("GET", ".json", nil)
	return err
}

func (t *TwilioConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "send-sms":
		return t.sendSMS(params)
	case "list-numbers":
		return t.listNumbers()
	case "call-log":
		return t.callLog(params)
	case "make-call":
		return t.makeCall(params)
	case "search-numbers":
		return t.searchNumbers(params)
	case "list-messages":
		return t.listMessages(params)
	default:
		return nil, fmt.Errorf("twilio: unknown action %q, available: %v", action, t.Actions())
	}
}

func (t *TwilioConnector) sendSMS(params map[string]interface{}) (interface{}, error) {
	c, _, err := t.client()
	if err != nil {
		return nil, err
	}
	_, _, fromNum, _ := t.getCreds()

	to := pStr(params, "to")
	body := pStr(params, "body")
	from := pStr(params, "from")
	if from == "" {
		from = fromNum
	}
	if to == "" || body == "" {
		return nil, fmt.Errorf("send-sms requires 'to' and 'body' params")
	}
	if from == "" {
		return nil, fmt.Errorf("send-sms requires 'from' param or from_number in credentials")
	}

	return c.DoFormRequest("Messages.json", map[string]string{
		"To":   to,
		"From": from,
		"Body": body,
	})
}

func (t *TwilioConnector) listNumbers() (interface{}, error) {
	c, _, err := t.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("GET", "IncomingPhoneNumbers.json", nil)
}

func (t *TwilioConnector) callLog(params map[string]interface{}) (interface{}, error) {
	c, _, err := t.client()
	if err != nil {
		return nil, err
	}
	path := "Calls.json"
	limit := pStr(params, "limit")
	if limit != "" {
		path += "?PageSize=" + url.QueryEscape(limit)
	}
	return c.DoRequest("GET", path, nil)
}

func (t *TwilioConnector) makeCall(params map[string]interface{}) (interface{}, error) {
	c, _, err := t.client()
	if err != nil {
		return nil, err
	}
	_, _, fromNum, _ := t.getCreds()

	to := pStr(params, "to")
	from := pStr(params, "from")
	twimlURL := pStr(params, "url")
	if from == "" {
		from = fromNum
	}
	if to == "" || from == "" {
		return nil, fmt.Errorf("make-call requires 'to' and 'from' params")
	}
	if twimlURL == "" {
		// Use a simple TwiML that says a greeting.
		twimlURL = "http://demo.twilio.com/docs/voice.xml"
	}

	return c.DoFormRequest("Calls.json", map[string]string{
		"To":   to,
		"From": from,
		"Url":  twimlURL,
	})
}

func (t *TwilioConnector) searchNumbers(params map[string]interface{}) (interface{}, error) {
	sid, token, _, err := t.getCreds()
	if err != nil {
		return nil, err
	}

	country := pStr(params, "country")
	if country == "" {
		country = "US"
	}

	auth := base64.StdEncoding.EncodeToString([]byte(sid + ":" + token))
	c := NewConnectorHTTPClient(
		fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/AvailablePhoneNumbers/%s", sid, country),
		map[string]string{"Authorization": "Basic " + auth},
	)
	return c.DoRequest("GET", "Local.json", nil)
}

func (t *TwilioConnector) listMessages(params map[string]interface{}) (interface{}, error) {
	c, _, err := t.client()
	if err != nil {
		return nil, err
	}
	path := "Messages.json"
	limit := pStr(params, "limit")
	if limit != "" {
		path += "?PageSize=" + url.QueryEscape(limit)
	}
	return c.DoRequest("GET", path, nil)
}
