package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"time"

	"github.com/yomorun/yomo-codec-golang/pkg/codes"
	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

// Run runs the workflow by config (.yaml).
func Run(wfConf *conf.WorkflowConfig) error {
	// TODO: multi sources
	// sourceApp := wfConf.Sources[0]
	// sourceStream, err := connectToApp(sourceApp)
	// if err != nil {
	// 	log.Print(getConnectFailedMsg("Source", sourceApp), err)
	// }

	// actions
	actionStreams := []quic.Stream{}
	for _, app := range wfConf.Actions {
		actionStream, err := connectToApp(app)
		if err != nil {
			log.Print(getConnectFailedMsg("Action", app), err)
		} else {
			actionStreams = append(actionStreams, actionStream)
			log.Print(fmt.Sprintf("✅ Connect to %s successfully.", getAppInfo("Action", app)))
		}
	}

	// sinks
	sinkStreams := []quic.Stream{}
	for _, app := range wfConf.Sinks {
		sinkStream, err := connectToApp(app)
		if err != nil {
			log.Print(getConnectFailedMsg("Sink", app), err)
		} else {
			sinkStreams = append(sinkStreams, sinkStream)
			log.Print(fmt.Sprintf("✅ Connect to %s successfully.", getAppInfo("Sink", app)))
		}
	}

	// validate
	// if sourceStream == nil {
	// 	return nil, errors.New("Not available sources")
	// }
	if len(actionStreams) == 0 {
		return errors.New("Not available actions")
	}
	if len(sinkStreams) == 0 {
		return errors.New("Not available sinks")
	}

	// build rxStream
	rxStream := rx.FromReaderWithFunc(mockSource())
	for _, stream := range actionStreams {
		s := func() io.ReadWriter { return stream }
		rxStream = rxStream.MergeReadWriterWithFunc(s)
	}

	for _, stream := range sinkStreams {
		s := func() io.ReadWriter { return stream }
		rxStream = rxStream.MergeReadWriterWithFunc(s)
	}

	// observe stream
	for customer := range rxStream.Observe() {
		if customer.Error() {
			log.Print(customer.E.Error())
		}

		log.Print(string(customer.V.([]byte)))
	}

	return nil
}

var protoCodec = codes.NewProtoCodec(0x10)

func mockSource() func() io.Reader {
	f := func() io.Reader {
		sendingBuf, _ := protoCodec.Marshal((rand.New(rand.NewSource(time.Now().UnixNano())).Float32() * 200))
		r := bytes.NewReader(sendingBuf)
		time.Sleep(100 * time.Millisecond)
		return r
	}
	return f
}

func connectToApp(app conf.App) (quic.Stream, error) {
	client, err := quic.NewClient(fmt.Sprintf("%s:%d", app.Host, app.Port))
	if err != nil {
		return nil, err
	}

	return client.CreateStream(context.Background())
}

func getConnectFailedMsg(appType string, app conf.App) string {
	return fmt.Sprintf("❌ Connect to %s failure with err: ",
		getAppInfo(appType, app))
}

func getAppInfo(appType string, app conf.App) string {
	return fmt.Sprintf("%s %s (%s:%d)",
		appType,
		app.Name,
		app.Host,
		app.Port)
}
