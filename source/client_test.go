package source

import (
	"testing"

	mockserver "github.com/yomorun/yomo/zipper/mock"
)

func TestSendDataToServer(t *testing.T) {
	go mockserver.New()

	cli := New("test source")
	defer cli.Close()

	// connect to server
	cli, err := cli.Connect(mockserver.IP, mockserver.Port)
	if err != nil {
		t.Errorf("[source.Connect] expected err is nil, but got %v", err)
	}

	// send data to server
	n, err := cli.Write([]byte("test"))
	if n <= 0 {
		t.Errorf("[source.Write] expected n > 0, but got %d", n)
	}
	if err != nil {
		t.Errorf("[source.Write] expected err is nil, but got %v", err)
	}
}
