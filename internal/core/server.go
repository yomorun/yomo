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
	"fmt"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lucas-clemente/quic-go"
	"github.com/yomorun/yomo/internal/frame"
	"github.com/yomorun/yomo/logger"
	"github.com/yomorun/yomo/zipper/tracing"
)

// Server 是 QUIC Server 的抽象，被 Zipper 使用
type Server struct {
	token              string
	stream             quic.Stream
	state              string
	funcs              *ConcurrentMap
	funcBuckets        map[int]string
	connSfnMap         sync.Map // key: ConnID, value: Sfn Name.
	counterOfDataFrame int64
	// logger             utils.Logger
}

func NewServer(name string) *Server {
	s := &Server{
		token:       name,
		funcs:       NewConcurrentMap(),
		funcBuckets: make(map[int]string, 0),
		connSfnMap:  sync.Map{},
	}
	once.Do(func() {
		s.init()
	})

	return s
}

func (s *Server) ListenAndServe(ctx context.Context, endpoint string) error {
	qconf := &quic.Config{
		Versions:                       []quic.VersionNumber{quic.Version1, quic.VersionDraft29},
		MaxIdleTimeout:                 time.Second * 3,
		KeepAlive:                      true,
		MaxIncomingStreams:             100000,
		MaxIncomingUniStreams:          100000,
		HandshakeIdleTimeout:           time.Second * 3,
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
		logger.Errorf("%s quic.ListenAddr on: %s, err=%v", ServerLogPrefix, endpoint, err)
		return err
	}
	defer listener.Close()
	logger.Printf("%s✅ Listening on: %s, QUIC: %v", ServerLogPrefix, listener.Addr(), qconf.Versions)

	s.state = ConnStateConnected

	for {
		// 有新的 YomoClient 连接时，创建一个 session
		sctx, cancel := context.WithCancel(ctx)
		defer cancel()

		session, err := listener.Accept(sctx)
		if err != nil {
			logger.Errorf("%screate session error: %v", ServerLogPrefix, err)
			sctx.Done()
			return err
		}

		connID := session.RemoteAddr().String()
		logger.Infof("%s❤️1/ new connection: %s", ServerLogPrefix, connID)

		go func(ctx context.Context, sess quic.Session) {
			for {
				logger.Infof("%s❤️2/ waiting for new stream", ServerLogPrefix)
				stream, err := sess.AcceptStream(ctx)
				if err != nil {
					// if client close the connection, then we should close the session
					logger.Errorf("%s❤️3/ %T on [stream] %v, deleting from s.funcs if this stream is [sfn]", ServerLogPrefix, err, err)
					// 检查当前连接是否为 sfn，如果是则需要删除已注册的 sfn
					if name, ok := s.connSfnMap.Load(connID); ok {
						s.funcs.Remove(name.(string))
						s.connSfnMap.Delete(connID)
					}
					break
				}
				defer stream.Close()
				// defer ctx.Done()
				logger.Infof("%s❤️4/ [stream:%d] created, connID=%s", ServerLogPrefix, stream.StreamID(), connID)
				// 监听 stream 并做处理
				s.handleSession(connID, session, stream)
				logger.Infof("%s❤️5/ [stream:%d] handleSession DONE", ServerLogPrefix, stream.StreamID())
			}
		}(sctx, session)
	}

	// logger.Errorf("%sXXXXXXXXXXXXXXX - zipper - XXXXXXXXXXXXXXXX: ", ServerLogPrefix, finalErr)
	// return finalErr
	return nil
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

func (s *Server) handleSession(connID string, session quic.Session, mainStream quic.Stream) {
	fs := NewFrameStream(mainStream)
	// check update for stream
	for {
		logger.Infof("%shandleSession 💚 waiting read next...", ServerLogPrefix)
		f, err := fs.ReadFrame()
		if err != nil {
			logger.Errorf("%s%T %v", ServerLogPrefix, err, err)
			if errors.Is(err, net.ErrClosed) {
				// if client close the connection, net.ErrClosed will be raise
				// by quic-go IdleTimeoutError after connection's KeepAlive config.
				// logger.Infof("[ERR] on [ParseFrame] %v", net.ErrClosed)
				break
			}
			// any error occurred, we should close the session
			// after this, session.AcceptStream() will raise the error
			// which specific in session.CloseWithError()
			mainStream.Close()
			session.CloseWithError(0xCC, err.Error())
			logger.Warnf("%ssession.Close()", ServerLogPrefix)
			break
		}

		frameType := f.Type()
		logger.Debugf("%stype=%s, frame=%# x", ServerLogPrefix, frameType, logger.BytesString(f.Encode()))
		switch frameType {
		case frame.TagOfHandshakeFrame:
			s.handleHandShakeFrame(connID, mainStream, session, f.(*frame.HandshakeFrame))
		case frame.TagOfPingFrame:
			s.handlePingFrame(mainStream, session, f.(*frame.PingFrame))
		case frame.TagOfDataFrame:
			s.handleDataFrame(mainStream, session, f.(*frame.DataFrame))
		default:
			logger.Errorf("%serr=%v, frame=%v", ServerLogPrefix, err, logger.BytesString(f.Encode()))
		}
	}
}

func (s *Server) StatsFunctions() map[string]*quic.Stream {
	return s.funcs.GetCurrentSnapshot()
}

func (s *Server) StatsCounter() int64 {
	return s.counterOfDataFrame
}

func (s *Server) handleHandShakeFrame(connID string, stream quic.Stream, session quic.Session, f *frame.HandshakeFrame) error {
	logger.Infof("%s ------> GOT ❤️ HandshakeFrame : %# x", ServerLogPrefix, f)
	logger.Infof("%sClientType=%# x, is %s", ServerLogPrefix, f.ClientType, ConnectionType(f.ClientType))
	// client type
	clientType := ConnectionType(f.ClientType)
	switch clientType {
	case ConnTypeSource:
	case ConnTypeStreamFunction:
		// 检查 name 是否有效，如果无效则需要关闭连接。
		if !s.validateHandshake(f) {
			// 校验无效，关闭连接
			stream.Close()
			session.CloseWithError(0xCC, "Didn't pass the handshake validation, ilegal!")
			// break
			return fmt.Errorf("Didn't pass the handshake validation, ilegal!")
		}

		// 校验成功，注册 sfn 给 SfnManager
		s.funcs.Set(f.Name, &stream)
		// 添加 conn 和 sfn 的映射关系
		s.connSfnMap.Store(connID, f.Name)

	case ConnTypeUpstreamZipper:
	default:
		// Step 1-4: 错误，不认识该 client-type，关闭连接
		logger.Errorf("%sClientType=%# x, ilegal!", ServerLogPrefix, f.ClientType)
		// stream.Close()
		// session.CloseWithError(0xCC, "Unknown ClientType, ilegal!")
		return fmt.Errorf("Unknown ClientType, ilegal!")
	}
	return nil
}

func (s *Server) handlePingFrame(stream quic.Stream, session quic.Session, f *frame.PingFrame) error {
	logger.Infof("%s------> GOT ❤️ PingFrame : %# x", ServerLogPrefix, f)
	return nil
}

func (s *Server) handleDataFrame(mainStream quic.Stream, session quic.Session, f *frame.DataFrame) error {
	currentIssuer := f.GetIssuer()
	// tracing
	span, err := tracing.NewRemoteTraceSpan(f.GetMetadata("TraceID"), f.GetMetadata("SpanID"), "server", fmt.Sprintf("handleDataFrame <-[%s]", currentIssuer))
	if err == nil {
		defer span.End()
	}
	// counter +1
	atomic.AddInt64(&s.counterOfDataFrame, 1)
	// 收到数据帧
	logger.Infof("%sframeType=%s, metadata=%s, issuer=%s, counter=%d", ServerLogPrefix, f.Type(), f.GetMetadatas(), currentIssuer, s.counterOfDataFrame)
	// 因为是Immutable Stream，按照规则发送给 sfn
	var j int
	for i, fn := range s.funcBuckets {
		// 发送给 currentIssuer 的下一个 sfn
		if fn == currentIssuer {
			j = i + 1
		}
	}
	// 表示要执行第一个 sfn
	if j == 0 {
		logger.Infof("%s1st sfn write to [%s] -> [%s]:", ServerLogPrefix, currentIssuer, s.funcBuckets[0])
		targetStream := s.funcs.Get(s.funcBuckets[0])
		if targetStream == nil {
			logger.Debugf("%ssfn[%s] stream is nil", ServerLogPrefix, s.funcBuckets[0])
			err := fmt.Errorf("sfn[%s] stream is nil", s.funcBuckets[0])
			return err
		}
		(*targetStream).Write(f.Encode())
		return nil
	}

	if len(s.funcBuckets[j]) == 0 {
		logger.Debugf("%sno sfn found, drop this data frame", ServerLogPrefix)
		err := errors.New("no sfn found, drop this data frame")
		return err
	}

	targetStream := s.funcs.Get(s.funcBuckets[j])
	logger.Infof("%swill write to: [%s] -> [%s], target is nil:%v", ServerLogPrefix, currentIssuer, s.funcBuckets[j], targetStream == nil)
	if targetStream != nil {
		(*targetStream).Write(f.Encode())
	}
	// TODO: 独立流测试
	// send data to downstream.
	// stream, err := session.OpenUniStream()
	// if err != nil {
	// 	logger.Error("[MergeStreamFunc] session.OpenUniStream failed", "stream-fn", currentIssuer, "err", err)
	// 	// pass the data to next `stream function` if the current stream has error.
	// 	// next <- data
	// 	// cancel the current session when error.
	// 	// cancel()
	// 	return
	// }

	// _, err = stream.Write(f.Encode())
	// stream.Close()
	// logger.Info("[MergeStreamFunc] session.ISOStream Write", "stream-fn", currentIssuer, "err", err)
	// if err != nil {
	// 	logger.Error("[MergeStreamFunc] YoMo-Zipper sent data to `stream-fn` failed.", "stream-fn", currentIssuer, "err", err)
	// 	// cancel the current session when error.
	// 	// cancel()
	// 	return
	// }
	// s.funcs.WriteToAll(f.Encode())
	return nil
}

func (s *Server) AddWorkflow(wfs ...Workflow) error {
	for _, wf := range wfs {
		s.funcBuckets[wf.Seq] = wf.Token
	}
	return nil
}

// validateHandshake validates if the handshake frame is valid.
func (s *Server) validateHandshake(f *frame.HandshakeFrame) bool {
	isValid := false
	for _, k := range s.funcBuckets {
		if k == f.Name {
			isValid = true
			break
		}
	}

	logger.Warnf("%svalidateHandshake(%v), result: %v", ServerLogPrefix, *f, isValid)
	return isValid
}

// generateTLSConfig Setup a bare-bones TLS config for the server
func generateTLSConfig(host ...string) *tls.Config {
	tlsCert, _ := generateCertificate(host...)

	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		ClientSessionCache: tls.NewLRUClientSessionCache(1),
		NextProtos:         []string{"yomo"},
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

// Notify
// func (s *Server) Notify() <-chan error {
// 	return s.notify
// }

/*
// createStreamFunc creates a `GetStreamFunc` for `Stream Function`.
func createStreamFunc(app App, connMap *sync.Map, connType ConnectionType) GetStreamFunc {
	f := func() (string, []streamFuncWithCancel) {
		// get from local cache.
		// if funcs, ok := streamFuncCache.Load(app.Name); ok {
		// 	return app.Name, funcs.([]streamFuncWithCancel)
		// }

		// get from connMap.
		conns := findConn(app, connMap, connType)
		funcs := make([]streamFuncWithCancel, len(conns))

		// if len(conns) == 0 {
		// 	streamFuncCache.Store(app.Name, funcs)
		// 	return app.Name, funcs
		// }

		i := 0
		for id, conn := range conns {
			funcs[i] = streamFuncWithCancel{
				addr:    conn.Addr,
				session: conn.Session,
				// cancel:  cancelStreamFunc(app.Name, conn, connMap, id),
			}
			i++
		}

		// streamFuncCache.Store(app.Name, funcs)
		return app.Name, funcs
	}

	return f
}

// IsMatched indicates if the connection is matched.
func findConn(app App, connMap *sync.Map, connType ConnectionType) map[string]*Conn {
	results := make(map[string]*Conn)
	connMap.Range(func(key, value interface{}) bool {
		c := value.(*Conn)
		if c.Conn.Name == app.Name && c.Conn.Type == connType {
			results[key.(string)] = c
		}
		return true
	})

	return results
}
*/

func (s *Server) init() {
	// tracing
	_, _, err := tracing.NewTracerProvider(s.token)
	if err != nil {
		logger.Errorf("tracing: %v", err)
	}
}
