package msgbus

import (
	"github.com/BullionBear/seq/core/model/common"
	"github.com/BullionBear/seq/core/model/event"
)

// EventPublisher is the interface required by StateNotifier.
// It abstracts the event publishing functionality so that both
// EventBus and MsgBus can be used.
type EventPublisher interface {
	Allocate(topic event.Topic, size uint64) (EventRef, []byte, bool)
	Publish(ref EventRef) bool
	Cancel(ref EventRef)
}

// StateNotifier publishes engine state events to the event bus.
// It provides a convenient interface for engines to report their lifecycle states
// (Ready, Stop, Finished, Abnormal) through the event bus system.
//
// State topics are critical-class: Allocate/Publish never drop them, so the
// ok returns below can only be false for droppable topics and are effectively
// always true here (kept for interface symmetry).
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
	e := event.ReadyEvent{Source: source, Timestamp: timestamp}
	ref, buf, ok := n.publisher.Allocate(event.TopicEventReady, uint64(e.GetBufferLength()))
	if !ok {
		return
	}
	if err := e.Encode(buf); err != nil {
		n.publisher.Cancel(ref)
		return
	}
	n.publisher.Publish(ref)
}

// NotifyStop publishes a StopEvent indicating an engine is stopping.
func (n *StateNotifier) NotifyStop(source common.EngineType, timestamp uint64) {
	e := event.StopEvent{Source: source, Timestamp: timestamp}
	ref, buf, ok := n.publisher.Allocate(event.TopicEventStop, uint64(e.GetBufferLength()))
	if !ok {
		return
	}
	if err := e.Encode(buf); err != nil {
		n.publisher.Cancel(ref)
		return
	}
	n.publisher.Publish(ref)
}

// NotifyFinished publishes a FinishedEvent indicating an engine has finished.
func (n *StateNotifier) NotifyFinished(source common.EngineType, timestamp uint64) {
	e := event.FinishedEvent{Source: source, Timestamp: timestamp}
	ref, buf, ok := n.publisher.Allocate(event.TopicEventFinished, uint64(e.GetBufferLength()))
	if !ok {
		return
	}
	if err := e.Encode(buf); err != nil {
		n.publisher.Cancel(ref)
		return
	}
	n.publisher.Publish(ref)
}

// NotifyAbnormal publishes an AbnormalEvent indicating an engine encountered an error.
func (n *StateNotifier) NotifyAbnormal(source common.EngineType, errorCode int, timestamp uint64) {
	e := event.AbnormalEvent{Source: source, ErrorCode: errorCode, Timestamp: timestamp}
	ref, buf, ok := n.publisher.Allocate(event.TopicEventAbnormal, uint64(e.GetBufferLength()))
	if !ok {
		return
	}
	if err := e.Encode(buf); err != nil {
		n.publisher.Cancel(ref)
		return
	}
	n.publisher.Publish(ref)
}
