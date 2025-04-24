package models

import (
	"time"

	"gostripe/storage"

	"github.com/gobuffalo/uuid"
	"github.com/pkg/errors"
)

// Customer represents a customer in our system
type Customer struct {
	ID        uuid.UUID `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	StripeID  string    `json:"stripe_id" db:"stripe_id"`
	Email     string    `json:"email" db:"email"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// TableName returns the table name for the Customer model
func (Customer) TableName() string {
	return "stripe_customers"
}

// FindCustomerByUserID finds a customer by user ID
func FindCustomerByUserID(conn *storage.Connection, userID uuid.UUID) (*Customer, error) {
	customer := &Customer{}
	if err := conn.Where("user_id = ?", userID).First(customer); err != nil {
		if errors.Cause(err).Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return customer, nil
}

// FindCustomerByStripeID finds a customer by Stripe ID
func FindCustomerByStripeID(conn *storage.Connection, stripeID string) (*Customer, error) {
	customer := &Customer{}
	if err := conn.Where("stripe_id = ?", stripeID).First(customer); err != nil {
		if errors.Cause(err).Error() == "sql: no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	return customer, nil
}

// CreateCustomer creates a new customer
func CreateCustomer(conn *storage.Connection, userID uuid.UUID, stripeID, email, name string) (*Customer, error) {
	customer := &Customer{
		ID:        uuid.Must(uuid.NewV4()),
		UserID:    userID,
		StripeID:  stripeID,
		Email:     email,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := conn.Create(customer); err != nil {
		return nil, err
	}

	return customer, nil
}
