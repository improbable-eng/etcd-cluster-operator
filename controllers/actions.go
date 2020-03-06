package controllers

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Action is a single operation which will be executed by the controller.
type Action interface {
	Execute(context.Context) error
}

// CreateRuntimeObject is an Action which creates the supplied API object
type CreateRuntimeObject struct {
	client client.Client
	obj    runtime.Object
}

func (o *CreateRuntimeObject) Execute(ctx context.Context) error {
	if err := o.client.Create(ctx, o.obj); err != nil {
		return fmt.Errorf("error %q while creating object ", err)
	}
	return nil
}

// PatchStatus is an Action which patches the status of original with any
// changed status fields of new.
type PatchStatus struct {
	client   client.Client
	original runtime.Object
	new      runtime.Object
}

func (o *PatchStatus) Execute(ctx context.Context) error {
	if reflect.DeepEqual(o.original, o.new) {
		return nil
	}
	if err := o.client.Status().Patch(ctx, o.new, client.MergeFrom(o.original)); err != nil {
		return fmt.Errorf("error %q while patching status", err)
	}
	return nil
}
