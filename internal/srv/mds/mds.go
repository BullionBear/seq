package mds

import (
	"github.com/BullionBear/seq/internal/srv/catalog"
	"github.com/BullionBear/seq/pkg/evbus"
)

type MarketDataManager struct {
	catalog *catalog.Catalog
}

func NewMarketDataManager(catalog *catalog.Catalog) *MarketDataManager {
	return &MarketDataManager{catalog: catalog}
}

func (m *MarketDataManager) SubscribeTick(symbolID int, callback func(*evbus.Event[Tick]) error, errCallback func(error)) (unsubscribe func(), err error) {
	return nil, nil
}
