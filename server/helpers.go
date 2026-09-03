package main

import (
	"context"
	"encoding/json"
	"net/http"
)

type ctxKeyType string

const userCtxKey ctxKeyType = "gr_user"

type apiError struct {
	Error string `json:"error"`
}

func contextWithUser(r *http.Request, u *user) context.Context {
	return context.WithValue(r.Context(), userCtxKey, u)
}

func userFrom(r *http.Request) *user {
	u, _ := r.Context().Value(userCtxKey).(*user)
	return u
}

func mustJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	// API responses may depend on the caller's identity; never let the browser
	// cache them (a cached GET /api/me could leak another user's data).
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	mustJSON(w, v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{msg})
}
