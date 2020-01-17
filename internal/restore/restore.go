package restore

import (
	"fmt"
	"go.etcd.io/etcd/etcdserver"
	"go.etcd.io/etcd/etcdserver/membership"
	"go.etcd.io/etcd/pkg/types"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

func Restore(restoreName string, dataDir string, downloadDir string, bucketName string, objectName string) error {
	lg, err := zap.NewProduction()
	if err != nil {
		return err
	}

	// Download snapshot file
	// The API might be generic, but we only support Google Cloud Storage right now, so just use that.


	// Restore
	urlmap, err := types.NewURLsMap(restoreCluster)
	if err != nil {
		return err
	}

	cfg := etcdserver.ServerConfig{
		InitialClusterToken: restoreClusterToken,
		InitialPeerURLsMap:  urlmap,
		PeerURLs:            types.MustNewURLs(strings.Split(restorePeerURLs, ",")),
		Name:                restoreName,
	}
	if err := cfg.VerifyBootstrap(); err != nil {
		return err
	}

	cl, cerr := membership.NewClusterFromURLsMap(restoreClusterToken, urlmap)
	if cerr != nil {
		return err
	}

	basedir := restoreDataDir
	if basedir == "" {
		basedir = restoreName + ".etcd"
	}

	waldir := restoreWalDir
	if waldir == "" {
		waldir = filepath.Join(basedir, "member", "wal")
	}
	snapdir := filepath.Join(basedir, "member", "snap")

	if _, err := os.Stat(basedir); err == nil {
		ExitWithError(ExitInvalidInput, fmt.Errorf("data-dir %q exists", basedir))
	}

	makeDB(snapdir, args[0], len(cl.Members()))
	makeWALAndSnap(waldir, snapdir, cl)
}