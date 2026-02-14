package evbus

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// EventPublisher is the interface required by StateNotifier.
// It abstracts the event publishing functionality so that both
// EventBus and MsgBus can be used.
type EventPublisher interface {
	Allocate(size uint64) (offset uint64, buffer []byte)
	Publish(ref EventRef)
}

// StateNotifier publishes engine state events to the event bus.
// It provides a convenient interface for engines to report their lifecycle states
// (Ready, Stop, Finished, Abnormal) through the event bus system.
type StateNotifier struct {
	publisher EventPublisher
}

// NewStateNotifier creates a new StateNotifier with the given EventPublisher.
// Both *EventBus and *msgbus.MsgBus satisfy EventPublisher.
func NewStateNotifier(publisher EventPublisher) *StateNotifier {
	return &StateNotifier{publisher: publisher}
}

// NotifyReady publishes a ReadyEvent indicating an engine is ready.
func (n *StateNotifier) NotifyReady(source common.EngineType, timestamp uint64) {
	size := ReadyEventSize()
	offset, buf := n.publisher.Allocate(size)
	SerializeReadyEvent(buf, &event.ReadyEvent{
		Source:    source,
		Timestamp: timestamp,
	})
	n.publisher.Publish(EventRef{
		Topic:  event.TopicEventReady,
		Index:  offset,
		Length: size,
	})
}

// NotifyStop publishes a StopEvent indicating an engine is stopping.
func (n *StateNotifier) NotifyStop(source common.EngineType, timestamp uint64) {
	size := StopEventSize()
	offset, buf := n.publisher.Allocate(size)
	SerializeStopEvent(buf, &event.StopEvent{
		Source:    source,
		Timestamp: timestamp,
	})
	n.publisher.Publish(EventRef{
		Topic:  event.TopicEventStop,
		Index:  offset,
		Length: size,
	})
}

// NotifyFinished publishes a FinishedEvent indicating an engine has finished.
func (n *StateNotifier) NotifyFinished(source common.EngineType, timestamp uint64) {
	size := FinishedEventSize()
	offset, buf := n.publisher.Allocate(size)
	SerializeFinishedEvent(buf, &event.FinishedEvent{
		Source:    source,
		Timestamp: timestamp,
	})
	n.publisher.Publish(EventRef{
		Topic:  event.TopicEventFinished,
		Index:  offset,
		Length: size,
	})
}

// NotifyAbnormal publishes an AbnormalEvent indicating an engine encountered an error.
func (n *StateNotifier) NotifyAbnormal(source common.EngineType, errorCode int, timestamp uint64) {
	size := AbnormalEventSize()
	offset, buf := n.publisher.Allocate(size)
	SerializeAbnormalEvent(buf, &event.AbnormalEvent{
		Source:    source,
		ErrorCode: errorCode,
		Timestamp: timestamp,
	})
	n.publisher.Publish(EventRef{
		Topic:  event.TopicEventAbnormal,
		Index:  offset,
		Length: size,
	})
}
