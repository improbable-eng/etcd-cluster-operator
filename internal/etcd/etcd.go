package etcd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/blang/semver"
	clientv2 "go.etcd.io/etcd/client"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/pkg/logutil"
)

type Config struct {
	Endpoints []string
	TLS       *tls.Config
}

type Versions struct {
	Server  string `json:"etcdserver"`
	Cluster string `json:"etcdcluster"`
}

type ClientEtcdAPI struct {
	Config   Config
	ClientV2 clientv2.Client
	ClientV3 *clientv3.Client
}

func (c *ClientEtcdAPI) List(ctx context.Context) ([]Member, error) {
	var members []Member
	if c.ClientV3 != nil {
		m, err := c.ClientV3.MemberList(ctx)
		if err != nil {
			return nil, err
		}

		for _, clusterMember := range m.Members {
			member := Member{
				ID:         strconv.FormatUint(clusterMember.ID, 10),
				Name:       clusterMember.Name,
				PeerURLs:   clusterMember.PeerURLs,
				ClientURLs: clusterMember.ClientURLs,
			}

			members = append(members, member)
		}
	} else if c.ClientV2 != nil {
		api := clientv2.NewMembersAPI(c.ClientV2)
		m, err := api.List(ctx)
		if err != nil {
			return nil, err
		}

		for _, clusterMember := range m {
			member := Member{
				ID:         clusterMember.ID,
				Name:       clusterMember.Name,
				PeerURLs:   clusterMember.PeerURLs,
				ClientURLs: clusterMember.ClientURLs,
			}

			members = append(members, member)
		}
	}

	return members, nil
}

func (c *ClientEtcdAPI) Add(ctx context.Context, peerURL string) (*Member, error) {
	var member Member
	if c.ClientV3 != nil {
		m, err := c.ClientV3.MemberAdd(ctx, []string{peerURL})
		if err != nil {
			return nil, err
		}

		member = Member{
			ID:         strconv.FormatUint(m.Member.ID, 10),
			Name:       m.Member.Name,
			PeerURLs:   m.Member.PeerURLs,
			ClientURLs: m.Member.ClientURLs,
		}

	} else if c.ClientV2 != nil {
		api := clientv2.NewMembersAPI(c.ClientV2)
		m, err := api.Add(ctx, peerURL)
		if err != nil {
			return nil, err
		}

		member = Member{
			ID:         m.ID,
			Name:       m.Name,
			PeerURLs:   m.PeerURLs,
			ClientURLs: m.ClientURLs,
		}
	}

	return &member, nil
}

func (c *ClientEtcdAPI) Remove(ctx context.Context, memberID string) error {
	if c.ClientV3 != nil {
		u, err := strconv.ParseUint(memberID, 10, 64)
		if err != nil {
			return err
		}

		_, err = c.ClientV3.MemberRemove(ctx, u)
		if err != nil {
			return err
		}

	} else if c.ClientV2 != nil {
		api := clientv2.NewMembersAPI(c.ClientV2)
		err := api.Remove(ctx, memberID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *ClientEtcdAPI) GetVersion(ctx context.Context) (*Versions, error) {
	versions, err := checkVersion(c.Config)
	if err != nil {
		return nil, err
	}

	return versions, nil
}

func (c *ClientEtcdAPI) Close() error {
	if c.ClientV3 != nil {
		err := c.ClientV3.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func checkVersion(config Config) (*Versions, error) {
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 2 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   2 * time.Second,
		IdleConnTimeout:       2 * time.Second,
		ResponseHeaderTimeout: 2 * time.Second,
	}

	if config.TLS != nil {
		tr.TLSClientConfig = config.TLS
	}

	httpClient := &http.Client{
		Transport: tr,
		Timeout:   2 * time.Second,
	}

	// always get the first endpoint
	url := fmt.Sprintf("%s/version", config.Endpoints[0])
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// allowed response code are HTTP 200
	if resp.StatusCode == http.StatusOK {
		var versions Versions

		err := json.NewDecoder(resp.Body).Decode(&versions)
		if err != nil {
			return nil, err
		}

		return &versions, nil
	} else {
		return nil, errors.New(fmt.Sprintf("error while get version 200 expected, but got %d", resp.StatusCode))
	}
}

// API contains only the ETCD APIs that we use in the operator
// to allow for testing fakes.
type API interface {
	// List enumerates the current cluster membership.
	List(ctx context.Context) ([]Member, error)

	// Add instructs etcd to accept a new Member into the cluster.
	Add(ctx context.Context, peerURL string) (*Member, error)

	// Remove demotes an existing Member out of the cluster.
	Remove(ctx context.Context, mID string) error

	// GetVersion retrieves the current etcd server and cluster version
	GetVersion(ctx context.Context) (*Versions, error)

	// Close the client
	Close() error
}

// APIBuilder is used to connect to etcd in the first place
type APIBuilder interface {
	New(Config) (API, error)
}

var _ API = &ClientEtcdAPI{}

type ClientEtcdAPIBuilder struct{}

var _ APIBuilder = &ClientEtcdAPIBuilder{}

func (o *ClientEtcdAPIBuilder) New(config Config) (API, error) {
	versions, err := checkVersion(config)
	if err != nil {
		return nil, err
	}

	version, err := semver.Make(versions.Server)
	if err != nil {
		return nil, err
	}

	var cliV2 clientv2.Client
	var cliV3 *clientv3.Client
	if version.Major == 3 && version.Minor <= 2 {
		transport := &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			Dial: (&net.Dialer{
				Timeout:   2 * time.Second,
				KeepAlive: 2 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   2 * time.Second,
			IdleConnTimeout:       2 * time.Second,
			ResponseHeaderTimeout: 2 * time.Second,
		}

		if config.TLS != nil {
			transport.TLSClientConfig = config.TLS
		}

		cfg := clientv2.Config{
			Endpoints:               config.Endpoints,
			Transport:               transport,
			HeaderTimeoutPerRequest: 2 * time.Second,
		}
		cliV2, err = clientv2.New(cfg)
		if err != nil {
			return nil, err
		}
	} else if version.Major == 3 && version.Minor > 2 {
		logConfig := logutil.DefaultZapLoggerConfig
		logConfig.Sampling = nil
		cfg := clientv3.Config{
			Endpoints:            config.Endpoints,
			DialTimeout:          2 * time.Second,
			DialKeepAliveTime:    2 * time.Second,
			DialKeepAliveTimeout: 2 * time.Second,
			LogConfig:            &logConfig,
		}

		if config.TLS != nil {
			cfg.TLS = config.TLS
		}

		cliV3, err = clientv3.New(cfg)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("etcd version not compatible")
	}

	client := &ClientEtcdAPI{
		Config:   config,
		ClientV2: cliV2,
		ClientV3: cliV3,
	}

	return client, nil
}
