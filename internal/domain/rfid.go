package domain

import "time"

// RFIDSnapshot represents the state and metadata of an RFID item at a specific time.
type RFIDSnapshot struct {
	RFID      string
	ItemCode  string
	Title     string
	Status    int
	UpdatedAt time.Time
}
