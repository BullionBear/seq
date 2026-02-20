package oms

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/BullionBear/seq/execution"
	"github.com/mitchellh/mapstructure"
)

func init() {
	execution.Register("oms", func(_ *msgbus.MsgBus, c *cache.Cache) actor.Actor {
		return NewOMS(c)
	})
}

var _ actor.Actor = (*OMS)(nil)

type OMS struct {
	actor.ActorBase
	cache     *cache.Cache
	accountID int
	account   string
}

func NewOMS(c *cache.Cache) *OMS {
	return &OMS{
		ActorBase: actor.NewActorBase("oms", []event.Topic{
			event.TopicEventOrderNew,
			event.TopicEventOrderAccepted,
			event.TopicEventOrderPartialFill,
			event.TopicEventOrderFilled,
			event.TopicEventOrderCanceled,
			event.TopicEventOrderRejected,
		}),
		cache: c,
	}
}

func (o *OMS) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventOrderNew:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderNew := event.NewOrderNewFromBytes(buf)
		o.OnOrderNew(orderNew)
	case event.TopicEventOrderAccepted:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderAccepted := event.NewOrderAcceptedFromBytes(buf)
		o.OnOrderAccepted(orderAccepted)
	case event.TopicEventOrderPartialFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderPartiallyFilled := event.NewOrderPartiallyFilledFromBytes(buf)
		o.OnOrderPartiallyFilled(orderPartiallyFilled)
	case event.TopicEventOrderFilled:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderFilled := event.NewOrderFilledFromBytes(buf)
		o.OnOrderFilled(orderFilled)
	case event.TopicEventOrderCanceled:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderCanceled := event.NewOrderCanceledFromBytes(buf)
		o.OnOrderCanceled(orderCanceled)
	case event.TopicEventOrderRejected:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderRejected := event.NewOrderRejectedFromBytes(buf)
		o.OnOrderRejected(orderRejected)
	}
}

func (o *OMS) OnInit(config map[string]any) {
	var cfg OMSConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &cfg,
		TagName: "yaml",
	})
	if err != nil {
		o.Log().Error().Err(err).Msg("OMS: failed to create decoder")
		return
	}
	if err := decoder.Decode(config); err != nil {
		o.Log().Error().Err(err).Msg("OMS: failed to decode config")
		return
	}

	o.accountID = cfg.ID
	o.account = cfg.Account
	o.Log().Info().Int("accountID", o.accountID).Str("account", o.account).Msg("OMS initialized")
}

func (o *OMS) OnStart() {
	o.Log().Info().Msg("OMS started")
}

func (o *OMS) OnStop() {
	o.Log().Info().Msg("OMS stopped")
}

func (o *OMS) OnOrderNew(ev event.OrderNew) {
	if ev.AccountID != o.accountID {
		return
	}
	o.Log().Info().
		Int("clientOrderID", ev.ClientOrderID).
		Int("orderID", ev.OrderID).
		Int("accountID", ev.AccountID).
		Int("symbolID", ev.SymbolID).
		Float64("price", ev.Price).
		Float64("quantity", ev.Quantity).
		Msg("Order new")
	o.cache.InsertOrder(&common.Order{
		ClientOrderID: ev.ClientOrderID,
		OrderID:       ev.OrderID,
		AccountID:     ev.AccountID,
		SymbolID:      ev.SymbolID,
		Side:          ev.Side,
		OrderType:     ev.OrderType,
		TimeInForce:   ev.TimeInForce,
		OrderStatus:   common.OrderStatusInitialized,
		Quantity:      ev.Quantity,
		Price:         ev.Price,
		ExecutedQty:   ev.ExecutedQty,
		CreatedAt:     ev.CreatedAt,
		UpdatedAt:     ev.UpdatedAt,
	})
}

func (o *OMS) OnOrderAccepted(ev event.OrderAccepted) {
	if ev.AccountID != o.accountID {
		return
	}
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Msg("Order accepted")
}

func (o *OMS) OnOrderPartiallyFilled(ev event.OrderPartiallyFilled) {
	order, ok := o.cache.GetOrder(ev.ClientOrderID)
	if !ok {
		o.Log().Error().Int("clientOrderID", ev.ClientOrderID).Msg("Order not found")
		return
	}
	order.ExecutedQty += ev.ExecutedQty
	order.UpdatedAt = ev.UpdatedAt
	o.cache.UpdateOrder(&order)
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Float64("executedQty", ev.ExecutedQty).Msg("Order partially filled")
}

func (o *OMS) OnOrderFilled(ev event.OrderFilled) {
	if ev.AccountID != o.accountID {
		return
	}
	order, ok := o.cache.GetOrder(ev.ClientOrderID)
	if !ok {
		o.Log().Error().Int("clientOrderID", ev.ClientOrderID).Msg("Order not found")
		return
	}
	order.ExecutedQty += ev.ExecutedQty
	order.UpdatedAt = ev.UpdatedAt
	order.OrderStatus = common.OrderStatusFilled
	o.cache.UpdateOrder(&order)
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Float64("executedQty", ev.ExecutedQty).Msg("Order filled")
}

func (o *OMS) OnOrderCanceled(ev event.OrderCanceled) {
	order, ok := o.cache.GetOrder(ev.ClientOrderID)
	if !ok {
		o.Log().Error().Int("clientOrderID", ev.ClientOrderID).Msg("Order not found")
		return
	}
	order.OrderStatus = common.OrderStatusCanceled
	order.UpdatedAt = ev.UpdatedAt
	o.cache.UpdateOrder(&order)
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Int("accountID", ev.AccountID).Msg("Order canceled")
}

func (o *OMS) OnOrderRejected(ev event.OrderRejected) {
	order, ok := o.cache.GetOrder(ev.ClientOrderID)
	if !ok {
		o.Log().Error().Int("clientOrderID", ev.ClientOrderID).Msg("Order not found")
		return
	}
	order.OrderStatus = common.OrderStatusRejected
	order.UpdatedAt = ev.UpdatedAt
	o.cache.UpdateOrder(&order)
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Int("accountID", ev.AccountID).Msg("Order rejected")
}
