package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gobuffalo/uuid"
	"github.com/sirupsen/logrus"
)

// JWTClaims represents the claims in a JWT
type JWTClaims struct {
	jwt.StandardClaims
	Email    string                 `json:"email"`
	AppData  map[string]interface{} `json:"app_metadata"`
	UserData map[string]interface{} `json:"user_metadata"`
}

// requireAuthentication is middleware that requires a valid JWT token
func (a *API) requireAuthentication(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token := getToken(r)

		if token == "" {
			unauthorizedError(w)
			return
		}

		claims, err := a.parseJWT(token)
		if err != nil {
			logrus.WithError(err).Info("Invalid JWT token")
			unauthorizedError(w)
			return
		}

		if claims.Subject == "" {
			logrus.Info("JWT token missing sub claim")
			unauthorizedError(w)
			return
		}

		_, err = uuid.FromString(claims.Subject)
		if err != nil {
			logrus.WithError(err).Info("Invalid user ID in JWT token")
			unauthorizedError(w)
			return
		}

		// Add user ID to context
		ctx = context.WithValue(ctx, "user_id", claims.Subject)
		ctx = context.WithValue(ctx, "email", claims.Email)

		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	}
}

// parseJWT parses a JWT token
func (a *API) parseJWT(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(a.config.JWT.Secret), nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	if claims.ExpiresAt < time.Now().Unix() {
		return nil, fmt.Errorf("token expired")
	}

	if claims.Audience != a.config.JWT.Aud {
		return nil, fmt.Errorf("invalid token audience")
	}

	return claims, nil
}
