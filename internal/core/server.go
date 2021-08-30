package core

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

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
)

const (
	ServerLogPrefix = "\033[32m[core:server]\033[0m "
)

// Server 是 QUIC Server 的抽象，被 Zipper 使用
type Server struct {
	stream quic.Stream
	state  string
	// logger             utils.Logger
	funcs              *ConcurrentMap
	counterOfDataFrame int64
}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) ListenAndServe(ctx context.Context, endpoint string) error {
	s.funcs = NewConcurrentMap()

	qconf := &quic.Config{
		Versions:                       []quic.VersionNumber{quic.Version1},
		MaxIdleTimeout:                 time.Second * 30,
		KeepAlive:                      true,
		MaxIncomingStreams:             1000000,
		MaxIncomingUniStreams:          1000000,
		HandshakeIdleTimeout:           time.Second * 10,
		InitialStreamReceiveWindow:     1024 * 1024 * 2,
		InitialConnectionReceiveWindow: 1024 * 1024 * 2,
		DisablePathMTUDiscovery:        true,
		// Tracer:                         getQlogConfig("server"),
	}

	// if os.Getenv("YOMO_QLOG") != "" {
	// 	s.logger.Debugf("YOMO_QLOG=%s", os.Getenv("YOMO_QLOG"))
	// 	qconf.Tracer = getQlogConfig("server")
	// }

	// listen the address
	listener, err := quic.ListenAddr(endpoint, generateTLSConfig(endpoint), qconf)
	if err != nil {
		return err
	}
	defer listener.Close()
	logger.Printf("%s✅ Listening on: %s", ServerLogPrefix, listener.Addr())

	s.state = ConnStateConnected

	var finalErr error = nil

	for {
		// 有新的 YomoClient 连接时，创建一个 session
		sctx, cancel := context.WithCancel(ctx)
		defer cancel()

		session, err := listener.Accept(sctx)
		if err != nil {
			logger.Errorf("%screate session error: %v", ServerLogPrefix, err)
			// sctx.Done()
			cancel()
			finalErr = err
			break
		}
		logger.Infof("%s❤️1/ new connection: %s", ServerLogPrefix, session.RemoteAddr())

		go func(sess quic.Session, cancel context.CancelFunc) {
			defer cancel()
			for {
				logger.Infof("%s❤️2/ waiting for new stream", ServerLogPrefix)
				stream, err := sess.AcceptStream(sctx)
				if err != nil {
					// if client close the connection, then we should close the session
					logger.Errorf("%s❤️3/ [ERR] on [stream] %v, deleting from s.funcs if this stream is [sfn]", ServerLogPrefix, err)
					s.funcs.Remove(sess.RemoteAddr().String())
					break
				}
				defer stream.Close()
				// defer sctx.Done()
				logger.Infof("%s❤️4/ [stream:%d] created", ServerLogPrefix, stream.StreamID())
				// 监听 stream 并做处理
				s.handleSession(stream, session)
				logger.Infof("%s❤️5/ [stream:%d] handleSession DONE", ServerLogPrefix, stream.StreamID())
			}
		}(session, cancel)
	}

	logger.Errorf("%sXXXXXXXXXXXXXXX - zipper - XXXXXXXXXXXXXXXX: ", ServerLogPrefix, finalErr)
	return finalErr
}

func (s *Server) Close() error {
	if s.stream != nil {
		if err := s.stream.Close(); err != nil {
			logger.Errorf("%sClose(): %v", ServerLogPrefix, err)
			return err
		}
	}
	return nil
}

func (s *Server) handleSession(stream quic.Stream, session quic.Session) {
	fs := NewFrameStream(stream)
	for {
		logger.Printf("%shandleSession 💚 waiting read next..", ServerLogPrefix)
		f, err := fs.ReadFrame()
		if err != nil {
			logger.Errorf("%son [ParseFrame] %v", ServerLogPrefix, err)
			if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				// logger.Infof("[ERR] on [ParseFrame] %v", net.ErrClosed)
				break
			}
			// any error occurred, we should close the session
			// after this, session.AcceptStream() will raise the error
			// which specific in session.CloseWithError()
			stream.Close()
			session.CloseWithError(0xCC, err.Error())
			logger.Warnf("%ssession.Close()", ServerLogPrefix)
			break
		}

		frameType := f.Type()
		logger.Debug(ServerLogPrefix, "type", frameType.String(), "frame", logger.BytesString(f.Encode()))
		switch frameType {
		// Step 1：handshake frame
		case frame.TagOfHandshakeFrame:
			v := f.(*frame.HandshakeFrame)
			logger.Infof("%s ------> GOT ❤️ HandshakeFrame : %# x", ServerLogPrefix, v)
			// 判断 client-type
			if v.ClientType == byte(ConnTypeSource) {
				// Step 1-1：如果是 `source`，则接收消息，并转发
				logger.Infof("%sClientType=%# x, is source", ServerLogPrefix, v.ClientType)
			} else if v.ClientType == byte(ConnTypeStreamFunction) {
				// Step 1-2：如果是 `sfn`，则开始转发数据模式，TODO：immutable stream、流经 sfn 而不会被 handler 所 block
				logger.Infof("%sClientType=%# x, is sfn", ServerLogPrefix, v.ClientType)
				// 注册 sfn 给 SfnManager
				s.funcs.Set(session.RemoteAddr().String(), &stream)
			} else if v.ClientType == byte(ConnTypeUpstreamZipper) {
				// Step 1-3：如果是 `upstream zipper`，则并行转发消息
				logger.Infof("%sClientType=%# x, is upstream zipper", ServerLogPrefix, v.ClientType)
			} else {
				// Step 1-4: 错误，不认识该 client-type，关闭连接
				logger.Errorf("%sClientType=%# x, ilegal!", ServerLogPrefix, v.ClientType)
				break
			}

			// Step 2: PingFrame
		case frame.TagOfPingFrame:
			if v, ok := f.(*frame.PingFrame); ok {
				// 收到心跳
				logger.Infof("%s------> GOT ❤️ PingFrame : %# x", ServerLogPrefix, v)
			}

			// Step 3: DataFrame
		case frame.TagOfDataFrame:
			if v, ok := f.(*frame.DataFrame); ok {
				// counter +1
				s.counterOfDataFrame++
				// 收到数据帧
				logger.Infof("%s------> GOT ❤️ DataFrame: %# x, seqNum(%d)", ServerLogPrefix, v, s.counterOfDataFrame)
				// 因为是Immutable Stream，按照规则发送给 sfn
				// for k, target := range s.sfnCollection {
				// 	s.logger.Infof("\t💚 send to sfn: %s, frame: %v", k, v)
				// 	target.Write(v.Encode())
				// }
				s.funcs.WriteToAll(v.Encode())
			}
		default:
			logger.Errorf("%sunknown signal.", "frame", ServerLogPrefix, logger.BytesString(f.Encode()))
		}
	}
}

func (s *Server) StatsFunctions() map[string]*quic.Stream {
	return s.funcs.GetCurrentSnapshot()
}

func (s *Server) StatsCounter() int64 {
	return s.counterOfDataFrame
}

// generateTLSConfig Setup a bare-bones TLS config for the server
func generateTLSConfig(host ...string) *tls.Config {
	tlsCert, _ := generateCertificate(host...)

	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
		NextProtos:         []string{"spdy/3", "h2", "hq-29"},
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
