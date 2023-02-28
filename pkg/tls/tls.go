// Package tls provides tls config for yomo.
package tls

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

// CreateServerTLSConfig creates server tls config.
func CreateServerTLSConfig(host string) (*tls.Config, error) {
	// ca pool
	pool, err := getCACertPool()
	if err != nil {
		return nil, err
	}

	// server certificate
	tlsCert, err := getCertAndKey()
	if err != nil {
		return nil, err
	}

	if tlsCert == nil {
		tlsCert, err = generateCertificate(host)
		if err != nil {
			return nil, err
		}
	}

	clientAuth := tls.NoClientCert
	if verifyPeer() {
		clientAuth = tls.RequireAndVerifyClientCert
	}

	return &tls.Config{
		Certificates: []tls.Certificate{*tlsCert},
		ClientCAs:    pool,
		ClientAuth:   clientAuth,
		NextProtos:   []string{"yomo"},
	}, nil
}

// MustCreateClientTLSConfig creates client tls config, It is panic If error here.
func MustCreateClientTLSConfig() *tls.Config {
	conf, err := CreateClientTLSConfig()
	if err != nil {
		panic(err)
	}
	return conf
}

// CreateClientTLSConfig creates client tls config.
func CreateClientTLSConfig() (*tls.Config, error) {
	// ca pool
	pool, err := getCACertPool()
	if err != nil {
		return nil, err
	}

	// client certificate
	tlsCert, err := getCertAndKey()
	if err != nil {
		return nil, err
	}

	certificates := []tls.Certificate{}
	if tlsCert != nil {
		certificates = append(certificates, *tlsCert)
	}

	return &tls.Config{
		InsecureSkipVerify: !verifyPeer(),
		Certificates:       certificates,
		RootCAs:            pool,
		NextProtos:         []string{"yomo"},
		ClientSessionCache: tls.NewLRUClientSessionCache(0),
	}, nil
}

func verifyPeer() bool {
	return strings.ToLower(os.Getenv("YOMO_TLS_VERIFY_PEER")) == "true"
}

func getCACertPool() (*x509.CertPool, error) {
	var err error
	var caCert []byte

	caCertPath := os.Getenv("YOMO_TLS_CACERT_FILE")
	if len(caCertPath) == 0 {
		return nil, nil
	}

	caCert, err = os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}

	if len(caCert) == 0 {
		return nil, errors.New("tls: cannot load CA cert")
	}

	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caCert); !ok {
		return nil, errors.New("tls: cannot append CA cert to pool")
	}

	return pool, nil
}

func getCertAndKey() (*tls.Certificate, error) {
	var err error
	var cert, key []byte

	certPath := os.Getenv("YOMO_TLS_CERT_FILE")
	keyPath := os.Getenv("YOMO_TLS_KEY_FILE")
	if len(certPath) == 0 || len(keyPath) == 0 {
		return nil, nil
	}

	// certificate
	cert, err = os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	// private key
	key, err = os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	if len(cert) == 0 || len(key) == 0 {
		return nil, errors.New("tls: cannot load tls cert/key")
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}

	return &tlsCert, nil
}

func generateCertificate(host ...string) (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour * 24 * 365)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"YoMo"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	for _, h := range host {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	// create public key
	certOut := bytes.NewBuffer(nil)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return nil, err
	}

	// create private key
	keyOut := bytes.NewBuffer(nil)
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair(certOut.Bytes(), keyOut.Bytes())
	return &cert, err
}
