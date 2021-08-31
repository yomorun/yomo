package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yomorun/yomo/cli/serverless"
)

func parseURL(url string, opts *serverless.Options) error {
	if url == "" {
		url = "localhost:9000"
	}
	splits := strings.Split(url, ":")
	if len(splits) != 2 {
		return fmt.Errorf(`The format of url "%s" is incorrect, it should be "host:port", f.e. localhost:9000`, url)
	}
	host := splits[0]
	port, _ := strconv.Atoi(splits[1])
	opts.Host = host
	opts.Port = port
	return nil
}
