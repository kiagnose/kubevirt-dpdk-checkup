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
	"regexp"
	"strings"
	"time"

	expect "github.com/google/goexpect"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/executor/console"
)

type Client struct {
	vmiSerialClient                  vmiSerialConsoleClient
	namespace                        string
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
	StreamsPyPath = "/opt/tests"
)

func NewClient(vmiSerialClient vmiSerialConsoleClient,
	namespace, trafficGeneratorPacketsPerSecond string,
	testDuration time.Duration,
	verbosePrintsEnabled bool) Client {
	return Client{
		vmiSerialClient:                  vmiSerialClient,
		namespace:                        namespace,
		trafficGeneratorPacketsPerSecond: trafficGeneratorPacketsPerSecond,
		testDuration:                     testDuration,
		verbosePrintsEnabled:             verbosePrintsEnabled,
	}
}

func (t Client) WaitForServerToBeReady(ctx context.Context, vmiName string) error {
	const (
		interval = 5 * time.Second
		timeout  = time.Minute
	)
	var err error
	ctxWithNewDeadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conditionFn := func(ctx context.Context) (bool, error) {
		if t.isServerRunning(vmiName) {
			log.Printf("trex-server is now ready")
			return true, nil
		}
		if t.verbosePrintsEnabled {
			log.Printf("trex-server is not yet ready...")
		}
		return false, nil
	}
	if err = wait.PollImmediateUntilWithContext(ctxWithNewDeadline, interval, conditionFn); err != nil {
		if !errors.Is(err, wait.ErrWaitTimeout) {
			return err
		}
		if t.verbosePrintsEnabled {
			if logErr := t.printTrexServiceFailLogs(vmiName); logErr != nil {
				return logErr
			}
		}
		return fmt.Errorf("timeout waiting for trex-server to be ready")
	}
	return nil
}

func (t Client) isServerRunning(vmiName string) bool {
	const helpSubstring = "Console Commands"
	resp, err := t.runTrexConsoleCmd(vmiName, "help")
	if err != nil || !strings.Contains(resp, helpSubstring) {
		return false
	}
	return true
}

func (t Client) printTrexServiceFailLogs(vmiName string) error {
	var err error
	trexServiceStatus, err := t.getTrexServiceStatus(vmiName)
	if err != nil {
		return fmt.Errorf("failed gathering systemctl service status after trex-server timeout: %w", err)
	}
	trexJournalctlLogs, err := t.getTrexServiceJournalctl(vmiName)
	if err != nil {
		return fmt.Errorf("failed gathering trex.service related joutnalctl logs after trex-server timeout: %w", err)
	}
	log.Printf("timeout waiting for trex-server to be ready\n"+
		"systemd service status:\n%s\n"+
		"joutnalctl logs:\n%s", trexServiceStatus, trexJournalctlLogs)
	return nil
}

func (t Client) getTrexServiceStatus(vmiName string) (string, error) {
	command := fmt.Sprintf("systemctl status %s | cat", SystemdUnitFileName)
	resp, err := console.SafeExpectBatchWithResponse(t.vmiSerialClient, t.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: command + "\n"},
			&expect.BExp{R: shellPrompt},
		},
		batchTimeout,
	)
	return resp[0].Output, err
}

func (t Client) getTrexServiceJournalctl(vmiName string) (string, error) {
	command := fmt.Sprintf("journalctl | grep %s", SystemdUnitFileName)
	resp, err := console.SafeExpectBatchWithResponse(t.vmiSerialClient, t.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: command + "\n"},
			&expect.BExp{R: shellPrompt},
		},
		batchTimeout,
	)
	return resp[0].Output, err
}

func (t Client) ClearStats(vmiName string) (string, error) {
	return t.runTrexConsoleCmd(vmiName, "clear")
}

func (t Client) StartTraffic(vmiName string, port PortIdx) (string, error) {
	startTrafficCmd := t.getStartTrafficCmd(port)
	return t.runTrexConsoleCmd(vmiName, startTrafficCmd)
}

func (t Client) getStartTrafficCmd(port PortIdx) string {
	sb := strings.Builder{}
	sb.WriteString("start ")
	sb.WriteString(fmt.Sprintf("-f %s/testpmd.py ", StreamsPyPath))
	sb.WriteString(fmt.Sprintf("-m %spps ", t.trafficGeneratorPacketsPerSecond))
	sb.WriteString(fmt.Sprintf("-p %d ", port))
	sb.WriteString(fmt.Sprintf("-d %.0f", t.testDuration.Seconds()))
	return sb.String()
}

func (t Client) GetGlobalStats(vmiName string) (GlobalStats, error) {
	const (
		globalStatsCommand    = "stats -g"
		globalStatsRequestKey = "get_global_stats"
	)
	globalStatsJSONString, err := t.runTrexConsoleCmdWithJSONResponse(vmiName, globalStatsCommand, globalStatsRequestKey)
	if err != nil {
		return GlobalStats{}, fmt.Errorf("failed to get global stats json: %w", err)
	}

	if t.verbosePrintsEnabled {
		log.Printf("GetGlobalStats JSON: %s", globalStatsJSONString)
	}

	var gs GlobalStats
	err = json.Unmarshal([]byte(globalStatsJSONString), &gs)
	if err != nil {
		return GlobalStats{}, fmt.Errorf("failed to unmarshal global stats json: %w", err)
	}
	return gs, nil
}

func (t Client) runTrexConsoleCmd(vmiName, command string) (string, error) {
	shellCommand := fmt.Sprintf("cd %s && echo %q | ./trex-console -q", BinDirectory, command)
	resp, err := console.SafeExpectBatchWithResponse(t.vmiSerialClient, t.namespace, vmiName,
		[]expect.Batcher{
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

func (t Client) runTrexConsoleCmdWithJSONResponse(vmiName, command, requestKey string) (string, error) {
	const verboseOn = "verbose on;"
	trexConsoleCommand := verboseOn + command
	shellCommand := fmt.Sprintf("cd %s && echo %q | ./trex-console -q", BinDirectory, trexConsoleCommand)

	resp, err := console.SafeExpectBatchWithResponse(t.vmiSerialClient, t.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: shellCommand + "\n"},
			&expect.BExp{R: shellPrompt},
		},
		batchTimeout,
	)

	if err != nil {
		return "", err
	}

	return extractJSONString(cleanStdout(resp[0].Output), requestKey)
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
