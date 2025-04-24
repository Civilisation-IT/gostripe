package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"gostripe/models"

	"github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/webhook"
)

// CreateCheckoutSessionRequest represents a request to create a checkout session
type CreateCheckoutSessionRequest struct {
	PriceID      string `json:"price_id"`
	SuccessURL   string `json:"success_url"`
	CancelURL    string `json:"cancel_url"`
	CustomerName string `json:"customer_name"`
}

// CreateCheckoutSession creates a Stripe checkout session
func (a *API) CreateCheckoutSession(w http.ResponseWriter, r *http.Request) {
	// Parse request
	var req CreateCheckoutSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequestError(w, "Invalid request body")
		return
	}

	if req.PriceID == "" {
		badRequestError(w, "price_id is required")
		return
	}

	if req.SuccessURL == "" {
		badRequestError(w, "success_url is required")
		return
	}

	if req.CancelURL == "" {
		badRequestError(w, "cancel_url is required")
		return
	}

	// Utiliser directement l'ID de prix fourni par le frontend
	priceID := req.PriceID

	// Get user ID from context
	userID, err := getUserID(r.Context())
	if err != nil {
		internalServerError(w, r, "Failed to get user ID")
		return
	}

	// Get email from context
	email, ok := r.Context().Value("email").(string)
	if !ok || email == "" {
		internalServerError(w, r, "Failed to get email")
		return
	}

	// Check if customer already exists
	dbCustomer, err := models.FindCustomerByUserID(a.db, userID)
	if err != nil {
		internalServerError(w, r, "Failed to check customer")
		return
	}

	var stripeCustomerID string
	if dbCustomer == nil {
		// Create a real customer in Stripe using the Stripe API
		customerParams := &stripe.CustomerParams{
			Email: stripe.String(email),
			Name:  stripe.String(req.CustomerName),
		}
		// Use the customer package from Stripe
		stripeCustomer, err := customer.New(customerParams)
		if err != nil {
			logrus.WithError(err).Error("Failed to create Stripe customer")
			internalServerError(w, r, "Failed to create customer")
			return
		}
		stripeCustomerID = stripeCustomer.ID

		// Create customer in database
		dbCustomer, err = models.CreateCustomer(a.db, userID, stripeCustomerID, email, req.CustomerName)
		if err != nil {
			logrus.WithError(err).Error("Failed to create customer in database")
			internalServerError(w, r, "Failed to create customer")
			return
		}
	} else {
		stripeCustomerID = dbCustomer.StripeID
	}

	// Create checkout session
	params := &stripe.CheckoutSessionParams{
		Customer: stripe.String(stripeCustomerID),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL: stripe.String(req.SuccessURL),
		CancelURL:  stripe.String(req.CancelURL),
	}

	s, err := session.New(params)
	if err != nil {
		logrus.WithError(err).Error("Failed to create checkout session")
		internalServerError(w, r, "Failed to create checkout session")
		return
	}

	// Retourner l'ID de session et l'URL complète pour faciliter la redirection
	sendJSON(w, http.StatusOK, map[string]string{
		"session_id": s.ID,
		"url":        s.URL,
	})
}

// HandleWebhook handles Stripe webhooks
func (a *API) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	const MaxBodyBytes = int64(65536)
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logrus.WithError(err).Error("Failed to read webhook payload")
		badRequestError(w, "Failed to read payload")
		return
	}

	// Verify signature
	event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), a.config.Stripe.WebhookSecret)
	if err != nil {
		logrus.WithError(err).Error("Failed to verify webhook signature")
		badRequestError(w, "Failed to verify signature")
		return
	}

	// Handle event
	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			logrus.WithError(err).Error("Failed to parse checkout session")
			badRequestError(w, "Failed to parse checkout session")
			return
		}

		// Process the checkout session
		if err := a.handleCheckoutSessionCompleted(&session); err != nil {
			logrus.WithError(err).Error("Failed to handle checkout session completed")
			internalServerError(w, r, "Failed to handle checkout session")
			return
		}

	case "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		err := json.Unmarshal(event.Data.Raw, &sub)
		if err != nil {
			logrus.WithError(err).Error("Failed to parse subscription")
			badRequestError(w, "Failed to parse subscription")
			return
		}

		// Process the subscription
		if err := a.handleSubscriptionUpdated(&sub); err != nil {
			logrus.WithError(err).Error("Failed to handle subscription updated")
			internalServerError(w, r, "Failed to handle subscription")
			return
		}
	}

	sendJSON(w, http.StatusOK, map[string]string{
		"status": "success",
	})
}

// GetSubscriptionStatus gets the subscription status for a user
func (a *API) GetSubscriptionStatus(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID, err := getUserID(r.Context())
	if err != nil {
		internalServerError(w, r, "Failed to get user ID")
		return
	}

	// Get customer
	dbCustomer, err := models.FindCustomerByUserID(a.db, userID)
	if err != nil {
		internalServerError(w, r, "Failed to get customer")
		return
	}

	if dbCustomer == nil {
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"has_subscription": false,
		})
		return
	}

	// Get subscription
	subscription, err := models.FindActiveSubscriptionByCustomerID(a.db, dbCustomer.ID)
	if err != nil {
		internalServerError(w, r, "Failed to get subscription")
		return
	}

	if subscription == nil {
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"has_subscription": false,
		})
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"has_subscription":    true,
		"subscription_status": subscription.Status,
		"current_period_end":  subscription.CurrentPeriodEnd,
	})
}

