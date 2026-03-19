package api

import (
	"fmt"
	"net/url"
)

// ────────────────────────────────────────────────────────────────────
// Additional Payment Gateway Connectors:
// PayPal, Square (Cash App), Authorize.net, GoCardless
// ────────────────────────────────────────────────────────────────────

// ── PayPalConnector (also covers Venmo via PayPal API) ─────────────

type PayPalConnector struct {
	registry *ConnectorRegistry
}

func NewPayPalConnector(registry *ConnectorRegistry) *PayPalConnector {
	return &PayPalConnector{registry: registry}
}

func (p *PayPalConnector) Name() string     { return "paypal" }
func (p *PayPalConnector) Category() string  { return "payments" }

func (p *PayPalConnector) Actions() []string {
	return []string{
		"list-transactions",
		"create-invoice",
		"send-invoice",
		"list-invoices",
		"create-payment",
		"get-balance",
	}
}

func (p *PayPalConnector) client() (*ConnectorHTTPClient, error) {
	creds, err := p.registry.GetConnectorCreds("paypal")
	if err != nil {
		return nil, fmt.Errorf("PayPal credentials not configured: %w", err)
	}
	// PayPal uses OAuth2 access token. User stores the access token directly
	// or client_id + secret for token generation.
	token := creds.Fields["access_token"]
	if token == "" {
		token = creds.Fields["api_key"]
	}
	if token == "" {
		return nil, fmt.Errorf("PayPal credentials incomplete: need access_token")
	}
	// Use sandbox or live based on credential
	baseURL := "https://api-m.paypal.com/v2"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://api-m.sandbox.paypal.com/v2"
	}
	return NewConnectorHTTPClient(baseURL, map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}), nil
}

func (p *PayPalConnector) TestConnection() error {
	// PayPal v1 endpoint for balance check
	creds, err := p.registry.GetConnectorCreds("paypal")
	if err != nil {
		return err
	}
	token := creds.Fields["access_token"]
	if token == "" {
		token = creds.Fields["api_key"]
	}
	baseURL := "https://api-m.paypal.com/v1"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://api-m.sandbox.paypal.com/v1"
	}
	c := NewConnectorHTTPClient(baseURL, map[string]string{
		"Authorization": "Bearer " + token,
	})
	_, err = c.DoRequest("GET", "reporting/balances", nil)
	return err
}

func (p *PayPalConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "list-transactions":
		return p.listTransactions(params)
	case "create-invoice":
		return p.createInvoice(params)
	case "send-invoice":
		return p.sendInvoice(params)
	case "list-invoices":
		return p.listInvoices(params)
	case "create-payment":
		return p.createPayment(params)
	case "get-balance":
		return p.getBalance()
	default:
		return nil, fmt.Errorf("paypal: unknown action %q", action)
	}
}

func (p *PayPalConnector) listTransactions(params map[string]interface{}) (interface{}, error) {
	creds, _ := p.registry.GetConnectorCreds("paypal")
	token := creds.Fields["access_token"]
	if token == "" {
		token = creds.Fields["api_key"]
	}
	baseURL := "https://api-m.paypal.com/v1"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://api-m.sandbox.paypal.com/v1"
	}
	c := NewConnectorHTTPClient(baseURL, map[string]string{"Authorization": "Bearer " + token})
	q := url.Values{}
	q.Set("start_date", pStr(params, "start_date"))
	q.Set("end_date", pStr(params, "end_date"))
	if q.Get("start_date") == "" {
		q.Set("start_date", "2024-01-01T00:00:00-0700")
	}
	if q.Get("end_date") == "" {
		q.Set("end_date", "2026-12-31T23:59:59-0700")
	}
	return c.DoRequest("GET", "reporting/transactions?"+q.Encode(), nil)
}

