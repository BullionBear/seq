package evbus

import (
	"github.com/BullionBear/seq/core/model/event"
)

type EventRef struct {
	DataType event.DataType
	Index    uint64
}

// Event wraps data with metadata. Data is embedded as a value type
// so Event and Data are pooled together (single allocation).
type Event struct {
	Ref       EventRef
	EventID   uint64
	CreatedAt uint64
	UpdatedAt uint64
}
