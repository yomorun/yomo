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
	"errors"
	"math/big"
	"net"
	"time"

	quicGo "github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/logger"
)

type quicGoServer struct {
	handler  ServerHandler
	listener quicGo.Listener
}

func (s *quicGoServer) SetHandler(handler ServerHandler) {
	s.handler = handler
}

func (s *quicGoServer) ListenAndServe(ctx context.Context, addr string) error {
	// Lock to use QUIC draft-29 version
	conf := &quicGo.Config{
		Versions:                []quicGo.VersionNumber{quicGo.Version1},
		MaxIdleTimeout:          time.Minute * 10080,
		KeepAlive:               true,
		MaxIncomingStreams:      1000000,
		MaxIncomingUniStreams:   1000000,
		DisablePathMTUDiscovery: true,
	}

	// listen the address
	listener, err := quicGo.ListenAddr(addr, generateTLSConfig(addr), conf)
	if err != nil {
		return err
	}
	s.listener = listener

	// serve
	logger.Print("✅ Listening on " + addr)

	if s.handler != nil {
		s.handler.Listen()
	}

	for {
		ctx, cancel := context.WithCancel(context.Background())
		session, err := s.listener.Accept(ctx)
		if err != nil {
			cancel()
			return err
		}

		go func(session quicGo.Session, cancel context.CancelFunc) {
			defer cancel()
			id := time.Now().UnixNano()

			for {
				stream, err := session.AcceptStream(context.Background())
				if err != nil {
					break
				}
				defer stream.Close()
				if s.handler != nil {
					s.handler.Read(id, session, stream)
				} else {
					logger.Print("handler isn't set in QUIC server")
					break
				}
			}
		}(session, cancel)
	}
}

type quicGoClient struct {
	session quicGo.Session
}

func (c *quicGoClient) Connect(addr string) error {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"spdy/3", "h2", "hq-29"},
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
	}

	session, err := quicGo.DialAddr(addr, tlsConf, &quicGo.Config{
		MaxIdleTimeout:        time.Minute * 10080,
		KeepAlive:             true,
		MaxIncomingStreams:    1000000,
		MaxIncomingUniStreams: 1000000,
		TokenStore:            quicGo.NewLRUTokenStore(1, 1),
	})

	if err != nil {
		return err
	}
	c.session = session
	return nil
}

func (c *quicGoClient) AcceptStream(ctx context.Context) (Stream, error) {
	if c.session == nil {
		return nil, errors.New("[QUIC client] session is nil")
	}

	return c.session.AcceptStream(ctx)
}

func (c *quicGoClient) AcceptUniStream(ctx context.Context) (ReceiveStream, error) {
	if c.session == nil {
		return nil, errors.New("[QUIC client] session is nil")
	}

	return c.session.AcceptUniStream(ctx)
}

func (c *quicGoClient) CreateStream(ctx context.Context) (Stream, error) {
	if c.session == nil {
		return nil, errors.New("[QUIC client] session is nil")
	}

	return c.session.OpenStream()
}

func (c *quicGoClient) CreateUniStream(ctx context.Context) (SendStream, error) {
	if c.session == nil {
		return nil, errors.New("[QUIC client] session is nil")
	}

	return c.session.OpenUniStream()
}

func (c *quicGoClient) Close() error {
	return c.session.CloseWithError(0, "")
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

// Close the server. All active sessions will be closed.
func (s *quicGoServer) Close() error {
	if s.listener != nil {
		logger.Debug("quicGoServer closing...")
		return s.listener.Close()
	}
	return nil
}
