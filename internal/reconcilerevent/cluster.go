package reconcilerevent

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
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

type ServiceMonitorCreatedEvent struct {
	Object             runtime.Object
	ServiceMonitorName string
}

func (s *ServiceMonitorCreatedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"ServiceMonitorCreated",
		fmt.Sprintf("Created service monitor with name %q", s.ServiceMonitorName))
}

type PDBPatchedEvent struct {
	Object       runtime.Object
	PDBName      string
	MinAvailable int
}

func (s *PDBPatchedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"PDBPatched",
		fmt.Sprintf("Patched pdb with name %q, MinAvailable: %d", s.PDBName, s.MinAvailable))
}

type PDBCreatedEvent struct {
	Object       runtime.Object
	PDBName      string
	MinAvailable int
}

func (s *PDBCreatedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"PDBCreated",
		fmt.Sprintf("Created pdb with name %q, MinAvailable: %d", s.PDBName, s.MinAvailable))
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
	Member *etcd.Member
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
	Member *etcd.Member
	Name   string
}

func (s *MemberRemovedEvent) Record(recorder record.EventRecorder) {
	recorder.Event(s.Object,
		K8sEventTypeNormal,
		"MemberRemoved",
		fmt.Sprintf("Removed a member with name %q", s.Name))
}
