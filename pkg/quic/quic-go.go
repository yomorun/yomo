package quic

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
	"log"
	"math/big"
	"net"
	"time"

	quicGo "github.com/lucas-clemente/quic-go"
)

type quicGoServer struct {
	handler ServerHandler
}

func (s *quicGoServer) SetHandler(handler ServerHandler) {
	s.handler = handler
}

func (s *quicGoServer) ListenAndServe(ctx context.Context, addr string) error {
	// Lock to use QUIC draft-29 version
	conf := &quicGo.Config{
		Versions:              []quicGo.VersionNumber{0xff00001d},
		MaxIdleTimeout:        time.Minute * 10080,
		KeepAlive:             true,
		MaxIncomingStreams:    1000000,
		MaxIncomingUniStreams: 1000000,
	}

	// listen the address
	listener, err := quicGo.ListenAddr(addr, generateTLSConfig(addr), conf)
	if err != nil {
		return err
	}

	// serve
	log.Print("QUIC Server listens on ", addr)
	for {
		session, err := listener.Accept(context.Background())
		if err != nil {
			return err
		}

		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			return err
		}

		if s.handler != nil {
			s.handler.Read(stream)
		} else {
			log.Print("handler isn't set in QUIC server")
		}
	}
}

// generateTLSConfig Setup a bare-bones TLS config for the server
func generateTLSConfig(host ...string) *tls.Config {
	tlsCert, _ := generateCertificate(host...)

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"hq-29"},
		//NextProtos: []string{"http/1.1"},
	}
}

func generateCertificate(host ...string) (tls.Certificate, error) {
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
