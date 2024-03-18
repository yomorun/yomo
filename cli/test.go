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
		// TODO: go run
		for _, dir := range sfnDir {
			// run sfn
			log.InfoStatusEvent(os.Stdout, "Run AI SFN on directory: %v", dir)
			cmd := exec.Command("go", "run", ".")
			cmd.Dir = dir
			cmd.Env = os.Environ()
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to run AI SFN on directory: %v, error: %v", dir, err)
				continue
			} else {
				pid := cmd.Process.Pid
				log.InfoStatusEvent(os.Stdout, "AI SFN pid: %v", pid)
				defer func(cmd *exec.Cmd) {
					cmd.Process.Release()
					cmd.Process.Kill()
				}(cmd)
			}

			// invoke llm api
			// TODO: need to wait for the sfn to be ready
			time.Sleep(3000 * time.Millisecond)
			// request
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
			apiEndpoint := fmt.Sprintf("%s/invoke", aiServerAddr)
			resp, err := http.Post(apiEndpoint, "application/json", bytes.NewBuffer(reqBuf))
			if err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to invoke llm api: %v", err)
				continue
			}
			defer resp.Body.Close()
			// response
			var invokeResp ai.InvokeResponse
			if err := json.NewDecoder(resp.Body).Decode(&invokeResp); err != nil {
				log.FailureStatusEvent(os.Stdout, "Failed to decode llm api response: %v", err)
				continue
			}
			log.InfoStatusEvent(os.Stdout, "--------------------------------------------------------")
			log.InfoStatusEvent(os.Stdout, "Invoke llm api response")
			for tag, tcs := range invokeResp.ToolCalls {
				toolCallCount := len(tcs)
				log.InfoStatusEvent(os.Stdout, "Tag: %v, ToolCalls: %v", tag, toolCallCount)
				if toolCallCount > 0 {
					log.InfoStatusEvent(os.Stdout, "Functions[%d]:", len(tcs))
					for _, tc := range tcs {
						log.InfoStatusEvent(os.Stdout, "\tname: %s, args: %v, description: %s", tc.Function.Name, tc.Function.Arguments, tc.Function.Description)
					}
				}
			}
		}
		// for i, dir := range sfnDir {
		// 	log.InfoStatusEvent(os.Stdout, "sfn source[%d] directory: %v", i, dir)
		// }
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
