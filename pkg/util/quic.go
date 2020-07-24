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
	"strings"
	"time"

	json "github.com/10cella/yomo-json-codec"
	"github.com/lucas-clemente/quic-go"
	quicGo "github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/pkg/plugin"
)

// YomoFrameworkStreamWriter is the stream of framework
type YomoFrameworkStreamWriter struct {
	Name   string
	Codec  *json.Codec
	Plugin plugin.YomoObjectPlugin
	io.Writer
}

func (w YomoFrameworkStreamWriter) Write(b []byte) (int, error) {
	var err error = nil
	var value interface{}
	var result interface{}

	w.Codec.Decoder(b)

	for {
		value, err = w.Codec.Read(w.Plugin.Mold())
		if err != nil {
			log.Panic(err)
			break
		}

		if value == nil {
			w.Codec.Refresh(w.Writer) // nolint
			break
		}

		//if value != nil {
		result, err = w.Plugin.Handle(value)
		if err != nil {
			log.Fatal(err)
		}
		//fmt.Println("handle result:", result)
		w.Codec.Write(w.Writer, result, w.Plugin.Mold()) // nolint
		//	break
		//}
	}
	return len(b), err
}

// QuicClient create new QUIC client
func QuicClient(endpoint string) (quicGo.Stream, error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true, // nolint
		NextProtos:         []string{"hq-29"},
		//NextProtos:         []string{"http/1.1"},
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
		Versions: []quicGo.VersionNumber{0xff00001d},
	}
	listener, err := quicGo.ListenAddr(endpoint, GenerateTLSConfig(endpoint), conf)

	if err != nil {
		panic(err)
	}

	for {
		sess, err := listener.Accept(context.Background())
		if err != nil {
			panic(err)
		}

		var stream quic.Stream
		for {
			stream, err = sess.AcceptStream(context.Background())
			if err != nil {
				if !strings.Contains(err.Error(), "No recent network activity") {
					panic(err)
				}
				log.Printf("%s", err.Error())
				time.Sleep(5 * time.Second)
				continue
			}
			break
		}

		go io.Copy(YomoFrameworkStreamWriter{plugin.Name(), codec, plugin, stream}, stream) // nolint
	}

}

// GenerateTLSConfig Setup a bare-bones TLS config for the server
func GenerateTLSConfig(host ...string) *tls.Config {
	tlsCert, _ := Certificate(host...)

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"hq-29"},
		// NextProtos:   []string{"http/1.1"},
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
