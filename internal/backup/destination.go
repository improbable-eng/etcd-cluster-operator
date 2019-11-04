package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"cloud.google.com/go/storage"
)

// Destination is the place to store the backup.
type Destination interface {
	// Upload uploads the file on disk to this destination. It takes the local `path' at which the backup is currently
	// sotred.
	Upload(ctx context.Context, source string) error

	// IsUploaded checks if the backup has already been uploaded to this destination.
	IsUploaded(ctx context.Context) (bool, error)

	// RemotePath returns a string representing the location that the backup has been/will be placed.
	RemotePath() (string, error)
}

// LocalVolumeDestination is a Destination which allows backups to be stored in a local filesystem volume.
type LocalVolumeDestination struct {
	// The absolute path to a file in which the backup will be placed. If the directory containing this file does not
	// exist, it will be created.
	Path string
}

func (d *LocalVolumeDestination) Upload(ctx context.Context, source string) error {
	r, err := os.Open(source)
	if err != nil {
		return err
	}
	defer r.Close()

	pathDir := filepath.Dir(d.Path)
	if _, err := os.Stat(pathDir); os.IsNotExist(err) {
		err = os.MkdirAll(pathDir, os.ModeDir|0755)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	w, err := os.Create(d.Path)
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}

	return nil
}

func (d *LocalVolumeDestination) IsUploaded(ctx context.Context) (bool, error) {
	if _, err := os.Stat(d.Path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (d *LocalVolumeDestination) RemotePath() (string, error) {
	absPath, err := filepath.Abs(d.Path)
	if err != nil {
		return "", err
	}

	return absPath, nil
}

// GCSDestination is a Destination which allows backups to be stored in Google Cloud Storage.
type GCSDestination struct {
	// The name of the GCS bucket to place the backup.
	Bucket string
	// The name of the object inside of the bucket. This can contain directories.
	BlobName string
}

func (d *GCSDestination) Upload(ctx context.Context, source string) error {
	r, err := os.Open(source)
	if err != nil {
		return err
	}
	defer r.Close()

	obj, err := d.objectClient(ctx)
	if err != nil {
		return err
	}

	w := obj.NewWriter(ctx)
	defer w.Close()

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}

	return nil
}

func (d *GCSDestination) IsUploaded(ctx context.Context) (bool, error) {
	obj, err := d.objectClient(ctx)
	if err != nil {
		return false, err
	}

	if _, err := obj.Attrs(ctx); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (d *GCSDestination) RemotePath() (string, error) {
	return fmt.Sprintf("gs://%s/%s", d.Bucket, d.BlobName), nil
}

func (d *GCSDestination) objectClient(ctx context.Context) (*storage.ObjectHandle, error) {
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}

	return gcsClient.Bucket(d.Bucket).Object(d.BlobName), nil
}
