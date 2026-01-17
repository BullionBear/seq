package strategy

import (
	"github.com/BullionBear/seq/internal/adapter"
	"github.com/rs/zerolog"
)

type StrategyCommon struct {
	log              zerolog.Logger
	dataClientRouter adapter.DataClientRouter
	executionRouter  adapter.ExecutionRouter
}
