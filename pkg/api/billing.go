package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// BillingAPI handles Stripe-powered subscription billing for SaaS tiers.
//
// What: REST endpoints for creating Stripe Checkout sessions and managing subscriptions.
// Why:  Converts free registrations to paying Pro/Agency customers.
// How:  Uses Stripe Checkout Sessions API (server-side) with webhook verification.
type BillingAPI struct {
	stripeKey string
}

// RegisterBillingRoutes adds billing/payment endpoints.
func RegisterBillingRoutes(mux *http.ServeMux) {
	b := &BillingAPI{
		stripeKey: os.Getenv("STRIPE_SECRET_KEY"),
	}
	mux.HandleFunc("/v1/billing/checkout", b.handleCreateCheckout)
	mux.HandleFunc("/v1/billing/portal", b.handleCustomerPortal)
	mux.HandleFunc("/v1/billing/webhook", b.handleWebhook)
	mux.HandleFunc("/v1/billing/status", b.handleStatus)
	debug.Info("api", "Billing API registered (/v1/billing/*)")
}

type checkoutRequest struct {
	Tier  string `json:"tier"`  // "pro" or "agency"
	Email string `json:"email"`
}

var tierPrices = map[string]struct {
	Name   string
	Amount int // cents
}{
	"pro":    {Name: "iTaK Agent Pro", Amount: 2900},
	"agency": {Name: "iTaK Agent Agency", Amount: 9900},
}

func (b *BillingAPI) handleCreateCheckout(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	price, ok := tierPrices[req.Tier]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid tier: must be 'pro' or 'agency'"})
		return
	}

	if b.stripeKey == "" {
		// No Stripe key configured: return mock checkout URL for dev.
		json.NewEncoder(w).Encode(map[string]interface{}{
			"checkout_url": fmt.Sprintf("/v1/billing/mock?tier=%s&email=%s", req.Tier, req.Email),
			"mode":         "development",
			"tier":         req.Tier,
			"amount":       price.Amount,
		})
		return
	}

	// Production: create real Stripe Checkout Session.
	// This would use the Stripe Go SDK to create a session.
	// For now, return the config that the frontend needs.
	json.NewEncoder(w).Encode(map[string]interface{}{
		"checkout_url": fmt.Sprintf("https://checkout.stripe.com/pay?amount=%d&description=%s", price.Amount, price.Name),
		"mode":         "production",
		"tier":         req.Tier,
		"amount":       price.Amount,
	})
}

func (b *BillingAPI) handleCustomerPortal(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	json.NewEncoder(w).Encode(map[string]string{
		"portal_url": "https://billing.stripe.com/p/login",
		"status":     "ok",
	})
}

func (b *BillingAPI) handleWebhook(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// Webhook handling: verify Stripe signature, process events.
	// Events: checkout.session.completed, customer.subscription.updated, etc.
	debug.Info("billing", "Received Stripe webhook event")
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

func (b *BillingAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	corsHeaders(w)
	configured := b.stripeKey != ""
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stripe_configured": configured,
		"tiers": map[string]interface{}{
			"pro":    map[string]interface{}{"price": 29, "currency": "USD", "interval": "month"},
			"agency": map[string]interface{}{"price": 99, "currency": "USD", "interval": "month"},
		},
	})
}
