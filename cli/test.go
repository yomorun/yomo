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
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/spf13/cobra"
	"github.com/yomorun/yomo/ai"
	"github.com/yomorun/yomo/cli/serverless"
	"github.com/yomorun/yomo/pkg/log"
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
			dir, err := filepath.Abs(dir)
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to get absolute path of sfn directory: %v", err)
				continue
			}
			// run sfn
			log.InfoStatusEvent(os.Stdout, "--------------------------------------------------------")
			log.InfoStatusEvent(os.Stdout, "Attaching LLM function in directory: %v", dir)
			// build
			sfnSource := filepath.Join(dir, "app.go")
			buildCmd.Run(nil, []string{sfnSource})
			sfnBin := filepath.Join(dir, "sfn.yomo")
			defer os.RemoveAll(sfnBin)

			// run
			cmd := exec.Command(sfnBin)
			cmd.Dir = dir
			err = serverless.LoadEnvFile(dir)
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to load env file in directory: %v, error: %v", dir, err)
				continue
			}
			env := append(os.Environ(), "YOMO_LOG_LEVEL=info")
			cmd.Env = env
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to attach LLM function in directory: %v, error: %v", dir, err)
				continue
			}
			defer stdout.Close()
			outputReader := bufio.NewReader(stdout)
			// read outputReader
			output := make(chan string)
			defer close(output)
			go func(outputReader *bufio.Reader, output chan string) {
				for {
					line, err := outputReader.ReadString('\n')
					// log.InfoStatusEvent(os.Stdout, "LLM function output: %s", line)
					if err != nil {
						break
					}
					if len(line) > 0 {
						output <- line
					}
				}
			}(outputReader, output)
			// start cmd
			if err := cmd.Start(); err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to run LLM function in directory: %v, error: %v", dir, err)
				continue
			}
			log.InfoStatusEvent(os.Stdout, "Run LLM function success, pid: %v", cmd.Process.Pid)
			// wait for the sfn to be ready
			for {
				select {
				case out := <-output:
					// log.InfoStatusEvent(os.Stdout, "AI SFN Output: %s", out)
					if len(out) > 0 && strings.Contains(out, "register ai function success") {
						log.InfoStatusEvent(os.Stdout, "Register LLM function success")
						goto REQUEST
					}
				case <-time.After(15 * time.Second):
					log.FailureStatusEvent(os.Stdout, "Connect to zipper failed, please check the zipper is running or not")
					os.Exit(1)
				}
			}
			// invoke llm api
			// request
		REQUEST:
			apiEndpoint := fmt.Sprintf("%s/invoke", aiServerAddr)
			log.InfoStatusEvent(os.Stdout, `Invoking LLM API "%s"`, apiEndpoint)
			invokeReq := ai.InvokeRequest{
				IncludeCallStack: true, // include call stack
				Prompt:           userPrompt,
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
					log.FailureStatusEvent(os.Stdout, "Failed to decode LLM API response: %v", err)
					continue
				}
				log.FailureStatusEvent(os.Stdout, "Failed to invoke LLM API response: %s", errorResp.Error)
				continue
			}
			// success to invoke LLM API
			var invokeResp ai.InvokeResponse
			if err := json.NewDecoder(resp.Body).Decode(&invokeResp); err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to decode LLM API response: %v", err)
				continue
			}
			// tool calls
			for tag, tcs := range invokeResp.ToolCalls {
				toolCallCount := len(tcs)
				if toolCallCount > 0 {
					log.InfoStatusEvent(os.Stdout, "Invoking functions[%d]:", toolCallCount)
					for _, tc := range tcs {
						if invokeResp.ToolMessages == nil {
							log.InfoStatusEvent(os.Stdout,
								"\t[%s] tag: %d, name: %s, arguments: %s",
								tc.ID,
								tag,
								tc.Function.Name,
								tc.Function.Arguments,
							)
						} else {
							log.InfoStatusEvent(os.Stdout,
								"\t[%s] tag: %d, name: %s, arguments: %s\n🌟 result: %s",
								tc.ID,
								tag,
								tc.Function.Name,
								tc.Function.Arguments,
								getToolCallResult(tc, invokeResp.ToolMessages),
							)
						}
					}
				}
			}
			// finish reason
			log.InfoStatusEvent(os.Stdout, "Finish Reason: %s", invokeResp.FinishReason)
			log.InfoStatusEvent(os.Stdout, "Final Content: \n🤖 %s", invokeResp.Content)

			// cmd process
			p, err := process.NewProcess(int32(cmd.Process.Pid))
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to get process: %v, pid: %v", err, p.Pid)
				continue
			}
			if err := p.Terminate(); err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to terminate process: %v, pid: %v", err, p.Pid)
				continue
			}
			cmd.Wait()
		}
	},
}

func getToolCallResult(tc *openai.ToolCall, tms []ai.ToolMessage) string {
	result := ""
	for _, tm := range tms {
		if tm.ToolCallId == tc.ID {
			result = tm.Content
		}
	}
	return result
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
		`You are a very helpful assistant. Your job is to choose the best possible action to solve the user question or task. Don't make assumptions about what values to plug into functions. Ask for clarification if a user request is ambiguous.`,
		"system prompt",
	)
	testPromptCmd.Flags().StringVarP(&aiServerAddr, "ai-server", "a", "http://localhost:8000", "LLM API server address")
}
