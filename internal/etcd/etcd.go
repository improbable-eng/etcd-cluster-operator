package etcd

import (
	etcdclient "go.etcd.io/etcd/client"
)

// EtcdDailer is used to connect to etcd in the first place
type EtcdAPI interface {
	// Give an API that provides access to etcd membership API functions.
	MembershipAPI(config etcdclient.Config) (etcdclient.MembersAPI, error)
}

type ClientEtcdAPI struct{}

func (_ *ClientEtcdAPI) MembershipAPI(config etcdclient.Config) (etcdclient.MembersAPI, error) {
	client, err := etcdclient.New(config)
	if err != nil {
		return nil, err
	}

	return etcdclient.NewMembersAPI(client), nil
}
