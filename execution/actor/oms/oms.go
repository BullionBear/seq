package oms

import (
	"github.com/BullionBear/seq/core/actor"
	"github.com/BullionBear/seq/core/cache"
	"github.com/BullionBear/seq/core/logger"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
	"github.com/rs/zerolog"
)

func log() *zerolog.Logger { l := logger.Get(); return &l }

var _ actor.Actor = (*OMS)(nil)

type OMS struct {
	actor.ActorBase
	cache *cache.Cache
}

func NewOMS(c *cache.Cache) *OMS {
	return &OMS{
		ActorBase: actor.NewActorBase("oms", []event.Topic{
			event.TopicEventOrderAccepted,
			event.TopicEventPartialFill,
			event.TopicEventFill,
			event.TopicEventOrderCanceled,
			event.TopicEventOrderRejected,
		}),
		cache: c,
	}
}

func (o *OMS) Handle(ev msgbus.Event, bus *msgbus.MsgBus) {
	switch ev.Ref.Topic {
	case event.TopicEventOrderAccepted:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderAccepted := event.NewOrderAcceptedFromBytes(buf)
		o.OnOrderAccepted(orderAccepted)
	case event.TopicEventPartialFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		orderPartiallyFilled := event.NewOrderPartiallyFilledFromBytes(buf)
		o.OnOrderPartiallyFilled(orderPartiallyFilled)
	case event.TopicEventFill:
		buf := bus.ReadBuffer(ev.Ref.Index, ev.Ref.Length)
		fill := event.NewFillFromBytes(buf)
		o.OnFill(fill)
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

func (o *OMS) OnInit() {
	log().Info().Msg("OMS initialized")
}

func (o *OMS) OnStart() {
	log().Info().Msg("OMS started")
}

func (o *OMS) OnStop() {
	log().Info().Msg("OMS stopped")
}

func (o *OMS) OnOrderAccepted(ev event.OrderAccepted) {
	log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order accepted")
}

func (o *OMS) OnOrderPartiallyFilled(ev event.OrderPartiallyFilled) {
	log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order partially filled")
}

func (o *OMS) OnFill(ev event.Fill) {
	log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Fill")
}

func (o *OMS) OnOrderCanceled(ev event.OrderCanceled) {
	log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order canceled")
}

func (o *OMS) OnOrderRejected(ev event.OrderRejected) {
	log().Info().Int("clientOrderID", ev.ClientOrderID).Int("orderID", ev.OrderID).Msg("Order rejected")
}
