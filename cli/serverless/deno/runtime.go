// Package deno provides a js/ts serverless runtime
package deno

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/yomorun/yomo"
)

func listen(path string) (*net.UnixListener, error) {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, err
	}
	return net.ListenUnix("unix", addr)
}

func accept(listener *net.UnixListener) ([]byte, *net.UnixConn, error) {
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

	observed := make([]byte, length)
	_, err = io.ReadFull(conn, observed)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	conn.SetReadDeadline(time.Time{})

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

func startSfn(name string, zipperAddr string, credential string, observed []byte, conn net.Conn, errCh chan<- error) (yomo.StreamFunction, error) {
	sfn := yomo.NewStreamFunction(
		name,
		yomo.WithZipperAddr(zipperAddr),
		yomo.WithObserveDataTags(observed...),
		yomo.WithCredential(credential),
	)

	sfn.SetHandler(
		func(data []byte) (byte, []byte) {
			err := binary.Write(conn, binary.LittleEndian, uint32(len(data)))
			if err != nil {
				errCh <- err
				return 0, nil
			}

			_, err = conn.Write(data)
			if err != nil {
				errCh <- err
				return 0, nil
			}

			return 0, nil
		},
	)

	sfn.SetErrorHandler(
		func(err error) {
			log.Printf("[flow][%s] error handler: %T %v\n", zipperAddr, err, err)
		},
	)

	err := sfn.Connect()
	if err != nil {
		return nil, err
	}

	return sfn, nil
}

func runResponse(conn net.Conn, sfn yomo.StreamFunction, errCh chan<- error) {
	var length uint32
	tag := make([]byte, 1)

	for {
		_, err := io.ReadFull(conn, tag)
		if err != nil {
			errCh <- err
			return
		}

		err = binary.Read(conn, binary.LittleEndian, &length)
		if err != nil {
			errCh <- err
			return
		}

		data := make([]byte, length)
		_, err = io.ReadFull(conn, data)
		if err != nil {
			errCh <- err
			return
		}

		sfn.Write(tag[0], data)
	}
}

func run(name string, zipperAddr string, credential string, jsPath string, socketPath string) error {
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

	go runResponse(conn, sfn, errCh)

	err = <-errCh
	return err
}
