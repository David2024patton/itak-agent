package api

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

// ────────────────────────────────────────────────────────────────────
// StripeConnector: Payments/Invoicing integration via Stripe REST API.
//
// What: Manage invoices, customers, payments, and subscriptions via Stripe.
// Why:  Powers the Payments page with real billing data.
// How:  Uses Stripe REST API (api.stripe.com/v1).
//       Auth: Bearer token (Secret Key).
//       Credential fields: secret_key (sk_...).
// ────────────────────────────────────────────────────────────────────

// StripeConnector implements Connector for Stripe.
type StripeConnector struct {
	registry *ConnectorRegistry
}

func NewStripeConnector(registry *ConnectorRegistry) *StripeConnector {
	return &StripeConnector{registry: registry}
}

func (s *StripeConnector) Name() string     { return "stripe" }
func (s *StripeConnector) Category() string  { return "payments" }

func (s *StripeConnector) Actions() []string {
	return []string{
		"list-invoices",
		"create-invoice",
		"list-customers",
		"create-customer",
		"list-payments",
		"list-subscriptions",
		"get-balance",
	}
}

func (s *StripeConnector) client() (*ConnectorHTTPClient, error) {
	creds, err := s.registry.GetConnectorCreds("stripe")
	if err != nil {
		return nil, fmt.Errorf("stripe credentials not configured: %w", err)
	}
	key := creds.Fields["secret_key"]
	if key == "" {
		key = creds.Fields["api_key"]
	}
	if key == "" {
		return nil, fmt.Errorf("stripe credentials incomplete: need secret_key")
	}
	return NewConnectorHTTPClient("https://api.stripe.com/v1", map[string]string{
		"Authorization": "Bearer " + key,
	}), nil
}

func (s *StripeConnector) TestConnection() error {
	c, err := s.client()
	if err != nil {
		return err
	}
	_, err = c.DoRequest("GET", "balance", nil)
	return err
}

func (s *StripeConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "list-invoices":
		return s.listInvoices(params)
	case "create-invoice":
		return s.createInvoice(params)
	case "list-customers":
		return s.listCustomers(params)
	case "create-customer":
		return s.createCustomer(params)
	case "list-payments":
		return s.listPayments(params)
	case "list-subscriptions":
		return s.listSubscriptions(params)
	case "get-balance":
		return s.getBalance()
	default:
		return nil, fmt.Errorf("stripe: unknown action %q, available: %v", action, s.Actions())
	}
}

// Stripe uses form-encoded POST bodies, not JSON.
func (s *StripeConnector) stripePost(path string, params map[string]interface{}) (map[string]interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	form := make(map[string]string)
	for k, v := range params {
		form[k] = fmt.Sprintf("%v", v)
	}
	return c.DoFormRequest(path, form)
}

