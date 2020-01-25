package cluster

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type State struct {
}

func GetState(log logr.Logger, c client.Client, ctx context.Context, req ctrl.Request) (*State, error) {
	state := &State{}
	return state, nil
}
