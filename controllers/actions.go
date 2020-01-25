package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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
