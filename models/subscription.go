package models

import (
	"time"

	"gostripe/storage"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
)

// SubscriptionStatus represents the status of a subscription
type SubscriptionStatus string

const (
	// SubscriptionStatusActive represents an active subscription
	SubscriptionStatusActive SubscriptionStatus = "active"
	// SubscriptionStatusPastDue represents a past due subscription
	SubscriptionStatusPastDue SubscriptionStatus = "past_due"
	// SubscriptionStatusCanceled represents a canceled subscription
	SubscriptionStatusCanceled SubscriptionStatus = "canceled"
	// SubscriptionStatusIncomplete represents an incomplete subscription
	SubscriptionStatusIncomplete SubscriptionStatus = "incomplete"
	// SubscriptionStatusIncompleteExpired represents an incomplete expired subscription
	SubscriptionStatusIncompleteExpired SubscriptionStatus = "incomplete_expired"
	// SubscriptionStatusTrialing represents a trialing subscription
	SubscriptionStatusTrialing SubscriptionStatus = "trialing"
	// SubscriptionStatusUnpaid represents an unpaid subscription
	SubscriptionStatusUnpaid SubscriptionStatus = "unpaid"
)

// Subscription represents a subscription in our system
type Subscription struct {
	ID               uuid.UUID          `json:"id" db:"id"`
	CustomerID       uuid.UUID          `json:"customer_id" db:"customer_id"`
	StripeID         string             `json:"stripe_id" db:"stripe_id"`
	Status           SubscriptionStatus `json:"status" db:"status"`
	PriceID          string             `json:"price_id" db:"price_id"`
	CurrentPeriodEnd time.Time          `json:"current_period_end" db:"current_period_end"`
	CanceledAt       *time.Time         `json:"canceled_at,omitempty" db:"canceled_at"`
	CreatedAt        time.Time          `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time          `json:"updated_at" db:"updated_at"`
}

// TableName returns the table name for the Subscription model
func (Subscription) TableName() string {
	return "stripe_subscriptions"
}

// FindSubscriptionByStripeID finds a subscription by Stripe ID
func FindSubscriptionByStripeID(conn *storage.Connection, stripeID string) (*Subscription, error) {
	subscription := &Subscription{}
	if err := conn.Where("stripe_id = ?", stripeID).First(subscription); err != nil {
		if errors.Cause(err).Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return subscription, nil
}

// FindActiveSubscriptionByCustomerID finds an active subscription by customer ID
func FindActiveSubscriptionByCustomerID(conn *storage.Connection, customerID uuid.UUID) (*Subscription, error) {
	subscription := &Subscription{}
	if err := conn.Where("customer_id = ? AND status = ?", customerID, SubscriptionStatusActive).First(subscription); err != nil {
		if errors.Cause(err).Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return subscription, nil
}

// CreateSubscription creates a new subscription
func CreateSubscription(conn *storage.Connection, customerID uuid.UUID, stripeID, priceID string, status SubscriptionStatus, currentPeriodEnd time.Time) (*Subscription, error) {
	subscription := &Subscription{
		ID:               uuid.Must(uuid.NewV4()),
		CustomerID:       customerID,
		StripeID:         stripeID,
		Status:           status,
		PriceID:          priceID,
		CurrentPeriodEnd: currentPeriodEnd,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := conn.Create(subscription); err != nil {
		return nil, err
	}

	return subscription, nil
}

// UpdateSubscription updates a subscription
func UpdateSubscription(conn *storage.Connection, subscription *Subscription) error {
	subscription.UpdatedAt = time.Now()
	return conn.Update(subscription)
}
