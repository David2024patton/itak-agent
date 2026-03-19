package api

import (
	"fmt"
	"net/url"
)

// ────────────────────────────────────────────────────────────────────
// GHLConnector: CRM integration via GoHighLevel API v2.
//
// What: Contacts, conversations, opportunities, pipelines via GHL.
// Why:  Powers CRM features (Conversations, Dashboard, Contacts pages).
// How:  Uses GHL API v2 (services.leadconnectorhq.com).
//       Auth: Bearer (API Key or OAuth token).
//       Credential fields: api_key, location_id.
// ────────────────────────────────────────────────────────────────────

type GHLConnector struct {
	registry *ConnectorRegistry
}

func NewGHLConnector(registry *ConnectorRegistry) *GHLConnector {
	return &GHLConnector{registry: registry}
}

func (g *GHLConnector) Name() string     { return "ghl" }
func (g *GHLConnector) Category() string  { return "crm" }

func (g *GHLConnector) Actions() []string {
	return []string{
		"list-contacts",
		"create-contact",
		"search-contacts",
		"list-conversations",
		"send-message",
		"list-opportunities",
		"list-pipelines",
		"list-calendars",
		"list-campaigns",
	}
}

func (g *GHLConnector) client() (*ConnectorHTTPClient, string, error) {
	creds, err := g.registry.GetConnectorCreds("ghl")
	if err != nil {
		creds, err = g.registry.GetConnectorCreds("gohighlevel")
		if err != nil {
			return nil, "", fmt.Errorf("GHL credentials not configured: %w", err)
		}
	}
	apiKey := creds.Fields["api_key"]
	if apiKey == "" {
		apiKey = creds.Fields["access_token"]
	}
	locationID := creds.Fields["location_id"]
	if apiKey == "" {
		return nil, "", fmt.Errorf("GHL credentials incomplete: need api_key and location_id")
	}

	c := NewConnectorHTTPClient("https://services.leadconnectorhq.com", map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Version":       "2021-07-28",
	})
	return c, locationID, nil
}

func (g *GHLConnector) TestConnection() error {
	c, locationID, err := g.client()
	if err != nil {
		return err
	}
	path := "contacts/"
	if locationID != "" {
		path += "?locationId=" + url.QueryEscape(locationID) + "&limit=1"
	} else {
		path += "?limit=1"
	}
	_, err = c.DoRequest("GET", path, nil)
	return err
}

func (g *GHLConnector) Do(action string, params map[string]interface{}) (interface{}, error) {
	switch action {
	case "list-contacts":
		return g.listContacts(params)
	case "create-contact":
		return g.createContact(params)
	case "search-contacts":
		return g.searchContacts(params)
	case "list-conversations":
		return g.listConversations(params)
	case "send-message":
		return g.sendMessage(params)
	case "list-opportunities":
		return g.listOpportunities(params)
	case "list-pipelines":
		return g.listPipelines(params)
	case "list-calendars":
		return g.listCalendars(params)
	case "list-campaigns":
		return g.listCampaigns(params)
	default:
		return nil, fmt.Errorf("ghl: unknown action %q, available: %v", action, g.Actions())
	}
}

func (g *GHLConnector) locationParam(params map[string]interface{}) string {
	locID := pStr(params, "location_id")
	if locID == "" {
		_, locID, _ = g.client()
	}
	return locID
}

func (g *GHLConnector) listContacts(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	q := url.Values{}
	if locID != "" {
		q.Set("locationId", locID)
	}
	if limit := pStr(params, "limit"); limit != "" {
		q.Set("limit", limit)
	}
	path := "contacts/"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (g *GHLConnector) createContact(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	if locID != "" {
		params["locationId"] = locID
	}
	return c.DoRequest("POST", "contacts/", params)
}

func (g *GHLConnector) searchContacts(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	q := url.Values{}
	if locID != "" {
		q.Set("locationId", locID)
	}
	if query := pStr(params, "query"); query != "" {
		q.Set("query", query)
	}
	path := "contacts/search"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (g *GHLConnector) listConversations(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	q := url.Values{}
	if locID != "" {
		q.Set("locationId", locID)
	}
	path := "conversations/"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (g *GHLConnector) sendMessage(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	conversationID := pStr(params, "conversation_id")
	if conversationID == "" {
		return nil, fmt.Errorf("send-message requires 'conversation_id'")
	}
	body := map[string]interface{}{
		"type": pStr(params, "type"),
		"body": pStr(params, "body"),
	}
	if body["type"] == "" {
		body["type"] = "SMS"
	}
	return c.DoRequest("POST", "conversations/messages", body)
}

func (g *GHLConnector) listOpportunities(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	q := url.Values{}
	if locID != "" {
		q.Set("location_id", locID)
	}
	path := "opportunities/search"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (g *GHLConnector) listPipelines(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	q := url.Values{}
	if locID != "" {
		q.Set("locationId", locID)
	}
	path := "opportunities/pipelines"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (g *GHLConnector) listCalendars(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	q := url.Values{}
	if locID != "" {
		q.Set("locationId", locID)
	}
	path := "calendars/"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}

func (g *GHLConnector) listCampaigns(params map[string]interface{}) (interface{}, error) {
	c, _, err := g.client()
	if err != nil {
		return nil, err
	}
	locID := g.locationParam(params)
	q := url.Values{}
	if locID != "" {
		q.Set("locationId", locID)
	}
	path := "campaigns/"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.DoRequest("GET", path, nil)
}
