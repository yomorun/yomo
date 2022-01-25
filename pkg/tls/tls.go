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
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"time"
)

// Generate certificate by server host ip addresses and DNS names.
func GenerateCertificate(expireMonths uint, host ...string) (string, string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour * time.Duration(expireMonths*24*30))

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"YoMo"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::")},
		DNSNames:              []string{"localhost"},
	}

	for _, h := range host {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	// create public key
	certOut := bytes.NewBuffer(nil)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	if err != nil {
		return "", "", err
	}

	// create private key
	keyOut := bytes.NewBuffer(nil)
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	if err != nil {
		return "", "", err
	}

	return certOut.String(), keyOut.String(), nil
}

// Load tls certificate according to environment variables.
func GetTLSConfig() (*tls.Config, error) {
	var err error
	var cert, key []byte

	certPath := os.Getenv("YOMO_TLS_CERT_PATH")
	keyPath := os.Getenv("YOMO_TLS_KEY_PATH")
	if len(certPath) == 0 || len(keyPath) == 0 {
		env := os.Getenv("YOMO_ENV")
		if len(env) == 0 || env == "development" {
			cert, key = getDevCertAndKey()
		}
	} else {
		cert, err = ioutil.ReadFile(certPath)
		if err != nil {
			return nil, err
		}

		key, err = ioutil.ReadFile(keyPath)
		if err != nil {
			return nil, err
		}
	}

	if len(cert) == 0 || len(key) == 0 {
		return nil, errors.New("cannot load tls certificate")
	}

	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
		NextProtos:         []string{"yomo"},
	}, nil
}

// !!! For test only, do NOT use it in production environment !!!
func getDevCertAndKey() ([]byte, []byte) {
	return []byte(`-----BEGIN CERTIFICATE-----
MIIBpDCCAUqgAwIBAgIRAOEth6xOv1kpOuy6xMrcdAAwCgYIKoZIzj0EAwIwDzEN
MAsGA1UEChMEWW9NbzAeFw0yMjAxMjUwMzAyNTlaFw0zMTEyMDQwMzAyNTlaMA8x
DTALBgNVBAoTBFlvTW8wWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAThySyF4tnA
qBBCRXb6OqSZOfhWMqvJJUOI4YGTkLGIHdP2gdGigNzG4zE8zSzYQ34yaN/PiyDX
A6qaVd7h8d77o4GGMIGDMA4GA1UdDwEB/wQEAwICpDATBgNVHSUEDDAKBggrBgEF
BQcDATAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBSj6eRkPl2VKRPnMSPAhiNt
b9SrcjAsBgNVHREEJTAjgglsb2NhbGhvc3SHBH8AAAGHEAAAAAAAAAAAAAAAAAAA
AAAwCgYIKoZIzj0EAwIDSAAwRQIhALbGwA4X/N2hEY9gsu60UW1AsUS4QLYFGhdc
1d9cXFP8AiB+OOkZfresxFnWwDB8gJAjiep9X/9Ma4XhXZXiE4k+8Q==
-----END CERTIFICATE-----`),
		[]byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEICbh0gxW/oF3kCHv3TWJALGggT+pFZcAX1iqsRbLG2+XoAoGCCqGSM49
AwEHoUQDQgAE4cksheLZwKgQQkV2+jqkmTn4VjKrySVDiOGBk5CxiB3T9oHRooDc
xuMxPM0s2EN+Mmjfz4sg1wOqmlXe4fHe+w==
-----END EC PRIVATE KEY-----`)
}
