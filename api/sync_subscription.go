package api

import (
	"encoding/json"
	"net/http"
	"time"

	"gostripe/models"

	"github.com/sirupsen/logrus"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/checkout/session"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/sub"
)

// SyncSubscriptionRequest représente la requête pour synchroniser un abonnement
type SyncSubscriptionRequest struct {
	SessionID string `json:"session_id"`
}

// SyncSubscription force la synchronisation de l'abonnement après un paiement réussi
func (a *API) SyncSubscription(w http.ResponseWriter, r *http.Request) {
	// Décoder la requête pour obtenir l'ID de session
	var req SyncSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logrus.WithError(err).Error("Failed to decode request body")
		badRequestError(w, "Invalid request body")
		return
	}

	// Vérifier si l'ID de session est fourni
	if req.SessionID == "" {
		// Si aucun ID de session n'est fourni, utiliser l'ancienne méthode
		a.syncSubscriptionFromCustomer(w, r)
		return
	}

	// Récupérer la session Stripe
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("line_items")
	params.AddExpand("subscription")
	params.AddExpand("customer")
	sess, err := session.Get(req.SessionID, params)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve Stripe session")
		internalServerError(w, r, "Failed to retrieve Stripe session")
		return
	}

	// Vérifier si la session contient un abonnement
	if sess.Subscription == nil {
		logrus.Error("No subscription found in Stripe session")
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": "No subscription found in Stripe session",
		})
		return
	}

	// Récupérer l'utilisateur à partir du contexte
	userID, err := getUserID(r.Context())
	if err != nil {
		internalServerError(w, r, "Failed to get user ID")
		return
	}

	// Récupérer le client depuis la base de données
	dbCustomer, err := models.FindCustomerByUserID(a.db, userID)
	if err != nil {
		internalServerError(w, r, "Failed to get customer")
		return
	}

	if dbCustomer == nil {
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": "Customer not found",
		})
		return
	}

	// Extraire les informations de l'abonnement
	stripeSub := sess.Subscription
	hasSubscription := true
	stripeSubscriptionID := stripeSub.ID
	subscriptionStatus := string(stripeSub.Status)
	currentPeriodEnd := time.Unix(stripeSub.CurrentPeriodEnd, 0)

	// Déterminer l'ID du prix
	var priceID string
	if len(stripeSub.Items.Data) > 0 && stripeSub.Items.Data[0].Price != nil {
		priceID = stripeSub.Items.Data[0].Price.ID
	} else {
		// Utiliser un ID de prix par défaut si non disponible
		priceID = "price_1PSQokJKyP34gH73kOw1DhX1"
	}

	// Vérifier si l'abonnement existe déjà dans la base de données
	dbSubscription, err := models.FindSubscriptionByStripeID(a.db, stripeSubscriptionID)
	if err != nil {
		logrus.WithError(err).Error("Failed to check if subscription exists in database")
		internalServerError(w, r, "Failed to check subscription")
		return
	}

	if dbSubscription != nil {
		// L'abonnement existe déjà, nous le mettons à jour
		dbSubscription.Status = models.SubscriptionStatus(subscriptionStatus)
		dbSubscription.CurrentPeriodEnd = currentPeriodEnd
		dbSubscription.PriceID = priceID

		err = models.UpdateSubscription(a.db, dbSubscription)
		if err != nil {
			logrus.WithError(err).Error("Failed to update subscription in database")
			internalServerError(w, r, "Failed to update subscription")
			return
		}

		logrus.WithFields(logrus.Fields{
			"customer_id":            dbCustomer.ID,
			"stripe_subscription_id": stripeSubscriptionID,
			"status":                 subscriptionStatus,
		}).Info("Updated subscription in database")
	} else {
		// L'abonnement n'existe pas encore, nous le créons
		_, err = models.CreateSubscription(
			a.db,
			dbCustomer.ID,
			stripeSubscriptionID,
			priceID,
			models.SubscriptionStatus(subscriptionStatus),
			currentPeriodEnd,
		)
		if err != nil {
			logrus.WithError(err).Error("Failed to create subscription in database")
			internalServerError(w, r, "Failed to create subscription")
			return
		}

		logrus.WithFields(logrus.Fields{
			"customer_id":            dbCustomer.ID,
			"stripe_subscription_id": stripeSubscriptionID,
			"status":                 subscriptionStatus,
		}).Info("Created new subscription in database")
	}

	// Réponse de succès
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":             true,
		"message":             "Subscription synchronized successfully",
		"has_subscription":    hasSubscription,
		"subscription_status": subscriptionStatus,
		"current_period_end":  currentPeriodEnd,
	})
}