func (p *PayPalConnector) createInvoice(params map[string]interface{}) (interface{}, error) {
	c, err := p.client()
	if err != nil {
		return nil, err
	}
	// PayPal invoicing is under v2
	return c.DoRequest("POST", "invoicing/invoices", params)
}

func (p *PayPalConnector) sendInvoice(params map[string]interface{}) (interface{}, error) {
	c, err := p.client()
	if err != nil {
		return nil, err
	}
	invoiceID := pStr(params, "invoice_id")
	if invoiceID == "" {
		return nil, fmt.Errorf("send-invoice requires 'invoice_id'")
	}
	return c.DoRequest("POST", "invoicing/invoices/"+invoiceID+"/send", params)
}

func (p *PayPalConnector) listInvoices(params map[string]interface{}) (interface{}, error) {
	c, err := p.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("POST", "invoicing/search-invoices", map[string]interface{}{
		"page": 1, "page_size": 20,
	})
}

func (p *PayPalConnector) createPayment(params map[string]interface{}) (interface{}, error) {
	c, err := p.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("POST", "checkout/orders", params)
}

func (p *PayPalConnector) getBalance() (interface{}, error) {
	creds, err := p.registry.GetConnectorCreds("paypal")
	if err != nil {
		return nil, err
	}
	token := creds.Fields["access_token"]
	if token == "" {
		token = creds.Fields["api_key"]
	}
	baseURL := "https://api-m.paypal.com/v1"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://api-m.sandbox.paypal.com/v1"
	}
	c := NewConnectorHTTPClient(baseURL, map[string]string{"Authorization": "Bearer " + token})
	return c.DoRequest("GET", "reporting/balances", nil)
}

// ── SquareConnector (also covers Cash App via Square API) ──────────

type SquareConnector struct {
	registry *ConnectorRegistry
}

func NewSquareConnector(registry *ConnectorRegistry) *SquareConnector {
	return &SquareConnector{registry: registry}
}

func (s *SquareConnector) Name() string     { return "square" }
func (s *SquareConnector) Category() string  { return "payments" }

func (s *SquareConnector) Actions() []string {
	return []string{
		"list-payments",
		"create-payment",
		"list-customers",
		"create-customer",
		"list-invoices",
		"create-invoice",
		"list-orders",
		"get-balance",
	}
}

func (s *SquareConnector) client() (*ConnectorHTTPClient, error) {
	creds, err := s.registry.GetConnectorCreds("square")
	if err != nil {
		return nil, fmt.Errorf("Square credentials not configured: %w", err)
	}
	token := creds.Fields["access_token"]
	if token == "" {
		token = creds.Fields["api_key"]
	}
	if token == "" {
		return nil, fmt.Errorf("Square credentials incomplete: need access_token")
	}
	baseURL := "https://connect.squareup.com/v2"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://connect.squareupsandbox.com/v2"
	}
	return NewConnectorHTTPClient(baseURL, map[string]string{
		"Authorization":  "Bearer " + token,
		"Square-Version": "2024-01-18",
	}), nil
}

func (s *SquareConnector) TestConnection() error {
	c, err := s.client()
	if err != nil {
		return err
	}
	_, err = c.DoRequest("GET", "locations", nil)
	return err
}

func (s *SquareConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "list-payments":
		return s.listPayments(params)
	case "create-payment":
		return s.createPayment(params)
	case "list-customers":
		return s.listCustomers(params)
	case "create-customer":
		return s.createCustomer(params)
	case "list-invoices":
		return s.listInvoices(params)
	case "create-invoice":
		return s.createInvoice(params)
	case "list-orders":
		return s.listOrders(params)
	case "get-balance":
		return s.getBalance()
	default:
		return nil, fmt.Errorf("square: unknown action %q", action)
	}
}

func (s *SquareConnector) listPayments(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("GET", "payments", nil)
}

func (s *SquareConnector) createPayment(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("POST", "payments", params)
}

func (s *SquareConnector) listCustomers(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("GET", "customers", nil)
}

