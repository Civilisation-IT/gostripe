package api

import (
	"net/http"
	"time"

	"gostripe/models"

	"github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/price"
	"github.com/stripe/stripe-go/v72/sub"
)

// GetCustomerDetails gets detailed information about a customer and their subscription
func (a *API) GetCustomerDetails(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context
	userID, err := getUserID(r.Context())
	if err != nil {
		internalServerError(w, r, "Failed to get user ID")
		return
	}

	// Get customer from database
	dbCustomer, err := models.FindCustomerByUserID(a.db, userID)
	if err != nil {
		internalServerError(w, r, "Failed to get customer")
		return
	}

	if dbCustomer == nil {
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"has_customer": false,
			"has_subscription": false,
		})
		return
	}

	// Prepare response with customer details
	response := map[string]interface{}{
		"has_customer": true,
		"customer_id": dbCustomer.ID,
		"stripe_customer_id": dbCustomer.StripeID,
		"email": dbCustomer.Email,
		"name": dbCustomer.Name,
		"created_at": dbCustomer.CreatedAt,
		"updated_at": dbCustomer.UpdatedAt,
	}

	// Get subscription from database
	dbSubscription, err := models.FindActiveSubscriptionByCustomerID(a.db, dbCustomer.ID)
	if err != nil {
		internalServerError(w, r, "Failed to get subscription")
		return
	}

	// Add subscription details to response
	if dbSubscription != nil {
		response["has_subscription"] = true
		response["subscription_id"] = dbSubscription.ID
		response["stripe_subscription_id"] = dbSubscription.StripeID
		response["subscription_status"] = dbSubscription.Status
		response["price_id"] = dbSubscription.PriceID
		response["current_period_end"] = dbSubscription.CurrentPeriodEnd
		response["canceled_at"] = dbSubscription.CanceledAt
		response["subscription_created_at"] = dbSubscription.CreatedAt
		response["subscription_updated_at"] = dbSubscription.UpdatedAt

		// Récupérer les détails de l'abonnement directement depuis Stripe
		stripeSub, err := sub.Get(dbSubscription.StripeID, nil)
		if err == nil {
			// Ajouter des informations supplémentaires sur l'abonnement
			response["stripe_subscription_status"] = string(stripeSub.Status)
			response["stripe_current_period_start"] = time.Unix(stripeSub.CurrentPeriodStart, 0)
			response["stripe_current_period_end"] = time.Unix(stripeSub.CurrentPeriodEnd, 0)
			response["stripe_cancel_at_period_end"] = stripeSub.CancelAtPeriodEnd

			if stripeSub.CanceledAt > 0 {
				canceledAt := time.Unix(stripeSub.CanceledAt, 0)
				response["stripe_canceled_at"] = canceledAt
			}

			// Récupérer les informations de prix si disponibles
			if len(stripeSub.Items.Data) > 0 && stripeSub.Items.Data[0].Price != nil {
				priceID := stripeSub.Items.Data[0].Price.ID
				
				// Récupérer les détails complets du prix
				priceDetails, err := price.Get(priceID, nil)
				if err == nil {
					response["price_amount"] = float64(priceDetails.UnitAmount) / 100.0 // Convertir de centimes à euros
					response["price_currency"] = string(priceDetails.Currency)
					response["price_interval"] = string(priceDetails.Recurring.Interval)
					response["price_interval_count"] = priceDetails.Recurring.IntervalCount
					response["price_nickname"] = priceDetails.Nickname
					response["price_product"] = priceDetails.Product.ID
				} else {
					logrus.WithError(err).Warn("Failed to get price details from Stripe")
				}
			}
		} else {
			logrus.WithError(err).Warn("Failed to get subscription from Stripe")
			
			// Utiliser les données de la base de données pour les informations de base
			response["stripe_subscription_status"] = dbSubscription.Status
			
			// Estimer la date de début de la période actuelle (1 mois avant la fin)
			estimatedStart := dbSubscription.CurrentPeriodEnd.AddDate(0, -1, 0)
			response["stripe_current_period_start"] = estimatedStart
			response["stripe_current_period_end"] = dbSubscription.CurrentPeriodEnd
			
			// Déterminer si l'abonnement sera annulé à la fin de la période
			response["stripe_cancel_at_period_end"] = dbSubscription.CanceledAt != nil
			
			if dbSubscription.CanceledAt != nil {
				response["stripe_canceled_at"] = *dbSubscription.CanceledAt
			}
		}
	} else {
		response["has_subscription"] = false
	}

	// Get live customer data from Stripe API
	stripeCustomer, err := customer.Get(dbCustomer.StripeID, nil)
	if err == nil {
		// Add live Stripe customer details
		response["stripe_customer_email"] = stripeCustomer.Email
		response["stripe_customer_name"] = stripeCustomer.Name
		response["stripe_customer_phone"] = stripeCustomer.Phone
		response["stripe_customer_created"] = time.Unix(stripeCustomer.Created, 0)
		response["stripe_customer_default_source"] = stripeCustomer.DefaultSource

		// Add payment method details if available
		if stripeCustomer.InvoiceSettings.DefaultPaymentMethod != nil {
			response["default_payment_method"] = stripeCustomer.InvoiceSettings.DefaultPaymentMethod.ID
		}
	} else {
		logrus.WithError(err).Warn("Failed to get customer from Stripe")
	}

	sendJSON(w, http.StatusOK, response)
}