// CancelSubscription cancels a subscription
func (a *API) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID, err := getUserID(r.Context())
	if err != nil {
		internalServerError(w, r, "Failed to get user ID")
		return
	}

	// Get customer
	dbCustomer, err := models.FindCustomerByUserID(a.db, userID)
	if err != nil {
		internalServerError(w, r, "Failed to get customer")
		return
	}

	if dbCustomer == nil {
		notFoundError(w, "Customer not found")
		return
	}

	// Get subscription
	subscription, err := models.FindActiveSubscriptionByCustomerID(a.db, dbCustomer.ID)
	if err != nil {
		internalServerError(w, r, "Failed to get subscription")
		return
	}

	if subscription == nil {
		notFoundError(w, "Subscription not found")
		return
	}

	// Cancel subscription in Stripe
	// Note: In a real implementation, you would use the Stripe API to cancel the subscription
	// For now, we'll just update our database
	// _, err = subscription.Cancel(subscription.StripeID, nil)
	if err != nil {
		logrus.WithError(err).Error("Failed to cancel subscription in Stripe")
		internalServerError(w, r, "Failed to cancel subscription")
		return
	}

	// Update subscription in database
	now := time.Now()
	subscription.Status = models.SubscriptionStatusCanceled
	subscription.CanceledAt = &now
	if err := models.UpdateSubscription(a.db, subscription); err != nil {
		logrus.WithError(err).Error("Failed to update subscription in database")
		internalServerError(w, r, "Failed to update subscription")
		return
	}

	sendJSON(w, http.StatusOK, map[string]string{
		"status": "canceled",
	})
}

// handleCheckoutSessionCompleted processes a completed checkout session
func (a *API) handleCheckoutSessionCompleted(session *stripe.CheckoutSession) error {
	// Get subscription
	// Note: In a real implementation, you would use the Stripe API to get the subscription
	// For now, we'll just create a mock subscription object
	sub := &stripe.Subscription{
		ID:               session.Subscription.ID,
		Status:           "active",
		CurrentPeriodEnd: time.Now().AddDate(0, 1, 0).Unix(), // 1 month from now
		Items: &stripe.SubscriptionItemList{
			Data: []*stripe.SubscriptionItem{
				{
					Price: &stripe.Price{
						ID: "price_123",
					},
				},
			},
		},
	}
	var err error
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Get customer
	dbCustomer, err := models.FindCustomerByStripeID(a.db, session.Customer.ID)
	if err != nil {
		return fmt.Errorf("failed to get customer: %w", err)
	}

	if dbCustomer == nil {
		return fmt.Errorf("customer not found: %s", session.Customer.ID)
	}

	// Check if subscription already exists
	existingSub, err := models.FindSubscriptionByStripeID(a.db, sub.ID)
	if err != nil {
		return fmt.Errorf("failed to check subscription: %w", err)
	}

	if existingSub != nil {
		// Update existing subscription
		existingSub.Status = models.SubscriptionStatus(sub.Status)
		existingSub.CurrentPeriodEnd = time.Unix(sub.CurrentPeriodEnd, 0)
		if err := models.UpdateSubscription(a.db, existingSub); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}
	} else {
		// Create new subscription
		_, err = models.CreateSubscription(
			a.db,
			dbCustomer.ID,
			sub.ID,
			sub.Items.Data[0].Price.ID,
			models.SubscriptionStatus(sub.Status),
			time.Unix(sub.CurrentPeriodEnd, 0),
		)
		if err != nil {
			return fmt.Errorf("failed to create subscription: %w", err)
		}
	}

	return nil
}

// handleSubscriptionUpdated processes an updated subscription
func (a *API) handleSubscriptionUpdated(sub *stripe.Subscription) error {
	// Get subscription from database
	subscription, err := models.FindSubscriptionByStripeID(a.db, sub.ID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if subscription == nil {
		// This might be a new subscription created outside of our system
		// Get customer
		dbCustomer, err := models.FindCustomerByStripeID(a.db, sub.Customer.ID)
		if err != nil {
			return fmt.Errorf("failed to get customer: %w", err)
		}

		if dbCustomer == nil {
			return fmt.Errorf("customer not found: %s", sub.Customer.ID)
		}

		// Create new subscription
		_, err = models.CreateSubscription(
			a.db,
			dbCustomer.ID,
			sub.ID,
			sub.Items.Data[0].Price.ID,
			models.SubscriptionStatus(sub.Status),
			time.Unix(sub.CurrentPeriodEnd, 0),
		)
		if err != nil {
			return fmt.Errorf("failed to create subscription: %w", err)
		}
	} else {
		// Update existing subscription
		subscription.Status = models.SubscriptionStatus(sub.Status)
		subscription.CurrentPeriodEnd = time.Unix(sub.CurrentPeriodEnd, 0)
		if sub.CanceledAt > 0 {
			canceledAt := time.Unix(sub.CanceledAt, 0)
			subscription.CanceledAt = &canceledAt
		}
		if err := models.UpdateSubscription(a.db, subscription); err != nil {
			return fmt.Errorf("failed to update subscription: %w", err)
		}
	}

	return nil
}