func (s *SquareConnector) createCustomer(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("POST", "customers", params)
}

func (s *SquareConnector) listInvoices(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	creds, _ := s.registry.GetConnectorCreds("square")
	locID := creds.Fields["location_id"]
	path := "invoices"
	if locID != "" {
		path += "?location_id=" + url.QueryEscape(locID)
	}
	return c.DoRequest("GET", path, nil)
}

func (s *SquareConnector) createInvoice(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	return c.DoRequest("POST", "invoices", params)
}

func (s *SquareConnector) listOrders(params map[string]interface{}) (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	creds, _ := s.registry.GetConnectorCreds("square")
	locID := creds.Fields["location_id"]
	body := map[string]interface{}{}
	if locID != "" {
		body["location_ids"] = []string{locID}
	}
	return c.DoRequest("POST", "orders/search", body)
}

func (s *SquareConnector) getBalance() (interface{}, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	// Square doesn't have a direct balance endpoint; list locations shows currency config.
	return c.DoRequest("GET", "locations", nil)
}

// ── AuthorizeNetConnector ──────────────────────────────────────────

type AuthorizeNetConnector struct {
	registry *ConnectorRegistry
}

func NewAuthorizeNetConnector(registry *ConnectorRegistry) *AuthorizeNetConnector {
	return &AuthorizeNetConnector{registry: registry}
}

func (a *AuthorizeNetConnector) Name() string     { return "authorizenet" }
func (a *AuthorizeNetConnector) Category() string  { return "payments" }

func (a *AuthorizeNetConnector) Actions() []string {
	return []string{
		"charge-card",
		"list-transactions",
		"get-transaction",
	}
}

func (a *AuthorizeNetConnector) getCreds() (string, string, string, error) {
	creds, err := a.registry.GetConnectorCreds("authorizenet")
	if err != nil {
		creds, err = a.registry.GetConnectorCreds("authorize.net")
		if err != nil {
			return "", "", "", fmt.Errorf("Authorize.net credentials not configured: %w", err)
		}
	}
	loginID := creds.Fields["api_login_id"]
	transKey := creds.Fields["transaction_key"]
	if loginID == "" || transKey == "" {
		return "", "", "", fmt.Errorf("Authorize.net credentials incomplete: need api_login_id and transaction_key")
	}
	baseURL := "https://api.authorize.net/xml/v1/request.api"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://apitest.authorize.net/xml/v1/request.api"
	}
	return loginID, transKey, baseURL, nil
}

func (a *AuthorizeNetConnector) TestConnection() error {
	loginID, transKey, baseURL, err := a.getCreds()
	if err != nil {
		return err
	}
	c := NewConnectorHTTPClient(baseURL, nil)
	_, err = c.DoRequest("POST", "", map[string]interface{}{
		"getSettledBatchListRequest": map[string]interface{}{
			"merchantAuthentication": map[string]string{
				"name":           loginID,
				"transactionKey": transKey,
			},
		},
	})
	return err
}

func (a *AuthorizeNetConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	loginID, transKey, baseURL, err := a.getCreds()
	if err != nil {
		return nil, err
	}
	c := NewConnectorHTTPClient(baseURL, nil)
	auth := map[string]string{
		"name":           loginID,
		"transactionKey": transKey,
	}

	switch action {
	case "charge-card":
		return c.DoRequest("POST", "", map[string]interface{}{
			"createTransactionRequest": map[string]interface{}{
				"merchantAuthentication": auth,
				"transactionRequest":     params,
			},
		})
	case "list-transactions":
		return c.DoRequest("POST", "", map[string]interface{}{
			"getTransactionListRequest": map[string]interface{}{
				"merchantAuthentication": auth,
				"batchId":                pStr(params, "batch_id"),
			},
		})
	case "get-transaction":
		return c.DoRequest("POST", "", map[string]interface{}{
			"getTransactionDetailsRequest": map[string]interface{}{
				"merchantAuthentication": auth,
				"transId":               pStr(params, "transaction_id"),
			},
		})
	default:
		return nil, fmt.Errorf("authorizenet: unknown action %q", action)
	}
}

