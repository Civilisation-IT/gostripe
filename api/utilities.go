package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	"gostripe/conf"

	"github.com/gofrs/uuid"
	"github.com/sirupsen/logrus"
)

// Error represents an error that occurred while processing a request
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// sendJSON sends a JSON response with the given status code and data
func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logrus.WithError(err).Error("Error encoding json response")
	}
}

// addRequestID is middleware that adds a request ID to the context
func addRequestID(config *conf.GlobalConfiguration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if config.API.RequestIDHeader != "" {
				id = r.Header.Get(config.API.RequestIDHeader)
			}
			if id == "" {
				uid, err := uuid.NewV4()
				if err != nil {
					logrus.WithError(err).Error("Error generating request ID")
					internalServerError(w, r, err.Error())
					return
				}
				id = uid.String()
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, "request_id", id)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// recoverer is middleware that recovers from panics
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				logError(r, fmt.Errorf("panic: %+v", rvr))
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// logError logs an error
func logError(r *http.Request, err error) {
	var buf [4096]byte
	stack := buf[:runtime.Stack(buf[:], false)]
	logrus.WithFields(logrus.Fields{
		"method":     r.Method,
		"path":       r.URL.Path,
		"error_msg":  err.Error(),
		"stacktrace": string(stack),
	}).Error("Internal server error")
}

// badRequestError sends a 400 Bad Request response
func badRequestError(w http.ResponseWriter, msg string) {
	sendJSON(w, http.StatusBadRequest, &Error{
		Code:    http.StatusBadRequest,
		Message: msg,
	})
}

// unauthorizedError sends a 401 Unauthorized response
func unauthorizedError(w http.ResponseWriter) {
	sendJSON(w, http.StatusUnauthorized, &Error{
		Code:    http.StatusUnauthorized,
		Message: "Unauthorized",
	})
}

// forbiddenError sends a 403 Forbidden response
func forbiddenError(w http.ResponseWriter, msg string) {
	sendJSON(w, http.StatusForbidden, &Error{
		Code:    http.StatusForbidden,
		Message: msg,
	})
}

// notFoundError sends a 404 Not Found response
func notFoundError(w http.ResponseWriter, msg string) {
	sendJSON(w, http.StatusNotFound, &Error{
		Code:    http.StatusNotFound,
		Message: msg,
	})
}

// internalServerError sends a 500 Internal Server Error response
func internalServerError(w http.ResponseWriter, r *http.Request, msg string) {
	logrus.WithFields(logrus.Fields{
		"method": r.Method,
		"path":   r.URL.Path,
	}).Error(msg)
	sendJSON(w, http.StatusInternalServerError, &Error{
		Code:    http.StatusInternalServerError,
		Message: msg,
	})
}

// getRequestID gets the request ID from the context
func getRequestID(ctx context.Context) string {
	id, ok := ctx.Value("request_id").(string)
	if !ok {
		return ""
	}
	return id
}

// getUserID gets the user ID from the context
func getUserID(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value("user_id").(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("user_id not found in context")
	}
	return uuid.FromString(id)
}

// getToken gets the JWT token from the Authorization header
func getToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	matches := bearerRegexp.FindStringSubmatch(authHeader)
	if len(matches) != 2 {
		return ""
	}

	return matches[1]
}
