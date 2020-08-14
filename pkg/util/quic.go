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
	"log"
	"math/big"
	"net"
	"time"

	json "github.com/10cella/yomo-json-codec"
	"github.com/lucas-clemente/quic-go"
	quicGo "github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/pkg/plugin"
)

var logger = GetLogger("yomo::quic")

// YomoFrameworkStreamWriter is the stream of framework
type YomoFrameworkStreamWriter struct {
	Name   string
	Codec  *json.Codec
	Plugin plugin.YomoObjectPlugin
	io.Writer
}

func (w YomoFrameworkStreamWriter) Write(b []byte) (c int, e error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Errorf("Write error: %v, input data: %s", err, string(b[:]))
			c = 0
			e = err.(error)
		}
	}()

	var (
		err    error = nil
		value  interface{}
		result interface{}
		num    = 0
		sum    = 0
	)

	w.Codec.Decoder(b)

	for {
		value, err = w.Codec.Read(w.Plugin.Mold())
		if err != nil {
			logger.Errorf("Codec.Read error: %s", err.Error())
			return sum, nil
		}

		if value == nil {
			num, err = w.Codec.Refresh(w.Writer)
			if err != nil {
				logger.Errorf("Codec.Refresh error: %s", err.Error())
			}
			return sum + num, nil
		}

		result, err = w.process(value)
		if err != nil {
			logger.Errorf("Plugin.Handle error: %s", err.Error())
			// if plugin handle has error, then write the value of the original
			num, err = w.Codec.Write(w.Writer, value, w.Plugin.Mold())
			if err != nil {
				logger.Errorf("Codec.Write error: %s", err.Error())
				return 0, err
			}
			return sum + num, nil
		}

		logger.Debugf("Plugin.Handle result: %s", result) //debug:

		num, err = w.Codec.Write(w.Writer, result, w.Plugin.Mold())
		if err != nil {
			logger.Errorf("Codec.Write error: %s", err.Error())
			break
		}
		sum = sum + num
		num = 0
	}

	if sum > 0 {
		return sum, nil
	}

	if num > 0 {
		return num, nil
	}
	return 0, nil
}

func (w YomoFrameworkStreamWriter) process(value interface{}) (v interface{}, e error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Errorf("Plugin.Handle panic error: %v", err)
			e = err.(error)
		}
	}()

	result, err := w.Plugin.Handle(value)
	if err != nil {
		logger.Errorf("Plugin.Handle error: %s", err.Error())
	}
	return result, nil
}

// QuicClient create new QUIC client
func QuicClient(endpoint string) (quicGo.Stream, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true, // nolint
		NextProtos:         []string{"hq-29"},
		//NextProtos: []string{"http/1.1"},
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

// QuicServer create a QUIC server
func QuicServer(endpoint string, plugin plugin.YomoObjectPlugin, codec *json.Codec) {
	// Lock to use QUIC draft-29 version
	conf := &quic.Config{
		Versions:  []quicGo.VersionNumber{0xff00001d},
		KeepAlive: true,
	}
	listener, err := quicGo.ListenAddr(endpoint, GenerateTLSConfig(endpoint), conf)

	if err != nil {
		panic(err)
	}

	var n = 0
	for {
		n = n + 1
		logger.Debugf("QuicServer::loop[%v] Accept before", n)
		session, err := listener.Accept(context.Background())
		if err != nil {
			logger.Errorf("QuicServer::Accept error: %s", err.Error())
			continue
		}
		logger.Debugf("QuicServer::loop[%v] Accept after: %v", n, session.ConnectionState())

		logger.Debugf("QuicServer::loop[%v] AcceptStream before", n)
		stream, err := session.AcceptStream(context.Background())
		if err != nil {
			logger.Errorf("QuicServer::AcceptStream error: %s", err.Error())
			continue
		}
		logger.Debugf("QuicServer::loop[%v] AcceptStream after: %v", n, stream.StreamID())
		logger.Infof("QuicServer::Establish new Stream: StreamID=%v", stream.StreamID())

		go func() {
			go monitorContextErr(session, stream)
			yStream := YomoFrameworkStreamWriter{plugin.Name(), codec, plugin, stream}
			_, err = CopyTo(yStream, stream)
			//_, err = io.Copy(yStream, stream)
			if err != nil {
				closeSession(session, stream)
			}
		}()
	}
}

func monitorContextErr(session quicGo.Session, stream quicGo.Stream) {
	for {
		var err error = nil
		if session.Context().Err() != nil {
			err = session.Context().Err()
			logger.Errorf("session context error: %v", err)
		}
		if stream.Context().Err() != nil {
			err = stream.Context().Err()
			logger.Errorf("stream context error: %v", err)
		}
		if err != nil {
			closeSession(session, stream)
			break
		}
		time.Sleep(5 * time.Second)
	}

}

func closeSession(session quicGo.Session, stream quicGo.Stream) {
	var err error

	// close stream
	streamID := stream.StreamID()
	err = stream.Close()
	if err != nil {
		logger.Errorf("stream[%v] close error: %s", streamID, err.Error())
	} else {
		logger.Infof("stream[%v] closed", streamID)
	}

	// close session
	err = session.CloseWithError(0, "close session")
	if err != nil {
		logger.Errorf("close session error: %s", err.Error())
	}
}

// GenerateTLSConfig Setup a bare-bones TLS config for the server
func GenerateTLSConfig(host ...string) *tls.Config {
	tlsCert, _ := Certificate(host...)

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"hq-29"},
		//NextProtos: []string{"http/1.1"},
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

func CopyTo(dst io.Writer, src io.Reader) (written int64, err error) {
	var buf []byte
	size := 32 * 1024
	if l, ok := src.(*io.LimitedReader); ok && int64(size) > l.N {
		if l.N < 1 {
			size = 1
		} else {
			size = int(l.N)
		}
	}
	buf = make([]byte, size)

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				log.Printf("dst.Write error: %s", ew.Error())
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			log.Printf("src.Read error: %s", er.Error())
			break
		}
	}
	return written, err
}