// ── GoCardlessConnector (ACH / Direct Debit) ──────────────────────

type GoCardlessConnector struct {
	registry *ConnectorRegistry
}

func NewGoCardlessConnector(registry *ConnectorRegistry) *GoCardlessConnector {
	return &GoCardlessConnector{registry: registry}
}

func (g *GoCardlessConnector) Name() string     { return "gocardless" }
func (g *GoCardlessConnector) Category() string  { return "payments" }

func (g *GoCardlessConnector) Actions() []string {
	return []string{
		"list-payments",
		"create-payment",
		"list-customers",
		"create-customer",
		"list-mandates",
		"list-subscriptions",
	}
}

func (g *GoCardlessConnector) client() (*ConnectorHTTPClient, error) {
	creds, err := g.registry.GetConnectorCreds("gocardless")
	if err != nil {
		return nil, fmt.Errorf("GoCardless credentials not configured: %w", err)
	}
	token := creds.Fields["access_token"]
	if token == "" {
		token = creds.Fields["api_key"]
	}
	if token == "" {
		return nil, fmt.Errorf("GoCardless credentials incomplete: need access_token")
	}
	baseURL := "https://api.gocardless.com"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://api-sandbox.gocardless.com"
	}
	return NewConnectorHTTPClient(baseURL, map[string]string{
		"Authorization":    "Bearer " + token,
		"GoCardless-Version": "2015-07-06",
	}), nil
}

func (g *GoCardlessConnector) TestConnection() error {
	c, err := g.client()
	if err != nil {
		return err
	}
	_, err = c.DoRequest("GET", "customers?limit=1", nil)
	return err
}

func (g *GoCardlessConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	c, err := g.client()
	if err != nil {
		return nil, err
	}
	switch action {
	case "list-payments":
		return c.DoRequest("GET", "payments", nil)
	case "create-payment":
		return c.DoRequest("POST", "payments", map[string]interface{}{"payments": params})
	case "list-customers":
		return c.DoRequest("GET", "customers", nil)
	case "create-customer":
		return c.DoRequest("POST", "customers", map[string]interface{}{"customers": params})
	case "list-mandates":
		return c.DoRequest("GET", "mandates", nil)
	case "list-subscriptions":
		return c.DoRequest("GET", "subscriptions", nil)
	default:
		return nil, fmt.Errorf("gocardless: unknown action %q", action)
	}
}

// ── JPMCZelleConnector (Zelle via J.P. Morgan Global Payments API) ─

type JPMCZelleConnector struct {
	registry *ConnectorRegistry
}

func NewJPMCZelleConnector(registry *ConnectorRegistry) *JPMCZelleConnector {
	return &JPMCZelleConnector{registry: registry}
}

func (z *JPMCZelleConnector) Name() string     { return "zelle" }
func (z *JPMCZelleConnector) Category() string  { return "payments" }

func (z *JPMCZelleConnector) Actions() []string {
	return []string{
		"send-payment",
		"get-status",
	}
}

func (z *JPMCZelleConnector) client() (*ConnectorHTTPClient, error) {
	creds, err := z.registry.GetConnectorCreds("zelle")
	if err != nil {
		creds, err = z.registry.GetConnectorCreds("jpmc")
		if err != nil {
			return nil, fmt.Errorf("JPMC/Zelle credentials not configured: %w", err)
		}
	}
	token := creds.Fields["access_token"]
	if token == "" {
		token = creds.Fields["api_key"]
	}
	if token == "" {
		return nil, fmt.Errorf("JPMC/Zelle credentials incomplete: need access_token")
	}
	// JPMC Global Payments base URL
	baseURL := "https://api.payments.jpmorgan.com/tsapi/v1"
	if creds.Fields["sandbox"] == "true" {
		baseURL = "https://api-mock.payments.jpmorgan.com/tsapi/v1"
	}
	return NewConnectorHTTPClient(baseURL, map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
	}), nil
}

