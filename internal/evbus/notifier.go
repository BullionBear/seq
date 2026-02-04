package evbus

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// StateNotifier publishes engine state events to the EventBus.
// It provides a convenient interface for engines to report their lifecycle states
// (Ready, Stop, Finished, Abnormal) through the event bus system.
type StateNotifier struct {
	eventBus *EventBus
}

// NewStateNotifier creates a new StateNotifier with the given EventBus.
func NewStateNotifier(eventBus *EventBus) *StateNotifier {
	return &StateNotifier{eventBus: eventBus}
}

// NotifyReady publishes a ReadyEvent indicating an engine is ready.
func (n *StateNotifier) NotifyReady(source common.EngineType, timestamp uint64) {
	size := ReadyEventSize()
	offset, buf := n.eventBus.Allocate(size)
	SerializeReadyEvent(buf, &event.ReadyEvent{
		Source:    source,
		Timestamp: timestamp,
	})
	n.eventBus.Publish(EventRef{
		Topic:  event.TopicEventReady,
		Index:  offset,
		Length: size,
	})
}

// NotifyStop publishes a StopEvent indicating an engine is stopping.
func (n *StateNotifier) NotifyStop(source common.EngineType, timestamp uint64) {
	size := StopEventSize()
	offset, buf := n.eventBus.Allocate(size)
	SerializeStopEvent(buf, &event.StopEvent{
		Source:    source,
		Timestamp: timestamp,
	})
	n.eventBus.Publish(EventRef{
		Topic:  event.TopicEventStop,
		Index:  offset,
		Length: size,
	})
}

// NotifyFinished publishes a FinishedEvent indicating an engine has finished.
func (n *StateNotifier) NotifyFinished(source common.EngineType, timestamp uint64) {
	size := FinishedEventSize()
	offset, buf := n.eventBus.Allocate(size)
	SerializeFinishedEvent(buf, &event.FinishedEvent{
		Source:    source,
		Timestamp: timestamp,
	})
	n.eventBus.Publish(EventRef{
		Topic:  event.TopicEventFinished,
		Index:  offset,
		Length: size,
	})
}

// NotifyAbnormal publishes an AbnormalEvent indicating an engine encountered an error.
func (n *StateNotifier) NotifyAbnormal(source common.EngineType, errorCode int, timestamp uint64) {
	size := AbnormalEventSize()
	offset, buf := n.eventBus.Allocate(size)
	SerializeAbnormalEvent(buf, &event.AbnormalEvent{
		Source:    source,
		ErrorCode: errorCode,
		Timestamp: timestamp,
	})
	n.eventBus.Publish(EventRef{
		Topic:  event.TopicEventAbnormal,
		Index:  offset,
		Length: size,
	})
}
