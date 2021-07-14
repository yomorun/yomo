package mock

import (
	mockserver "github.com/yomorun/yomo/server/mock"
	"github.com/yomorun/yomo/source"
)

// SendDataToYoMoServer sends data to YoMo-Server.
func SendDataToYoMoServer(data []byte) error {
	cli := source.New("test source")
	defer cli.Close()

	// connect to server
	cli, err := cli.Connect(mockserver.IP, mockserver.Port)
	if err != nil {
		return err
	}

	// send data to server
	_, err = cli.Write(data)
	return err
}
