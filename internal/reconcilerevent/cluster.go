package reconcilerevent

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

/*
 These events are produced by the cluster operator.
*/

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
