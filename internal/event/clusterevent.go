package event

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// EtcdClusterReconcilerEvent represents the action of the operator having actually done anything. Any meaningful change
// to the cluster should result in one of these.
type EtcdClusterReconcilerEvent interface {

	// Record this into an event recorder
	Record(recorder record.EventRecorder)
}

type ServiceCreatedEvent struct {
	Object      runtime.Object
	ServiceName string
}

func (s *ServiceCreatedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		"Normal",
		"ServiceCreated",
		fmt.Sprintf("Created service with name '%s'", s.ServiceName))
}

type PeerCreatedEvent struct {
	Object   runtime.Object
	PeerName string
}

func (s *PeerCreatedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		"Normal",
		"PeerCreated",
		fmt.Sprintf("Created a new EtcdPeer with name '%s'", s.PeerName))
}