func (s *StripeConnector) listInvoices(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	path := "invoices"
	q := url.Values{}
	if limit := pStr(params, "limit"); limit != "" {
		q.Set("limit", limit)
	}
	if customer := pStr(params, "customer"); customer != "" {
		q.Set("customer", customer)
	}
	if status := pStr(params, "status"); status != "" {
		q.Set("status", status)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (s *StripeConnector) createInvoice(params map[string]interface{}) (interface{}, error) {
	customer := pStr(params, "customer")
	if customer == "" {
		return nil, fmt.Errorf("create-invoice requires 'customer' param (Stripe customer ID)")
	}
	return s.stripePost("invoices", params)
}

func (s *StripeConnector) listCustomers(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	path := "customers"
	q := url.Values{}
	if limit := pStr(params, "limit"); limit != "" {
		q.Set("limit", limit)
	}
	if email := pStr(params, "email"); email != "" {
		q.Set("email", email)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (s *StripeConnector) createCustomer(params map[string]interface{}) (interface{}, error) {
	return s.stripePost("customers", params)
}

func (s *StripeConnector) listPayments(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	path := "payment_intents"
	q := url.Values{}
	if limit := pStr(params, "limit"); limit != "" {
		q.Set("limit", limit)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (s *StripeConnector) listSubscriptions(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	path := "subscriptions"
	q := url.Values{}
	if limit := pStr(params, "limit"); limit != "" {
		q.Set("limit", limit)
	}
	if status := pStr(params, "status"); status != "" {
		q.Set("status", status)
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (s *StripeConnector) getBalance() (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("GET", "balance", nil)
}

// ────────────────────────────────────────────────────────────────────
// VonageConnector: alternative phone/SMS provider.
// ────────────────────────────────────────────────────────────────────

type VonageConnector struct {
	registry *ConnectorRegistry
}

func NewVonageConnector(registry *ConnectorRegistry) *VonageConnector {
	return &VonageConnector{registry: registry}
}

func (v *VonageConnector) Name() string     { return "vonage" }
func (v *VonageConnector) Category() string  { return "phone" }

func (v *VonageConnector) Actions() []string {
	return []string{"send-sms", "list-numbers"}
}

func (v *VonageConnector) TestConnection() error {
	creds, err := v.registry.GetConnectorCreds("vonage")
	if err != nil {
		return err
	}
	apiKey := creds.Fields["api_key"]
	apiSecret := creds.Fields["api_secret"]
	if apiKey == "" || apiSecret == "" {
		return fmt.Errorf("vonage credentials incomplete: need api_key and api_secret")
	}
	c := NewConnectorHTTPClient("https://rest.nexmo.com", nil)
	_, err = c.DoRequest("GET",
		fmt.Sprintf("account/numbers?api_key=%s&api_secret=%s", apiKey, apiSecret), nil)
	return err
}

func (v *VonageConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	creds, err := v.registry.GetConnectorCreds("vonage")
	if err != nil {
		return nil, err
	}
	apiKey := creds.Fields["api_key"]
	apiSecret := creds.Fields["api_secret"]

	switch action {
	case "send-sms":
		to := pStr(params, "to")
		body := pStr(params, "body")
		from := pStr(params, "from")
		if from == "" {
			from = creds.Fields["from_number"]
		}
		if to == "" || body == "" {
			return nil, fmt.Errorf("send-sms requires 'to' and 'body'")
		}
		c := NewConnectorHTTPClient("https://rest.nexmo.com", nil)
		return c.DoRequest("POST", "sms/json", map[string]interface{}{
			"api_key":    apiKey,
			"api_secret": apiSecret,
			"to":         to,
			"from":       from,
			"text":       body,
		})
	case "list-numbers":
		c := NewConnectorHTTPClient("https://rest.nexmo.com", nil)
		return c.DoRequest("GET",
			fmt.Sprintf("account/numbers?api_key=%s&api_secret=%s", apiKey, apiSecret), nil)
	default:
		return nil, fmt.Errorf("vonage: unknown action %q", action)
	}
}

// ────────────────────────────────────────────────────────────────────
// PlivoConnector: another phone/SMS alternative.
// ────────────────────────────────────────────────────────────────────

type PlivoConnector struct {
	registry *ConnectorRegistry
}

func NewPlivoConnector(registry *ConnectorRegistry) *PlivoConnector {
	return &PlivoConnector{registry: registry}
}

func (p *PlivoConnector) Name() string     { return "plivo" }
func (p *PlivoConnector) Category() string  { return "phone" }

func (p *PlivoConnector) Actions() []string {
	return []string{"send-sms", "list-numbers", "make-call"}
}

func (p *PlivoConnector) client() (*ConnectorHTTPClient, string, error) {
	creds, err := p.registry.GetConnectorCreds("plivo")
	if err != nil {
		return nil, "", err
	}
	authID := creds.Fields["auth_id"]
	authToken := creds.Fields["auth_token"]
	if authID == "" || authToken == "" {
		return nil, "", fmt.Errorf("plivo credentials incomplete: need auth_id and auth_token")
	}
	auth := "Basic " + base64Encode(authID+":"+authToken)
	c := NewConnectorHTTPClient(
		fmt.Sprintf("https://api.plivo.com/v1/Account/%s", authID),
		map[string]string{"Authorization": auth},
	)
	return c, authID, nil
}

func (p *PlivoConnector) TestConnection() error {
	c, _, err := p.client()
	if err != nil {
		return err
	}
	_, err = c.DoRequest("GET", "/", nil)
	return err
}

func (p *PlivoConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	c, _, err := p.client()
	if err != nil {
		return nil, err
	}

	switch action {
	case "send-sms":
		src := pStr(params, "from")
		dst := pStr(params, "to")
		text := pStr(params, "body")
		if dst == "" || text == "" {
			return nil, fmt.Errorf("send-sms requires 'to' and 'body'")
		}
		return c.DoRequest("POST", "Message/", map[string]interface{}{
			"src":  src,
			"dst":  dst,
			"text": text,
		})
	case "list-numbers":
		return c.DoRequest("GET", "Number/", nil)
	case "make-call":
		from := pStr(params, "from")
		to := pStr(params, "to")
		answerURL := pStr(params, "url")
		if to == "" || from == "" {
			return nil, fmt.Errorf("make-call requires 'to' and 'from'")
		}
		if answerURL == "" {
			answerURL = "https://s3.amazonaws.com/plivocloud/Phlo/bbe60064-bd0b-49ab-841b-4e90f8e05d16.xml"
		}
		return c.DoRequest("POST", "Call/", map[string]interface{}{
			"from":       from,
			"to":         to,
			"answer_url": answerURL,
		})
	default:
		return nil, fmt.Errorf("plivo: unknown action %q", action)
	}
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
