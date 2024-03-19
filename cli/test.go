/*
Copyright © 2021 Allegro Networks

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/pkg/log"

	// serverless registrations
	_ "github.com/yomorun/yomo/cli/serverless/deno"
	_ "github.com/yomorun/yomo/cli/serverless/golang"
	_ "github.com/yomorun/yomo/cli/serverless/wasm"
)

var (
	sfnDir       []string
	userPrompt   string
	systemPrompt string
	aiServerAddr string
)

// testPromptCmd represents the test prompt command for LLM function
// the source code of the LLM function is in the sfnDir
var testPromptCmd = &cobra.Command{
	Use:     "test-prompt",
	Aliases: []string{"p"},
	Short:   "Test LLM prompt",
	Long:    "Test LLM prompt",
	Run: func(cmd *cobra.Command, args []string) {
		// sfn source directory
		if len(sfnDir) == 0 {
			sfnDir = append(sfnDir, ".")
		}
		for _, dir := range sfnDir {
			// run sfn
			log.InfoStatusEvent(os.Stdout, "--------------------------------------------------------")
			log.InfoStatusEvent(os.Stdout, "Run AI SFN on directory: %v", dir)
			cmd := exec.Command("go", "run", ".")
			cmd.Dir = dir
			env := os.Environ()
			env = append(env, "YOMO_LOG_LEVEL=info")
			cmd.Env = env
			// cmd.Stdout = io.Discard
			// cmd.Stderr = io.Discard
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Setpgid: true,
			}
			outputReader, err := cmd.StdoutPipe()
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to run AI SFN on directory: %v, error: %v", dir, err)
				continue
			}
			// read outputReader
			output := make(chan []byte)
			defer close(output)
			go func(output chan []byte) {
				outputBuf := make([]byte, 1024)
				for {
					outputLen, err := outputReader.Read(outputBuf)
					if err != nil {
						break
					}
					if len(outputBuf[:outputLen]) > 0 {
						output <- outputBuf[:outputLen]
					}
				}
			}(output)
			// start cmd
			if err := cmd.Start(); err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to run AI SFN on directory: %v, error: %v", dir, err)
				continue
			} else {
				defer func(cmd *exec.Cmd) {
					pgid, err := syscall.Getpgid(cmd.Process.Pid)
					if err == nil {
						syscall.Kill(-pgid, syscall.SIGTERM)
					} else {
						cmd.Process.Kill()
					}
				}(cmd)
			}
			// wait for the sfn to be ready
			for {
				select {
				case out := <-output:
					// log.InfoStatusEvent(os.Stdout, "AI SFN Output: %s", out)
					if len(out) > 0 && strings.Contains(string(out), "register ai function success") {
						log.InfoStatusEvent(os.Stdout, "Register AI function success")
						goto REQUEST
					}
				case <-time.After(5 * time.Second):
					log.FailureStatusEvent(os.Stdout, "Connect to zipper failed, please check the zipper is running or not")
					os.Exit(1)
				}
			}
			// invoke llm api
			// request
		REQUEST:
			apiEndpoint := fmt.Sprintf("%s/invoke", aiServerAddr)
			log.InfoStatusEvent(os.Stdout, `Invoke LLM API "%s"`, apiEndpoint)
			invokeReq := ai.InvokeRequest{
				ReturnRaw: true, // return raw response
				Prompt:    userPrompt,
			}
			reqBuf, err := json.Marshal(invokeReq)
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to marshal invoke request: %v", err)
				continue
			}
			// invoke api endpoint
			log.InfoStatusEvent(os.Stdout, ">> LLM API Request")
			log.InfoStatusEvent(os.Stdout, "Messages:")
			log.InfoStatusEvent(os.Stdout, "\tSystem: %s", systemPrompt)
			log.InfoStatusEvent(os.Stdout, "\tUser: %s", userPrompt)
			resp, err := http.Post(apiEndpoint, "application/json", bytes.NewBuffer(reqBuf))
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to invoke llm api: %v", err)
				continue
			}
			defer resp.Body.Close()
			// response
			// failed to invoke llm api
			log.InfoStatusEvent(os.Stdout, "<< LLM API Response")
			if resp.StatusCode != http.StatusOK {
				var errorResp ai.ErrorResponse
				err := json.NewDecoder(resp.Body).Decode(&errorResp)
				if err != nil {
					log.FailureStatusEvent(os.Stdout, "Failed to decode llm api response: %v", err)
					continue
				}
				log.FailureStatusEvent(os.Stdout, "Failed to invoke llm api response: %s", errorResp.Error)
				continue
			}
			// success to invoke llm api
			var invokeResp ai.InvokeResponse
			if err := json.NewDecoder(resp.Body).Decode(&invokeResp); err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to decode llm api response: %v", err)
				continue
			}
			for tag, tcs := range invokeResp.ToolCalls {
				toolCallCount := len(tcs)
				if toolCallCount > 0 {
					log.InfoStatusEvent(os.Stdout, "Tag: %v", tag)
					log.InfoStatusEvent(os.Stdout, "Invoke functions[%d]:", toolCallCount)
					for i, tc := range tcs {
						log.InfoStatusEvent(os.Stdout,
							"\t[%d] name: %s, arguments: %v",
							i,
							tc.Function.Name,
							tc.Function.Arguments,
						)
					}
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(testPromptCmd)

	testPromptCmd.Flags().StringSliceVarP(&sfnDir, "sfn", "", []string{}, "sfn source directory")
	testPromptCmd.Flags().StringVarP(&userPrompt, "user-prompt", "u", "", "user prompt")
	testPromptCmd.MarkFlagRequired("user-prompt")
	testPromptCmd.Flags().StringVarP(
		&systemPrompt,
		"system-prompt",
		"s",
		`You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous. If you don't know the answer, stop the conversation by saying "no func call"`,
		"system prompt",
	)
	testPromptCmd.Flags().StringVarP(&aiServerAddr, "ai-server", "a", "http://localhost:8000", "LLM API server address")

	runViper = bindViper(testPromptCmd)
}
