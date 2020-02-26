package reconcilerevent

import (
	"fmt"

	etcdclient "go.etcd.io/etcd/client"
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
		K8sEventTypeNormal,
		"ServiceCreated",
		fmt.Sprintf("Created service with name %q", s.ServiceName))
}

type PeerCreatedEvent struct {
	Object   runtime.Object
	PeerName string
}

func (s *PeerCreatedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"PeerCreated",
		fmt.Sprintf("Created a new EtcdPeer with name %q", s.PeerName))
}

type PeerRemovedEvent struct {
	Object   runtime.Object
	PeerName string
}

func (s *PeerRemovedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"PeerRemoved",
		fmt.Sprintf("Removed EtcdPeer with name %q", s.PeerName))
}

type MemberAddedEvent struct {
	Object runtime.Object
	Member *etcdclient.Member
	Name   string
}

func (s *MemberAddedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"MemberAdded",
		fmt.Sprintf("Added a new member with name %q", s.Name))
}

type MemberRemovedEvent struct {
	Object runtime.Object
	Member *etcdclient.Member
	Name   string
}

func (s *MemberRemovedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"MemberRemoved",
		fmt.Sprintf("Removed a member with name %q", s.Name))
}
