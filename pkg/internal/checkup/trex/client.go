/*
 * This file is part of the kiagnose project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2023 Red Hat, Inc.
 *
 */

package trex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"
	"regexp"
	"strings"
	"time"

	expect "github.com/google/goexpect"

	"k8s.io/apimachinery/pkg/util/wait"
)

type consoleExpecter interface {
	SafeExpectBatchWithResponse(expected []expect.Batcher, timeout time.Duration) ([]expect.BatchRes, error)
}

type Client struct {
	consoleExpecter                  consoleExpecter
	trafficGeneratorPacketsPerSecond string
	testDuration                     time.Duration
	verbosePrintsEnabled             bool
}

type PortIdx int

const (
	SourcePort PortIdx = iota
	DestPort
)

const (
	shellPrompt  = "# "
	batchTimeout = 30 * time.Second
)

func NewClient(trafficGenConsoleExpecter consoleExpecter,
	trafficGeneratorPacketsPerSecond string,
	testDuration time.Duration,
	verbosePrintsEnabled bool) Client {
	return Client{
		consoleExpecter:                  trafficGenConsoleExpecter,
		trafficGeneratorPacketsPerSecond: trafficGeneratorPacketsPerSecond,
		testDuration:                     testDuration,
		verbosePrintsEnabled:             verbosePrintsEnabled,
	}
}

func (c Client) StartServer() error {
	command := "systemctl start " + SystemdUnitFileName
	_, err := c.consoleExpecter.SafeExpectBatchWithResponse([]expect.Batcher{
		&expect.BSnd{S: command + "\n"},
		&expect.BExp{R: shellPrompt},
	},
		batchTimeout,
	)
	return err
}

func (c Client) WaitForServerToBeReady(ctx context.Context) error {
	const (
		interval = 5 * time.Second
		timeout  = time.Minute
	)
	var err error
	ctxWithNewDeadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conditionFn := func(ctx context.Context) (bool, error) {
		if c.isServerRunning() {
			log.Printf("trex-server is now ready")
			return true, nil
		}
		if c.verbosePrintsEnabled {
			log.Printf("trex-server is not yet ready...")
		}
		return false, nil
	}
	if err = wait.PollImmediateUntilWithContext(ctxWithNewDeadline, interval, conditionFn); err != nil {
		if !errors.Is(err, wait.ErrWaitTimeout) {
			return err
		}
		if c.verbosePrintsEnabled {
			if logErr := c.printTrexServiceFailLogs(); logErr != nil {
				return logErr
			}
		}
		return fmt.Errorf("timeout waiting for trex-server to be ready")
	}
	return nil
}

func (c Client) ClearStats() (string, error) {
	return c.runTrexConsoleCmd("clear")
}

func (c Client) StartTraffic(port PortIdx) (string, error) {
	startTrafficCmd := c.getStartTrafficCmd(port)
	return c.runTrexConsoleCmd(startTrafficCmd)
}

func (c Client) GetGlobalStats() (GlobalStats, error) {
	const (
		globalStatsCommand    = "stats -g"
		globalStatsRequestKey = "get_global_stats"
	)
	globalStatsJSONString, err := c.runTrexConsoleCmdWithJSONResponse(globalStatsCommand, globalStatsRequestKey)
	if err != nil {
		return GlobalStats{}, fmt.Errorf("failed to get global stats json: %w", err)
	}

	if c.verbosePrintsEnabled {
		log.Printf("GetGlobalStats JSON Response:\n%s", globalStatsJSONString)
	}

	var gs GlobalStats
	err = json.Unmarshal([]byte(globalStatsJSONString), &gs)
	if err != nil {
		return GlobalStats{}, fmt.Errorf("failed to unmarshal global stats json: %w", err)
	}
	return gs, nil
}

func (c Client) GetPortStats(port PortIdx) (PortStats, error) {
	const (
		portStatsRequestKey = "get_port_stats"
	)
	portStatsJSONString, err := c.runTrexConsoleCmdWithJSONResponse(fmt.Sprintf("stats --port %d -p", port), portStatsRequestKey)
	if err != nil {
		return PortStats{}, fmt.Errorf("failed to get global stats json: %w", err)
	}

	if c.verbosePrintsEnabled {
		log.Printf("GetPortStats JSON Response:\n%s", portStatsJSONString)
	}

	var ps PortStats
	err = json.Unmarshal([]byte(portStatsJSONString), &ps)
	if err != nil {
		return PortStats{}, fmt.Errorf("failed to unmarshal port %d stats json: %w", port, err)
	}
	return ps, nil
}

func (c Client) isServerRunning() bool {
	const helpSubstring = "Console Commands"
	resp, err := c.runTrexConsoleCmd("help")
	if c.verbosePrintsEnabled {
		log.Printf("trex-console help resp:\n%s", resp)
	}
	if err != nil || !strings.Contains(resp, helpSubstring) {
		return false
	}
	return true
}

