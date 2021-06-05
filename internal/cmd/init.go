package cmd

import (
	"log"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// initOptions are the options for build init.
type initOptions struct {
	appName string
}

// NewCmdInit inits a new command version.
func NewCmdInit() *cobra.Command {
	var opts = &initOptions{}

	var cmd = &cobra.Command{
		Use:   "init",
		Short: "Initialize a YoMo Serverless Application",
		Long:  "Initialize a YoMo Serverless Application.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) >= 1 && args[0] != "" {
				// the second arg of `yomo build xxx` is the app name.
				opts.appName = args[0]
			}

			if opts.appName == "" {
				log.Print("❌ Please input your app name.")
				return
			}

			log.Print("Initializing the Serverless app...")
			// create folder
			err := os.Mkdir(opts.appName, 0755)
			if err != nil {
				log.Print("❌ Create the app folder failure with the error: ", err)
				return
			}

			// change working directory by app name.
			err = os.Chdir(opts.appName)
			if err != nil {
				log.Print("❌ Change the working directory into "+opts.appName+" failure with the error: ", err)
				return
			}

			// create app.go
			f, err := os.Create("app.go")
			if err != nil {
				log.Print("❌ Create the app.go file failure with the error: ", err)
				return
			}

			_, err = f.WriteString(exampleServerlessFunc)
			if err != nil {
				log.Print("❌ Write serverless function into app.go file failure with the error: ", err)
				return
			}

			// go mod
			modCmd := exec.Command("go", "mod", "init", opts.appName)
			err = modCmd.Run()
			if err != nil {
				log.Print("❌ Generate go.mod file failure with the error: ", err)
				return
			}

			// download dependencies
			modCmd = exec.Command("go", "mod", "tidy")
			err = modCmd.Run()
			if err != nil {
				log.Print("🛠 go.mod tidy err: ", err)
				return
			}

			log.Print("✅ Congratulations! You have initialized the serverless app successfully.")
			log.Print("🎉 You can enjoy the YoMo Serverless via the command: yomo dev")
		},
	}

	cmd.Flags().StringVarP(&opts.appName, "name", "n", "", "The name of Serverless app")

	return cmd
}

var exampleServerlessFunc = `package main

import (
	"context"
	"fmt"
	"time"

	y3 "github.com/yomorun/y3-codec-golang"
	"github.com/yomorun/yomo/pkg/rx"
)

// NoiseDataKey represents the Tag of a Y3 encoded data packet
const NoiseDataKey = 0x10

// NoiseData represents the structure of data
type NoiseData struct {
	Noise float32 ` + "`y3:\"0x11\"`" + `
	Time  int64   ` + "`y3:\"0x12\"`" + `
	From  string  ` + "`y3:\"0x13\"`" + `
}

var printer = func(_ context.Context, i interface{}) (interface{}, error) {
	value := i.(NoiseData)
	rightNow := time.Now().UnixNano() / int64(time.Millisecond)
	fmt.Println(fmt.Sprintf("[%s] %d > value: %f ⚡️=%dms", value.From, value.Time, value.Noise, rightNow-value.Time))
	return value.Noise, nil
}

var callback = func(v []byte) (interface{}, error) {
	var mold NoiseData
	err := y3.ToObject(v, &mold)
	if err != nil {
		return nil, err
	}
	mold.Noise = mold.Noise / 10
	return mold, nil
}

// Handler will handle data in Rx way
func Handler(rxstream rx.RxStream) rx.RxStream {
	stream := rxstream.
		Subscribe(NoiseDataKey).
		OnObserve(callback).
		Debounce(50).
		Map(printer).
		StdOut().
		Encode(0x11)

	return stream
}
`
