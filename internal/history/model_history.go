package history

import "time"

type HistoryLog struct {
	ID            int64     `json:"id"`
	RFID          string    `json:"rfid"`
	Status        int       `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	ItemCode      string    `json:"item_code"`
	Title         string    `json:"title"`
	BiblioID      string    `json:"biblio_id"`
	ImageFilename string    `json:"image_filename"`
}
