package oms

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
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
	case event.TopicEventExecution:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		execution := event.NewExecutionFromBytes(buf)
		o.OnExecution(execution)
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
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order new")
}

func (o *OMS) OnOrderAccepted(ev event.OrderAccepted) {
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order accepted")
}

func (o *OMS) OnOrderPartiallyFilled(ev event.OrderPartiallyFilled) {
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order partially filled")
}

func (o *OMS) OnOrderFilled(ev event.OrderFilled) {
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order filled")
}

func (o *OMS) OnExecution(ev event.Execution) {
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Execution")
}

func (o *OMS) OnOrderCanceled(ev event.OrderCanceled) {
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order canceled")
}

func (o *OMS) OnOrderRejected(ev event.OrderRejected) {
	o.Log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order rejected")
}
