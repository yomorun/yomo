// Package deno provides a js/ts serverless runtime
package deno

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/yomorun/yomo"
	"github.com/yomorun/yomo/core/frame"
	"github.com/yomorun/yomo/pkg/file"
	"github.com/yomorun/yomo/serverless"
)

func listen(path string) (*net.UnixListener, error) {
	err := file.Remove(path)
	if err != nil {
		return nil, err
	}

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}
	return net.ListenUnix("unix", addr)
}

func accept(listener *net.UnixListener) ([]frame.Tag, *net.UnixConn, error) {
	defer listener.Close()

	listener.SetUnlinkOnClose(true)
	listener.SetDeadline(time.Now().Add(3 * time.Second))

	conn, err := listener.AcceptUnix()
	if err != nil {
		return nil, nil, err
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var length uint32
	err = binary.Read(conn, binary.LittleEndian, &length)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}

	observedBytes := make([]byte, length*4)
	_, err = io.ReadFull(conn, observedBytes)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	conn.SetReadDeadline(time.Time{})

	observed := make([]frame.Tag, length)
	for i := 0; i < int(length); i++ {
		observed[i] = frame.Tag(binary.LittleEndian.Uint32(observedBytes[i*4 : i*4+4]))
	}

	return observed, conn, nil
}

func runDeno(jsPath string, socketPath string, errCh chan<- error) {
	cmd := exec.Command(
		"deno",
		"run",
		"--unstable",
		"--allow-read=.,"+socketPath,
		"--allow-write=.,"+socketPath,
		"--allow-env",
		"--allow-net",
		jsPath,
		socketPath,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		errCh <- err
	}
}

func startSfn(name string, zipperAddr string, credential string, observed []frame.Tag, conn net.Conn, errCh chan<- error) (yomo.StreamFunction, error) {
	sfn := yomo.NewStreamFunction(
		name,
		zipperAddr,
		yomo.WithSfnCredential(credential),
	)

	// init
	sfn.Init(func() error {
		return nil
	})

	sfn.SetObserveDataTags(observed...)

	sfn.SetHandler(
		func(ctx serverless.Context) {
			tag := ctx.Tag()
			err := binary.Write(conn, binary.LittleEndian, tag)
			if err != nil {
				errCh <- err
				return
			}

			data := ctx.Data()
			err = binary.Write(conn, binary.LittleEndian, uint32(len(data)))
			if err != nil {
				errCh <- err
				return
			}

			_, err = conn.Write(data)
			if err != nil {
				errCh <- err
				return
			}

			var length uint32
			for {
				err := binary.Read(conn, binary.LittleEndian, &tag)
				if err != nil {
					errCh <- err
					return
				}

				err = binary.Read(conn, binary.LittleEndian, &length)
				if err != nil {
					errCh <- err
					return
				}

				if tag == 0 && length == 0 {
					break
				}

				data := make([]byte, length)
				_, err = io.ReadFull(conn, data)
				if err != nil {
					errCh <- err
					return
				}

				ctx.Write(tag, data)
			}
		},
	)

	sfn.SetErrorHandler(
		func(err error) {
			log.Printf("[deno][%s] error handler: %T %v\n", zipperAddr, err, err)
		},
	)

	err := sfn.Connect()
	if err != nil {
		return nil, err
	}

	return sfn, nil
}

func run(name string, zipperAddr string, credential string, jsPath string, socketPath string) error {
	if _, err := exec.LookPath("deno"); err != nil {
		return errors.New("[deno] command was not found. For details, visit https://deno.land")
	}

	errCh := make(chan error)

	listener, err := listen(socketPath)
	if err != nil {
		return err
	}

	go runDeno(jsPath, socketPath, errCh)

	observed, conn, err := accept(listener)
	if err != nil {
		return err
	}
	defer conn.Close()

	sfn, err := startSfn(name, zipperAddr, credential, observed, conn, errCh)
	if err != nil {
		return err
	}
	defer sfn.Close()

	err = <-errCh
	return err
}
