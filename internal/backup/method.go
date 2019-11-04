package backup

import (
	"context"
	"os"
	"path/filepath"

	etcd "github.com/coreos/etcd/clientv3"
	"github.com/coreos/etcd/clientv3/snapshot"
	"go.uber.org/zap"
)

// Method is a technique to use to perform a backup.
type Method interface {
	// Save takes the backup & saves it on disk at the given filename.
	Save(ctx context.Context, path string) error

	// IsSaved checks if the backup already exists on the local disk.
	IsSaved(ctx context.Context, path string) (bool, error)

	// Delete removes the image from the local disk.
	Delete(ctx context.Context, path string) error
}

// SnapshotMethod is a Method which uses the built-in etcd snapshot mechanisms to perform a backup.
type SnapshotMethod struct {
	// Endpoints holds a list of etcd endpoints, used when calling in to the etcd API.
	// Each should be of the format `scheme://host-name:port'.
	Endpoints []string
}

func (m *SnapshotMethod) Save(ctx context.Context, path string) error {
	pathDir := filepath.Dir(path)
	if _, err := os.Stat(pathDir); os.IsNotExist(err) {
		err = os.MkdirAll(pathDir, os.ModeDir|0755)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if err := snapshot.NewV3(zap.NewNop()).Save(ctx, etcd.Config{
		Endpoints: m.Endpoints,
	}, path); err != nil {
		return err
	}

	return nil
}

func (m *SnapshotMethod) IsSaved(ctx context.Context, path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (m *SnapshotMethod) Delete(ctx context.Context, path string) error {
	return os.Remove(path)
}
