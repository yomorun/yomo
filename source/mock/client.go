package mock

import (
	"github.com/yomorun/yomo/source"
	mockserver "github.com/yomorun/yomo/zipper/mock"
)

// SendDataToYoMoServer sends data to YoMo-Zipper.
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
