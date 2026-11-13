package response

import (
	"time"
)

type Envelope struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`

	Data any `json:"data,omitempty"`
	Meta any `json:"meta,omitempty"`
}

type Meta struct {
	Timestamp time.Time `json:"timestamp"`
}