func (z *JPMCZelleConnector) TestConnection() error {
	_, err := z.client()
	return err
}

func (z *JPMCZelleConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	c, err := z.client()
	if err != nil {
		return nil, err
	}
	switch action {
	case "send-payment":
		return z.sendPayment(c, params)
	case "get-status":
		return z.getStatus(c, params)
	default:
		return nil, fmt.Errorf("zelle: unknown action %q", action)
	}
}

// sendPayment creates a Zelle disbursement via JPMC Global Payments API.
// Required params: creditor_name, creditor_token (email or phone), amount, debtor_account
// Optional: creditor_token_type (EMAL or PHON, default EMAL), end_to_end_id
func (z *JPMCZelleConnector) sendPayment(c *ConnectorHTTPClient, params map[string]interface{}) (interface{}, error) {
	creditorName := pStr(params, "creditor_name")
	creditorToken := pStr(params, "creditor_token")
	amount := pStr(params, "amount")
	debtorAccount := pStr(params, "debtor_account")
	debtorName := pStr(params, "debtor_name")
	endToEndID := pStr(params, "end_to_end_id")
	tokenType := pStr(params, "creditor_token_type")

	if creditorName == "" || creditorToken == "" || amount == "" {
		return nil, fmt.Errorf("send-payment requires creditor_name, creditor_token, amount")
	}
	if debtorAccount == "" {
		return nil, fmt.Errorf("send-payment requires debtor_account")
	}
	if tokenType == "" {
		// Auto-detect: if contains @ it's email, otherwise phone
		if len(creditorToken) > 0 && creditorToken[0] == '+' {
			tokenType = "PHON"
		} else {
			tokenType = "EMAL"
		}
	}
	if endToEndID == "" {
		endToEndID = fmt.Sprintf("ZELLE%d", len(creditorToken)*1000+len(amount)*100)
	}
	if debtorName == "" {
		debtorName = "Account Holder"
	}

	payload := map[string]interface{}{
		"payments": map[string]interface{}{
			"requestedExecutionDate": pStr(params, "execution_date"),
			"paymentIdentifiers": map[string]string{
				"endToEndId": endToEndID,
			},
			"paymentCurrency": "USD",
			"paymentAmount":   amount,
			"transferType":    "CREDIT",
			"debtor": map[string]interface{}{
				"debtorName": debtorName,
				"debtorAccount": map[string]string{
					"accountId": debtorAccount,
				},
			},
			"debtorAgent": map[string]interface{}{
				"financialInstitutionId": map[string]string{
					"bic": "CHASUS33",
				},
			},
			"creditor": map[string]interface{}{
				"creditorName": creditorName,
				"creditorAccount": map[string]interface{}{
					"accountType":                "ZELLE",
					"alternateAccountIdentifier": creditorToken,
					"schemeName": map[string]string{
						"proprietary": tokenType,
					},
				},
			},
		},
	}

	return c.DoRequest("POST", "payments", payload)
}

// getStatus retrieves the status of a Zelle disbursement by endToEndId.
func (z *JPMCZelleConnector) getStatus(c *ConnectorHTTPClient, params map[string]interface{}) (interface{}, error) {
	endToEndID := pStr(params, "end_to_end_id")
	firmRootID := pStr(params, "firm_root_id")
	if endToEndID == "" && firmRootID == "" {
		return nil, fmt.Errorf("get-status requires end_to_end_id or firm_root_id")
	}
	path := "payments/status"
	if firmRootID != "" {
		path += "?firmRootId=" + url.QueryEscape(firmRootID)
	} else {
		path += "?endToEndId=" + url.QueryEscape(endToEndID)
	}
	return c.DoRequest("GET", path, nil)
}
