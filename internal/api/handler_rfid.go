package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/frendyardiansyah31/rfid-middleware/internal/decision"
	"github.com/frendyardiansyah31/rfid-middleware/internal/http/response"
	"github.com/frendyardiansyah31/rfid-middleware/internal/logger"
)

type RFIDHandler struct {
	engine decision.Engine
	logger logger.Logger
}

func NewRFIDHandler(engine decision.Engine, logger logger.Logger) *RFIDHandler {
	return &RFIDHandler{
		engine: engine,
		logger: logger,
	}
}

// func (h *RFIDHandler) CheckRFID(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != http.MethodPost {
// 		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
// 		return
// 	}

// 	var req RFIDCheckRequest
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		h.respondError(w, http.StatusBadRequest, "invalid JSON body")
// 		return
// 	}

// 	req.RFID = strings.TrimSpace(req.RFID)
// 	if req.RFID == "" {
// 		h.respondError(w, http.StatusBadRequest, "rfid is required")
// 		return
// 	}

// 	decisionCode := h.engine.CheckRFID(r.Context(), req.RFID)

// 	resp := RFIDCheckResponse{
// 		RFID:     req.RFID,
// 		Decision: decisionCode.Decision,
// 	}

//		h.respondJSON(w, http.StatusOK, resp)
//	}
// func (h *RFIDHandler) CheckRFID(w http.ResponseWriter, r *http.Request) {
// 	if r.Method != http.MethodPost {
// 		h.logger.Warn("invalid method",
// 			logger.F("method", r.Method),
// 		)
// 		w.WriteHeader(http.StatusMethodNotAllowed)
// 		return
// 	}

// 	var req RFIDCheckRequest
// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		h.logger.Warn("invalid json", logger.F("error", err))
// 		w.WriteHeader(http.StatusBadRequest)
// 		return
// 	}

// 	req.RFID = strings.TrimSpace(req.RFID)
// 	if req.RFID == "" {
// 		h.logger.Warn("empty rfid")
// 		w.WriteHeader(http.StatusBadRequest)
// 		return
// 	}

// 	result := h.engine.CheckRFID(r.Context(), req.RFID)

// 	// LOG semua decision penting
// 	h.logger.Info("rfid decision",
// 		logger.F("rfid", req.RFID),
// 		logger.F("decision", result.Decision),
// 		logger.F("reason", result.Reason),
// 	)

// 	// 🚨 kirim sinyal ke C#
// 	// if result.Signal {
// 	// 	h.signal.Notify(result.Reason)
// 	// }

//		h.respondJSON(w, http.StatusOK, RFIDCheckResponse{
//			RFID:     req.RFID,
//			Decision: result,
//			Reason:   string(result.Reason),
//		})
//	}
func (h *RFIDHandler) CheckRFID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.logger.Warn("invalid method", logger.F("method", r.Method))
		response.Fail(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Invalid method")
		return
	}

	var req RFIDCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Fail(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON body")
		return
	}

	req.RFID = strings.TrimSpace(req.RFID)
	if req.RFID == "" {
		response.Fail(w, http.StatusBadRequest, "INVALID_RFID", "RFID is empty")
		return
	}

	result := h.engine.CheckRFID(r.Context(), req.RFID)

	h.logger.Info("rfid decision",
		logger.F("rfid", req.RFID),
		logger.F("decision", result.Decision),
		logger.F("reason", result.Reason),
	)

	response.OK(w, "RFID checked", map[string]any{
		"rfid":     req.RFID,
		"decision": result.Decision,
		"reason":   result.Reason,
		"signal":   result.Signal,
	})
}

func (h *RFIDHandler) respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// func (h *RFIDHandler) respondError(w http.ResponseWriter, status int, message string) {
// 	h.respondJSON(w, status, ErrorResponse{
// 		Error: message,
// 	})
// }
