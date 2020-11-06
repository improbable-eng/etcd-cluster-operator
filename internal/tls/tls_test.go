package tls

import (
	"crypto/x509"
	"encoding/pem"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateCertificates(t *testing.T) {
	caCert, caKey, err := IssueCA()
	assert.NoError(t, err)

	caBlock, _ := pem.Decode(caCert)
	assert.NotNil(t, caBlock)

	ca, err := x509.ParseCertificate(caBlock.Bytes)
	assert.NoError(t, err)
	assert.Equal(t, ca.Issuer.Organization, []string{Organization})

	keyBlock, _ := pem.Decode(caKey)
	assert.NotNil(t, keyBlock)

	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	assert.NoError(t, err)
	assert.NotNil(t, key)

	tlsCert, tlsKey, err := Issue([]string{"etcd-0", "etcd-1"}, caCert, caKey)
	assert.NoError(t, err)

	tlsBlock, _ := pem.Decode(tlsCert)
	assert.NotNil(t, tlsBlock)

	tls, err := x509.ParseCertificate(tlsBlock.Bytes)
	assert.NoError(t, err)
	assert.Equal(t, tls.Issuer.Organization, []string{Organization})
	assert.Equal(t, tls.DNSNames, []string{"etcd-0", "etcd-1"})

	tlsKeyBlock, _ := pem.Decode(tlsKey)
	assert.NotNil(t, tlsKeyBlock)

	key, err = x509.ParsePKCS1PrivateKey(tlsKeyBlock.Bytes)
	assert.NoError(t, err)
	assert.NotNil(t, key)
}
