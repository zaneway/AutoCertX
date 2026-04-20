package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, statusCode int, payload any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(payload)
}
