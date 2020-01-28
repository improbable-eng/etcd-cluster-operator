package controllers

import (
	"context"
	"fmt"
	"net/url"

	"github.com/go-logr/logr"
	etcdclient "go.etcd.io/etcd/client"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/improbable-eng/etcd-cluster-operator/internal/etcd"
	"github.com/improbable-eng/etcd-cluster-operator/internal/reconcilerevent"
)

type Action interface {
	Execute(context.Context) error
}

type CreateRuntimeObject struct {
	log    logr.Logger
	client client.Client
	obj    runtime.Object
}

func (o *CreateRuntimeObject) Execute(ctx context.Context) error {
	o.log.Info("Creating resource", "type", o.obj.GetObjectKind().GroupVersionKind().String())
	err := o.client.Create(ctx, o.obj)
	if apierrs.IsAlreadyExists(err) {
		err = fmt.Errorf("stale cache error: object was not found in cache but creation failed with AlreadyExists error: %s", err)
	}
	return err
}

type AddEtcdMember struct {
	log    logr.Logger
	config etcdclient.Config
	etcd   etcd.EtcdAPI
	url    *url.URL
}

func (o *AddEtcdMember) Execute(ctx context.Context) error {
	c, err := o.etcd.MembershipAPI(o.config)
	if err != nil {
		return fmt.Errorf("unable to connect to etcd: %s", err)
	}

	_, err = c.Add(ctx, o.url.String())
	if err != nil {
		return fmt.Errorf("unable to add member to etcd cluster: %s", err)
	}
	return nil
}

type RemoveEtcdMember struct {
	log    logr.Logger
	config etcdclient.Config
	etcd   etcd.EtcdAPI
	mID    string
}

func (o *RemoveEtcdMember) Execute(ctx context.Context) error {
	o.log.Info("Removing member", "member-id", o.mID)
	c, err := o.etcd.MembershipAPI(o.config)
	if err != nil {
		return fmt.Errorf("unable to connect to etcd: %s", err)
	}

	err = c.Remove(ctx, o.mID)
	if err != nil {
		return fmt.Errorf("unable to remove member of etcd cluster: %s", err)
	}
	return nil
}

type EventWrapper struct {
	recorder record.EventRecorder
	event    reconcilerevent.ReconcilerEvent
	wrapped  Action
}

func (o *EventWrapper) Execute(ctx context.Context) error {
	if o.wrapped == nil {
		return nil
	}
	err := o.wrapped.Execute(ctx)
	if err != nil {
		return err
	}
	o.event.Record(o.recorder)
	return nil
}
