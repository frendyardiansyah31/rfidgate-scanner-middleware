package response

import (
	"encoding/json"
	"net/http"
	"time"
)

func OK(w http.ResponseWriter, message string, data any) {
	write(w, http.StatusOK, Envelope{
		Success: true,
		Code:    "OK",
		Message: message,
		Data:    data,
		Meta:    Meta{Timestamp: time.Now()},
	})
}

func Fail(w http.ResponseWriter, status int, code, message string) {
	write(w, status, Envelope{
		Success: false,
		Code:    code,
		Message: message,
	})
}

func write(w http.ResponseWriter, status int, payload Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
