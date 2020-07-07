package util

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"time"

	"github.com/lucas-clemente/quic-go"
	quicGo "github.com/lucas-clemente/quic-go"
)

func QuicClient(endpoint string) (quicGo.Stream, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true, // nolint
		NextProtos:         []string{"http/1.1"},
	}

	session, err := quicGo.DialAddr(endpoint, tlsConf, &quic.Config{
		MaxIdleTimeout: time.Minute * 2,
		KeepAlive:      true,
	})

	if err != nil {
		return nil, err
	}

	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		return nil, err
	}

	return stream, nil
}

func QuicServer(endpoint string, w io.Writer, r io.Reader) {
	listener, err := quicGo.ListenAddr(endpoint, GenerateTLSConfig(endpoint), nil)
	if err != nil {
		panic(err)
	}

	for {
		sess, err := listener.Accept(context.Background())
		if err != nil {
			panic(err)
		}
		stream, err := sess.AcceptStream(context.Background())
		if err != nil {
			panic(err)
		}

		go io.Copy(w, stream) // nolint
		go io.Copy(stream, r) // nolint
	}

}

// GenerateTLSConfig Setup a bare-bones TLS config for the server

func GenerateTLSConfig(host ...string) *tls.Config {
	tlsCert, _ := Certificate(host...)

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"http/1.1"},
		//ServerName:   "echo.cella.fun",
	}
}

func Certificate(host ...string) (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour * 24 * 365)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"YoMo"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range host {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	template.IsCA = true
	template.KeyUsage |= x509.KeyUsageCertSign

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// create public key
	certOut := bytes.NewBuffer(nil)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	if err != nil {
		return tls.Certificate{}, err
	}

	// create private key
	keyOut := bytes.NewBuffer(nil)
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(certOut.Bytes(), keyOut.Bytes())
}
