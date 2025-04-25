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

	// Récupérer l'utilisateur à partir du contexte
	userID, err := getUserID(r.Context())
	if err != nil {
		internalServerError(w, r, "Failed to get user ID")
		return
	}

	// Vérifier si cette session a déjà été traitée
	logrus.WithFields(logrus.Fields{
		"session_id": req.SessionID,
		"user_id": userID,
	}).Info("Checking if session was already processed")

	// Vérifier si la session a déjà été traitée - mais ne pas créer immédiatement un enregistrement
	var processedSession *models.ProcessedSession
	processedSession, err = models.FindProcessedSessionBySessionID(a.db, req.SessionID)

	var newlyCreatedSession bool = false

	// Gérer le cas où aucune ligne n'est trouvée (normal au démarrage ou première utilisation)
	if err != nil {
		// Vérifier si c'est une erreur "no rows" qui est normale
		if err.Error() == "sql: no rows in result set" {
			// Ce n'est pas une vraie erreur, juste qu'aucune session n'a été trouvée
			logrus.WithFields(logrus.Fields{
				"session_id": req.SessionID,
				"user_id": userID,
			}).Info("No processed session found, this is the first time this session is processed")

			// On ne crée pas encore l'entrée dans la base de données, on le fera à la fin du traitement
			newlyCreatedSession = true
			processedSession = nil
		} else {
			// C'est une vraie erreur de base de données
			logrus.WithError(err).Error("Failed to check if session was already processed")
			// Ne pas bloquer l'utilisateur, continuer quand même
			processedSession = nil
		}
	}

	// Si la session a déjà été traitée (mais pas si elle vient d'être créée)
	if processedSession != nil {
		// Vérifier si l'utilisateur actuel est celui qui a traité la session
		if processedSession.UserID != userID {
			logrus.WithFields(logrus.Fields{
				"session_id": req.SessionID,
				"user_id": userID,
				"original_user_id": processedSession.UserID,
			}).Warn("Attempt to use a session ID that belongs to another user")

			sendJSON(w, http.StatusForbidden, map[string]interface{}{
				"success": false,
				"message": "Cette session de paiement a déjà été utilisée par un autre compte",
			})
			return
		}

		// Si c'est le même utilisateur, informer que la session a déjà été traitée
		logrus.WithFields(logrus.Fields{
			"session_id": req.SessionID,
			"user_id": userID,
		}).Info("Session already processed, but by the same user")

		// Récupérer l'abonnement actuel pour le renvoyer avec la réponse
		dbCustomer, err := models.FindCustomerByUserID(a.db, userID)
		if err != nil || dbCustomer == nil {
			// Si on ne trouve pas le client, on renvoie un message générique
			sendJSON(w, http.StatusOK, map[string]interface{}{
				"success": true,
				"already_processed": true,
				"message": "Ce paiement a déjà été traité. Votre abonnement est actif.",
			})
			return
		}

		// Trouver l'abonnement actif pour ce client
		subscription, err := models.FindActiveSubscriptionByCustomerID(a.db, dbCustomer.ID)
		if err != nil || subscription == nil {
			// Si on ne trouve pas d'abonnement actif, envoyer un message générique
			sendJSON(w, http.StatusOK, map[string]interface{}{
				"success": true,
				"already_processed": true,
				"message": "Ce paiement a déjà été traité. Votre abonnement est actif.",
			})
			return
		}

		// Renvoyer les détails de l'abonnement existant
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"already_processed": true,
			"message": "Ce paiement a déjà été traité. Votre abonnement est actif.",
			"has_subscription": true,
			"subscription_status": string(subscription.Status),
			"current_period_end": subscription.CurrentPeriodEnd,
		})
		return
	}

	// Récupérer la session Stripe
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("line_items")
	params.AddExpand("subscription")
	params.AddExpand("customer")

	logrus.WithFields(logrus.Fields{
		"session_id": req.SessionID,
	}).Info("Fetching session from Stripe")

	sess, err := session.Get(req.SessionID, params)
	if err != nil {
		logrus.WithError(err).Error("Failed to retrieve Stripe session")
		internalServerError(w, r, "Failed to retrieve Stripe session")
		return
	}

	// Préparer les valeurs pour le log
	hasSubscription := sess.Subscription != nil
	subscriptionID := "none"
	customerID := "none"

	if sess.Subscription != nil {
		subscriptionID = sess.Subscription.ID
	}

	if sess.Customer != nil {
		customerID = sess.Customer.ID
	}

	logrus.WithFields(logrus.Fields{
		"session_id": req.SessionID,
		"has_subscription": hasSubscription,
		"subscription_id": subscriptionID,
		"customer_id": customerID,
	}).Info("Retrieved session from Stripe")

	// Vérifier si la session contient un abonnement
	if sess.Subscription == nil {
		logrus.WithFields(logrus.Fields{
			"session_id": req.SessionID,
		}).Error("No subscription found in Stripe session")

		sendJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": "No subscription found in Stripe session",
		})
		return
	}

	// Récupérer l'utilisateur à partir du contexte
	userID, err = getUserID(r.Context())
	if err != nil {
		internalServerError(w, r, "Failed to get user ID")
		return
	}

	// Récupérer le client depuis la base de données
	dbCustomer, err := models.FindCustomerByUserID(a.db, userID)
	if err != nil {
		logrus.WithError(err).Error("Failed to find customer by user ID")
		internalServerError(w, r, "Failed to get customer")
		return
	}

	// Log pour voir si le client existe ou pas
	logrus.WithFields(logrus.Fields{
		"user_id":        userID,
		"customer_found": dbCustomer != nil,
		"stripe_customer_id": sess.Customer.ID,
	}).Info("Customer lookup result")

	// Si le client n'existe pas, nous devons le créer avec les informations de la session Stripe
	if dbCustomer == nil {
		logrus.WithFields(logrus.Fields{
			"user_id": userID,
			"stripe_customer_id": sess.Customer.ID,
		}).Info("Customer not found in database, creating new customer record")

		// Récupérer les détails du client depuis Stripe
		stripeCustomer, err := customer.Get(sess.Customer.ID, nil)
		if err != nil {
			logrus.WithError(err).Error("Failed to get customer details from Stripe")
			internalServerError(w, r, "Failed to get customer details from Stripe")
			return
		}

		// Log des détails du client Stripe avant création
		logrus.WithFields(logrus.Fields{
			"stripe_customer_id": stripeCustomer.ID,
			"email":             stripeCustomer.Email,
			"name":              stripeCustomer.Name,
		}).Info("Retrieved Stripe customer details")

		// Créer un nouveau client dans la base de données
		dbCustomer, err = models.CreateCustomer(a.db, userID, stripeCustomer.ID, stripeCustomer.Email, stripeCustomer.Name)
		if err != nil {
			logrus.WithError(err).Error("Failed to create customer in database")
			internalServerError(w, r, "Failed to create customer")
			return
		}

		logrus.WithFields(logrus.Fields{
			"user_id": userID,
			"stripe_customer_id": sess.Customer.ID,
			"db_customer_id": dbCustomer.ID,
		}).Info("Created new customer in database")
	} else {
		logrus.WithFields(logrus.Fields{
			"user_id": userID,
			"db_customer_id": dbCustomer.ID,
			"stripe_customer_id": dbCustomer.StripeID,
		}).Info("Found existing customer in database")
	}

	if dbCustomer == nil {
		logrus.Error("Customer is still nil after creation attempt")
		sendJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"message": "Customer not found or could not be created",
		})
		return
	}

	// Extraire les informations de l'abonnement
	stripeSub := sess.Subscription
	hasSubscription = true
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

	logrus.WithFields(logrus.Fields{
		"subscription_exists": dbSubscription != nil,
		"stripe_id": stripeSubscriptionID,
		"status": subscriptionStatus,
	}).Info("Checking if subscription exists in database")

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
		createdSubscription, err := models.CreateSubscription(
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
			"subscription_id":        createdSubscription.ID,
		}).Info("Created new subscription in database")
	}

	// Enregistrer la session comme traitée si elle ne l'a pas déjà été
	if newlyCreatedSession && req.SessionID != "" {
		logrus.WithFields(logrus.Fields{
			"session_id": req.SessionID,
			"user_id": userID,
		}).Info("Marking session as processed")

		_, err = models.CreateProcessedSession(a.db, req.SessionID, userID)
		if err != nil {
			logrus.WithError(err).Warn("Failed to mark session as processed")
			// On continue quand même, ce n'est pas une erreur critique
		}
	}

	logrus.WithFields(logrus.Fields{
		"session_id": req.SessionID,
		"user_id": userID,
	}).Info("Session processing completed successfully")

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
