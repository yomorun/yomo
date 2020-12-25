package wf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/yomorun/yomo-codec-golang/pkg/codes"
	"github.com/yomorun/yomo/internal/conf"
	"github.com/yomorun/yomo/pkg/quic"
	"github.com/yomorun/yomo/pkg/rx"
)

type baseOptions struct {
	// Config is the name of workflow config file (default is workflow.yaml).
	Config string
}

func parseConfig(opts *baseOptions, args []string) (*conf.WorkflowConfig, error) {
	if len(args) >= 1 && strings.HasSuffix(args[0], ".yaml") {
		// the second arg of `yomo wf dev xxx.yaml` is a .yaml file.
		opts.Config = args[0]
	}

	// validate opts.Config
	if opts.Config == "" {
		return nil, errors.New("Please input the file name of workflow config")
	}

	if !strings.HasSuffix(opts.Config, ".yaml") {
		return nil, errors.New(`The extension of workflow config is incorrect, it should ".yaml"`)
	}

	// parse workflow.yaml
	wfConf, err := conf.Load(opts.Config)
	if err != nil {
		return nil, errors.New("Parse the workflow config failure with the error: " + err.Error())
	}

	err = validateConfig(wfConf)
	if err != nil {
		return nil, err
	}

	return wfConf, nil
}

func validateConfig(wfConf *conf.WorkflowConfig) error {
	if wfConf == nil {
		return errors.New("conf is nil")
	}

	m := map[string][]conf.App{
		"Sources": wfConf.Sources,
		"Actions": wfConf.Actions,
		"Sinks":   wfConf.Sinks,
	}

	missingApps := []string{}
	missingParams := []string{}
	for k, apps := range m {
		if len(apps) == 0 {
			missingApps = append(missingApps, k)
		} else {
			for _, app := range apps {
				if app.Name == "" || app.Host == "" || app.Port <= 0 {
					missingParams = append(missingParams, k)
				}
			}
		}
	}

	errMsg := ""
	if wfConf.Name == "" || wfConf.Host == "" || wfConf.Port <= 0 {
		errMsg = "Missing name, host or port in workflow config. "
	}
	if len(missingApps) > 0 {
		errMsg += "Missing apps in " + strings.Join(missingApps, ", "+". ")
	}
	if len(missingApps) > 0 {
		errMsg += "Missing name, host or port in " + strings.Join(missingApps, ", "+". ")
	}

	if errMsg != "" {
		return errors.New(errMsg)
	}

	return nil
}

func run(wfConf *conf.WorkflowConfig) error {
	// TODO: multi sources
	// sourceApp := wfConf.Sources[0]
	// sourceStream, err := connectToApp(sourceApp)
	// if err != nil {
	// 	log.Print("❌ Connect to Source " + sourceApp.Name + " failure with err: ", err)
	// }

	// actions
	actionStreams := []quic.Stream{}
	for _, app := range wfConf.Actions {
		actionStream, err := connectToApp(app)
		if err != nil {
			log.Print("❌ Connect to Action "+app.Name+" failure with err: ", err)
		} else {
			actionStreams = append(actionStreams, actionStream)
		}
	}

	// sinks
	sinkStreams := []quic.Stream{}
	for _, app := range wfConf.Sinks {
		sinkStream, err := connectToApp(app)
		if err != nil {
			log.Print("❌ Connect to Sink "+app.Name+" failure with err: ", err)
		} else {
			sinkStreams = append(sinkStreams, sinkStream)
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

		fmt.Println(string(customer.V.([]byte)))
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
