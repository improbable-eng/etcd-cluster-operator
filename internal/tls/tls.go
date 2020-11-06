package tls

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/pkg/errors"
)

const (
	Organization = "Etcd Cluster Operator CA"
)

var validityNotAfter = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// IssueCA returns CA certificate and key
func IssueCA() (caCert []byte, caKey []byte, err error) {
	rsaBits := 2048
	priv, err := rsa.GenerateKey(rand.Reader, rsaBits)
	if err != nil {
		return nil, nil, fmt.Errorf("generate rsa key: %v", err)
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate serial number for root")
	}
	subject := pkix.Name{
		Organization: []string{Organization},
	}
	caTemplate := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             time.Now(),
		NotAfter:              validityNotAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA certificate: %v", err)
	}
	certOut := &bytes.Buffer{}
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return nil, nil, fmt.Errorf("encode CA certificate: %v", err)
	}
	caCert = certOut.Bytes()

	caKeyOut := &bytes.Buffer{}
	caBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}
	err = pem.Encode(caKeyOut, caBlock)
	if err != nil {
		return nil, nil, fmt.Errorf("encode RSA private key: %v", err)
	}
	caKey = caKeyOut.Bytes()

	return caCert, caKey, nil
}


// Issue TLS certificate and TLS private key
func Issue(hosts []string, caCert []byte, caKey []byte) (tlsCert []byte, tlsKey []byte, err error) {
	caBlock, _ := pem.Decode(caCert)
	if caBlock == nil {
		return nil, nil, fmt.Errorf("failed to parse certificate PEM: %v", err)
	}

	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %v", err)
	}

	keyBlock, _ := pem.Decode(caKey)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to parse certificate key PEM: %v", err)
	}

	priv, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse private key: %v", err)
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate serial number for client")
	}
	subject := pkix.Name{
		Organization: []string{"Etcd Cluster Operator"},
	}

	tlsTemplate := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		Issuer:                ca.Issuer,
		NotBefore:             time.Now(),
		NotAfter:              validityNotAfter,
		DNSNames:              hosts,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, errors.Wrap(err, "generate client key")
	}
	tlsDerBytes, err := x509.CreateCertificate(rand.Reader, &tlsTemplate, ca, &clientKey.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}
	tlsCertOut := &bytes.Buffer{}
	err = pem.Encode(tlsCertOut, &pem.Block{Type: "CERTIFICATE", Bytes: tlsDerBytes})
	if err != nil {
		return nil, nil, fmt.Errorf("encode TLS  certificate: %v", err)
	}
	tlsCert = tlsCertOut.Bytes()

	keyOut := &bytes.Buffer{}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)}
	err = pem.Encode(keyOut, block)
	if err != nil {
		return nil, nil, fmt.Errorf("encode RSA private key: %v", err)
	}
	tlsKey = keyOut.Bytes()

	return tlsCert, tlsKey, nil
}
