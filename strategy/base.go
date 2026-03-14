package strategy

import (
	"time"

	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/catalog"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

var _ actor.Actor = (*StrategyActorBase)(nil)

type StrategyActorBase struct {
	actor.ActorBase
	catalog *catalog.Catalog
	cache   *cache.Cache
	msgbus  *msgbus.MsgBus
}

func NewStrategyActorBase(name string, catalog *catalog.Catalog, cache *cache.Cache, msgbus *msgbus.MsgBus, topics []event.Topic) StrategyActorBase {
	return StrategyActorBase{
		ActorBase: actor.NewActorBase(name, topics),
		catalog:   catalog,
		cache:     cache,
		msgbus:    msgbus,
	}
}

func (s *StrategyActorBase) GetCatalog() *catalog.Catalog {
	return s.catalog
}

func (s *StrategyActorBase) SubmitOrder(accountID int, symbolID int, side common.Side, orderType common.OrderType, timeInForce common.TimeInForce, price float64, quantity float64) int {
	now := time.Now().UnixNano()
	clientOrderId := int(now % 1000000)

	s.cache.InsertOrder(&common.Order{
		ClientOrderID: clientOrderId,
		AccountID:     accountID,
		SymbolID:      symbolID,
		Side:          side,
		OrderType:     orderType,
		TimeInForce:   timeInForce,
		OrderStatus:   common.OrderStatusInitialized,
		Quantity:      quantity,
		Price:         price,
		CreatedAt:     uint64(now),
	})

	riskCmd := command.RiskCheck{
		ClientOrderID: clientOrderId,
		AccountID:     accountID,
		SymbolID:      symbolID,
		Side:          side,
		OrderType:     orderType,
		TimeInForce:   timeInForce,
		Price:         price,
		Quantity:      quantity,
		Timestamp:     uint64(now),
	}
	size := uint64(riskCmd.GetBufferLength())
	offset, buf := s.msgbus.AllocateCmd(size)
	riskCmd.Encode(buf)
	s.msgbus.Send(msgbus.CommandRef{
		CommandType: command.CommandTypeOrderRiskCheck,
		Index:       offset,
		Length:      size,
	})
	return clientOrderId
}

func (s *StrategyActorBase) CancelOrder(clientOrderID int, accountID int) {
	cancelCmd := command.CancelOrder{
		AccountID:     accountID,
		ClientOrderID: clientOrderID,
	}
	size := uint64(cancelCmd.GetBufferLength())
	offset, buf := s.msgbus.AllocateCmd(size)
	cancelCmd.Encode(buf)
	s.msgbus.Send(msgbus.CommandRef{
		CommandType: command.CommandTypeOrderCancel,
		Index:       offset,
		Length:      size,
	})
}

func (s *StrategyActorBase) CancelAllOrders(accountID int, symbolID int) {
	cancelAllCmd := command.CancelAll{
		AccountID: accountID,
		SymbolID:  symbolID,
	}
	size := uint64(cancelAllCmd.GetBufferLength())
	offset, buf := s.msgbus.AllocateCmd(size)
	cancelAllCmd.Encode(buf)
	s.msgbus.Send(msgbus.CommandRef{
		CommandType: command.CommandTypeCancelAll,
		Index:       offset,
		Length:      size,
	})
}
