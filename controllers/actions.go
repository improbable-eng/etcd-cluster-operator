package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/improbable-eng/etcd-cluster-operator/internal/reconcilerevent"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
