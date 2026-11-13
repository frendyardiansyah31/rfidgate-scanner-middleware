package decision

import "context"

type Engine interface {
	CheckRFID(ctx context.Context, rfid string) Result
}
