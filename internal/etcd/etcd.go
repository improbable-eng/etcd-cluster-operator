package etcd

import (
	"context"

	etcdclient "go.etcd.io/etcd/client"
)

// API contains only the ETCD APIs that we use in the operator
// to allow for testing fakes.
type API interface {
	// List enumerates the current cluster membership.
	List(ctx context.Context) ([]etcdclient.Member, error)

	// Add instructs etcd to accept a new Member into the cluster.
	Add(ctx context.Context, peerURL string) (*etcdclient.Member, error)

	// Remove demotes an existing Member out of the cluster.
	Remove(ctx context.Context, mID string) error
}

// APIBuilder is used to connect to etcd in the first place
type APIBuilder interface {
	New(etcdclient.Config) (API, error)
}

type ClientEtcdAPI struct {
	etcdclient.MembersAPI
}

var _ API = &ClientEtcdAPI{}

type ClientEtcdAPIBuilder struct{}

var _ APIBuilder = &ClientEtcdAPIBuilder{}

func (o *ClientEtcdAPIBuilder) New(config etcdclient.Config) (API, error) {
	client, err := etcdclient.New(config)
	if err != nil {
		return nil, err
	}
	return &ClientEtcdAPI{
		MembersAPI: etcdclient.NewMembersAPI(client),
	}, nil
}