// syncSubscriptionFromCustomer synchronise l'abonnement en utilisant les informations du client
func (a *API) syncSubscriptionFromCustomer(w http.ResponseWriter, r *http.Request) {
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
			"success": false,
			"message": "Customer not found",
		})
		return
	}

	// Vérifier que le client existe dans Stripe
	_, err = customer.Get(dbCustomer.StripeID, nil)
	if err != nil {
		logrus.WithError(err).Error("Failed to get customer from Stripe")
		internalServerError(w, r, "Failed to get customer from Stripe")
		return
	}

	// Récupérer directement les abonnements depuis l'API Stripe
	hasSubscription := false
	var stripeSubscriptionID string
	var subscriptionStatus string
	var currentPeriodEnd time.Time
	var priceID string

	// Paramètres pour récupérer les abonnements actifs du client
	params := &stripe.SubscriptionListParams{}
	params.SetStripeAccount("")
	params.Customer = dbCustomer.StripeID
	params.Status = "all" // Récupérer tous les abonnements, pas seulement les actifs

	// Limiter à 1 résultat pour simplifier
	params.Limit = stripe.Int64(1)

	// Récupérer les abonnements depuis Stripe
	subscriptionIterator := sub.List(params)

	// Vérifier si nous avons au moins un abonnement
	if subscriptionIterator.Next() {
		// Récupérer le premier abonnement
		stripeSub := subscriptionIterator.Subscription()

		hasSubscription = true
		stripeSubscriptionID = stripeSub.ID
		subscriptionStatus = string(stripeSub.Status)
		currentPeriodEnd = time.Unix(stripeSub.CurrentPeriodEnd, 0)

		// Déterminer l'ID du prix
		if len(stripeSub.Items.Data) > 0 && stripeSub.Items.Data[0].Price != nil {
			priceID = stripeSub.Items.Data[0].Price.ID
		} else {
			// Utiliser un ID de prix par défaut si non disponible
			priceID = "price_1PSQokJKyP34gH73kOw1DhX1"
		}

		// Vérifier si l'abonnement existe déjà dans la base de données
		dbSubscription, err := models.FindSubscriptionByStripeID(a.db, stripeSubscriptionID)
		if err != nil {
			logrus.WithError(err).Error("Failed to check if subscription exists in database")
			internalServerError(w, r, "Failed to check subscription")
			return
		}

		if dbSubscription != nil {
			// L'abonnement existe déjà, nous le mettons à jour
			dbSubscription.Status = models.SubscriptionStatus(subscriptionStatus)
			dbSubscription.CurrentPeriodEnd = currentPeriodEnd
			dbSubscription.PriceID = priceID

			err = models.UpdateSubscription(a.db, dbSubscription)
			if err != nil {
				logrus.WithError(err).Error("Failed to update subscription in database")
				internalServerError(w, r, "Failed to update subscription")
				return
			}

			logrus.WithFields(logrus.Fields{
				"customer_id":            dbCustomer.ID,
				"stripe_subscription_id": stripeSubscriptionID,
				"status":                 subscriptionStatus,
			}).Info("Updated subscription in database")
		} else {
			// L'abonnement n'existe pas encore, nous le créons
			_, err = models.CreateSubscription(
				a.db,
				dbCustomer.ID,
				stripeSubscriptionID,
				priceID,
				models.SubscriptionStatus(subscriptionStatus),
				currentPeriodEnd,
			)
			if err != nil {
				logrus.WithError(err).Error("Failed to create subscription in database")
				internalServerError(w, r, "Failed to create subscription")
				return
			}

			logrus.WithFields(logrus.Fields{
				"customer_id":            dbCustomer.ID,
				"stripe_subscription_id": stripeSubscriptionID,
				"status":                 subscriptionStatus,
			}).Info("Created new subscription in database")
		}
	} else if subscriptionIterator.Err() != nil {
		// Une erreur s'est produite lors de la récupération des abonnements
		logrus.WithError(subscriptionIterator.Err()).Error("Failed to list subscriptions from Stripe")
		internalServerError(w, r, "Failed to list subscriptions from Stripe")
		return
	} else {
		// Aucun abonnement trouvé pour ce client
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": "No subscription found for this customer",
		})
		return
	}

	// Réponse de succès
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"success":             true,
		"message":             "Subscription synchronized successfully",
		"has_subscription":    hasSubscription,
		"subscription_status": subscriptionStatus,
		"current_period_end":  currentPeriodEnd,
	})
}
