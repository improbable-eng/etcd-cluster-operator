package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var backupUrlTests = []struct {
	blobUrl string
	bucket  string
	path    string
}{
	{"gs://foo/bar", "gs://foo", "/bar"},
	{"s3://foo/bar", "s3://foo", "/bar"},
	{"s3://foo/bar/baz.mkv", "s3://foo", "/bar/baz.mkv"},
	{"s3://mybucket/foo/bar.mkv?endpoint=my.minio.local:8080&disableSSL=true&s3ForcePathStyle=true",
		"s3://mybucket?endpoint=my.minio.local:8080&disableSSL=true&s3ForcePathStyle=true",
		"/foo/bar.mkv"},
}

func TestParseBackupURL(t *testing.T) {
	for _, tt := range backupUrlTests {
		t.Run(tt.blobUrl, func(t *testing.T) {
			actualBucket, actualPath, err := parseBackupURL(tt.blobUrl)
			assert.NoError(t, err)
			assert.Equal(t, tt.bucket, actualBucket)
			assert.Equal(t, tt.path, actualPath)
		})
	}
}
