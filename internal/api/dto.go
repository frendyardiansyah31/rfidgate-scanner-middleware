package api

import "github.com/frendyardiansyah31/rfid-middleware/internal/decision"

type RFIDCheckRequest struct {
	RFID string `json:"rfid"`
}

type RFIDCheckResponse struct {
	RFID     string          `json:"rfid"`
	Decision decision.Result `json:"decision"` // 0 = allowed, 1 = alarm
	Reason   string          `json:"reason"`
}

// type ErrorResponse struct {
// 	Error string `json:"error"`
// }
