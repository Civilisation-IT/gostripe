package models

import (
	"log"
	"time"

	"gostripe/storage"

	"github.com/gofrs/uuid"
	"github.com/pkg/errors"
)

// ProcessedSession représente une session Stripe déjà traitée
type ProcessedSession struct {
	ID        uuid.UUID `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// TableName retourne le nom de la table pour le modèle ProcessedSession
func (ProcessedSession) TableName() string {
	return "stripe_processed_sessions"
}

// FindProcessedSessionBySessionID recherche une session traitée par son ID de session Stripe
func FindProcessedSessionBySessionID(conn *storage.Connection, sessionID string) (*ProcessedSession, error) {
	processedSession := &ProcessedSession{}
	err := conn.Where("session_id = ?", sessionID).First(processedSession)
	
	// Gérer explicitement le cas "pas de lignes trouvées"
	if err != nil {
		// Différentes façons dont l'erreur "no rows" peut être exprimée selon le driver
		if err == storage.ErrNotFound || 
		   err.Error() == "sql: no rows in result set" || 
		   err.Error() == "no rows in result set" {
			// Retourner une erreur standardisée pour que le code appelant puisse la détecter facilement
			return nil, errors.New("sql: no rows in result set")
		}
		return nil, errors.Wrap(err, "error finding processed session")
	}
	
	return processedSession, nil
}

// CreateProcessedSession crée une nouvelle session traitée
func CreateProcessedSession(conn *storage.Connection, sessionID string, userID uuid.UUID) (*ProcessedSession, error) {
	// Logs pour déboguer les valeurs d'entrée
	log.Printf("CreateProcessedSession appelé avec sessionID=%s, userID=%s", sessionID, userID.String())
	
	processedSession := &ProcessedSession{
		ID:        uuid.Must(uuid.NewV4()),
		SessionID: sessionID,
		UserID:    userID,
		CreatedAt: time.Now(),
	}
	
	// Log de l'objet complet avant insertion
	log.Printf("Tentative de création d'une processed session: ID=%s, SessionID=%s, UserID=%s", 
		processedSession.ID.String(), processedSession.SessionID, processedSession.UserID.String())

	if err := conn.Create(processedSession); err != nil {
		log.Printf("ERREUR lors de la création de la processed session: %v", err)
		return nil, errors.Wrap(err, "error creating processed session")
	}

	log.Printf("Session traitée créée avec succès: ID=%s", processedSession.ID.String())
	return processedSession, nil
}