func (c Client) printTrexServiceFailLogs() error {
	var err error
	trexServiceStatus, err := c.getTrexServiceStatus()
	if err != nil {
		return fmt.Errorf("failed gathering systemctl service status after trex-server timeout: %w", err)
	}
	trexJournalctlLogs, err := c.getTrexServiceJournalctl()
	if err != nil {
		return fmt.Errorf("failed gathering trex.service related joutnalctl logs after trex-server timeout: %w", err)
	}
	log.Printf("timeout waiting for trex-server to be ready\n"+
		"systemd service status:\n%s\n"+
		"joutnalctl logs:\n%s", trexServiceStatus, trexJournalctlLogs)
	return nil
}

func (c Client) getTrexServiceStatus() (string, error) {
	command := fmt.Sprintf("systemctl status %s | cat", SystemdUnitFileName)
	resp, err := c.consoleExpecter.SafeExpectBatchWithResponse([]expect.Batcher{
		&expect.BSnd{S: command + "\n"},
		&expect.BExp{R: shellPrompt},
	},
		batchTimeout,
	)
	return resp[0].Output, err
}

func (c Client) getTrexServiceJournalctl() (string, error) {
	command := fmt.Sprintf("journalctl | grep %s", SystemdUnitFileName)
	resp, err := c.consoleExpecter.SafeExpectBatchWithResponse([]expect.Batcher{
		&expect.BSnd{S: command + "\n"},
		&expect.BExp{R: shellPrompt},
	},
		batchTimeout,
	)
	return resp[0].Output, err
}

func (c Client) getStartTrafficCmd(port PortIdx) string {
	sb := strings.Builder{}
	sb.WriteString("start ")
	sb.WriteString(fmt.Sprintf("-f %s ", path.Join(StreamsPyPath, StreamPyFileName)))
	sb.WriteString(fmt.Sprintf("-m %spps ", c.trafficGeneratorPacketsPerSecond))
	sb.WriteString(fmt.Sprintf("-p %d ", port))
	sb.WriteString(fmt.Sprintf("-d %.0f", c.testDuration.Seconds()))
	return sb.String()
}

func (c Client) runTrexConsoleCmd(command string) (string, error) {
	shellCommand := fmt.Sprintf("cd %s && echo %q | ./trex-console -q", BinDirectory, command)
	resp, err := c.consoleExpecter.SafeExpectBatchWithResponse([]expect.Batcher{
		&expect.BSnd{S: shellCommand + "\n"},
		&expect.BExp{R: shellPrompt},
	},
		batchTimeout,
	)

	if err != nil {
		return "", err
	}
	return cleanStdout(resp[0].Output), nil
}

func (c Client) runTrexConsoleCmdWithJSONResponse(command, requestKey string) (string, error) {
	const verboseOn = "verbose on;"
	trexConsoleCommand := verboseOn + command
	shellCommand := fmt.Sprintf("cd %s && echo %q | ./trex-console -q", BinDirectory, trexConsoleCommand)

	resp, err := c.consoleExpecter.SafeExpectBatchWithResponse([]expect.Batcher{
		&expect.BSnd{S: shellCommand + "\n"},
		&expect.BExp{R: shellPrompt},
	},
		batchTimeout,
	)

	if err != nil {
		return "", err
	}

	stdout := cleanStdout(resp[0].Output)
	jsonResponse, err := extractJSONString(stdout, requestKey)
	if err != nil {
		log.Printf("failed to extract JSON Response of %q in input: \n%q", requestKey, stdout)
		return "", fmt.Errorf("failed to extract JSON Response of %q: %w. See logs for more information", requestKey, err)
	}
	return jsonResponse, nil
}

func cleanStdout(rawStdout string) string {
	stdout := strings.Replace(rawStdout, "Using 'python3' as Python interpeter", "", -1)
	stdout = strings.Replace(stdout, "-=TRex Console v3.0=-", "", -1)
	stdout = strings.Replace(stdout, "Type 'help' or '?' for supported actions", "", -1)
	stdout = strings.Replace(stdout, "trex>Global Statistitcs", "", -1)
	stdout = strings.Replace(stdout, "trex>", "", -1)
	return removeUnprintableCharacters(stdout)
}

func removeUnprintableCharacters(input string) string {
	ansiEscape := regexp.MustCompile(`\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])|\r`) //nolint: gocritic
	cleanedInput := ansiEscape.ReplaceAllString(input, "")
	return cleanedInput
}

func extractJSONString(input, requestKey string) (string, error) {
	const (
		responseStart = "[verbose] Server Response:\n\n"
		responseEnd   = "\n\n"
	)

	requestIndex := strings.Index(input, requestKey) + len(requestKey)
	if requestIndex == -1 {
		return "", fmt.Errorf("could not find start of request Key JSON string: %q", requestKey)
	}
	requestIndex += len(requestKey)

	responseStartIndex := strings.Index(input[requestIndex:], responseStart)
	if responseStartIndex == -1 {
		return "", fmt.Errorf("could not find start of JSON string %q", responseStart)
	}
	responseStartIndex += len(responseStart) + requestIndex

	responseEndIndex := strings.Index(input[responseStartIndex:], responseEnd)
	if responseEndIndex == -1 {
		return "", fmt.Errorf("could not find end of JSON string: %q", responseEnd)
	}
	responseEndIndex += len(responseEnd) + responseStartIndex

	return strings.TrimSpace(input[responseStartIndex:responseEndIndex]), nil
}
