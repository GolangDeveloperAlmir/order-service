package respond

import (
	"encoding/json"
	"log"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode json: %v", err)
	}
}

func Error(w http.ResponseWriter, status int, msg string) {
	type e struct {
		Error string `json:"error"`
	}
	JSON(w, status, e{Error: msg})
}
