package mds

import (
	"github.com/BullionBear/seq/internal/srv/sms"
	"github.com/BullionBear/seq/pkg/evbus"
)

type MarketDataManager struct {
	sms *sms.SecretManager
}

func NewMarketDataManager(sms *sms.SecretManager) *MarketDataManager {
	return &MarketDataManager{sms: sms}
}

func (m *MarketDataManager) SubscribeTick(symbolID int, callback func(*evbus.Event[Tick]) error, errCallback func(error)) (unsubscribe func(), err error) {
	return nil, nil
}
