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
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
)

type PodExecuteClient interface {
	ExecuteCommandOnPod(ctx context.Context, namespace, name, containerName string, command []string) (stdout, stderr string, err error)
}

type trexClient struct {
	podClient            PodExecuteClient
	namespace            string
	name                 string
	containerName        string
	verbosePrintsEnabled bool
}

func NewClient(client PodExecuteClient, namespace, name, containerName string, verbosePrintsEnabled bool) trexClient {
	return trexClient{
		podClient:            client,
		namespace:            namespace,
		name:                 name,
		containerName:        containerName,
		verbosePrintsEnabled: verbosePrintsEnabled,
	}
}

func (t trexClient) GetPortStats(ctx context.Context, port int) (PortStats, error) {
	portStatsJSONString, err := t.runCommandWithJSONResponse(ctx, fmt.Sprintf("stats --port %d -p", port))
	if err != nil {
		return PortStats{}, fmt.Errorf("failed to get global stats json: %w", err)
	}

	if t.verbosePrintsEnabled {
		log.Printf("GetPortStats JSON: %s", portStatsJSONString)
	}

	var ps PortStats
	err = json.Unmarshal([]byte(portStatsJSONString), &ps)
	if err != nil {
		return PortStats{}, fmt.Errorf("failed to unmarshal port %d stats json: %w", port, err)
	}
	return ps, nil
}

func (t trexClient) GetGlobalStats(ctx context.Context) (GlobalStats, error) {
	globalStatsJSONString, err := t.runCommandWithJSONResponse(ctx, "stats -g")
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

func (t trexClient) MonitorDropRates(ctx context.Context, duration time.Duration) (float64, error) {
	const interval = 10 * time.Second
	log.Printf("Monitoring traffic generator side drop rates every %ss during the test duration...", interval)
	maxDropRateBps := float64(0)

	ctxWithNewDeadline, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	conditionFn := func(ctx context.Context) (bool, error) {
		statsGlobal, err := t.GetGlobalStats(ctx)
		if statsGlobal.Result.MRxDropBps > maxDropRateBps {
			maxDropRateBps = statsGlobal.Result.MRxDropBps
		}
		return false, err
	}
	if err := wait.PollImmediateUntilWithContext(ctxWithNewDeadline, interval, conditionFn); err != nil {
		if errors.Is(err, wait.ErrWaitTimeout) {
			log.Printf("finished polling for drop rates")
		} else {
			return 0, fmt.Errorf("failed to poll global stats in trex-console: %w", err)
		}
	}

	return maxDropRateBps, nil
}

func (t trexClient) ClearStats(ctx context.Context) (string, error) {
	return t.runCommand(ctx, "clear")
}

func (t trexClient) StartTraffic(ctx context.Context, packetPerSecond string, port int, testDuration time.Duration) (string, error) {
	testDurationSeconds := int(testDuration.Seconds())
	return t.runCommand(ctx, fmt.Sprintf("start -f /opt/tests/testpmd.py -m %spps -p %d -d %d",
		packetPerSecond, port, testDurationSeconds))
}

func (t trexClient) StopTraffic(ctx context.Context) (string, error) {
	return t.runCommand(ctx, "stop -a")
}

func (t trexClient) runCommand(ctx context.Context, command string) (string, error) {
	var (
		err            error
		stdout, stderr string
	)

	if stdout, stderr, err = t.podClient.ExecuteCommandOnPod(ctx, t.namespace, t.name, t.containerName,
		[]string{
			"/bin/sh",
			"-c",
			fmt.Sprintf("echo %q | ./trex-console -q", command),
		}); err != nil {
		return "", fmt.Errorf("failed to get pod stats \"%s/%s\": err %w, stderr: %s", t.namespace, t.name, err, stderr)
	}

	return cleanStdout(stdout), nil
}

func (t trexClient) runCommandWithJSONResponse(ctx context.Context, command string) (string, error) {
	var (
		err            error
		stdout, stderr string
	)

	const verboseOn = "verbose on;"
	command = verboseOn + command
	if stdout, stderr, err = t.podClient.ExecuteCommandOnPod(ctx, t.namespace, t.name, t.containerName,
		[]string{
			"/bin/sh",
			"-c",
			fmt.Sprintf("echo %q | ./trex-console -q", command),
		}); err != nil {
		return "", fmt.Errorf("failed to get pod stats \"%s/%s\": err %w, stderr: %s", t.namespace, t.name, err, stderr)
	}

	return extractJSONString(stdout)
}

func cleanStdout(rawStdout string) string {
	stdout := strings.Replace(rawStdout, "Using 'python3' as Python interpeter", "", -1)
	stdout = strings.Replace(stdout, "-=TRex Console v3.0=-", "", -1)
	stdout = strings.Replace(stdout, "Type 'help' or '?' for supported actions", "", -1)
	stdout = strings.Replace(stdout, "trex>Global Statistitcs", "", -1)
	stdout = strings.Replace(stdout, "trex>", "", -1)

	return stdout
}

func extractJSONString(input string) (string, error) {
	const responseStart = "Server Response:\n\n"
	const responseEnd = "\n\n"

	startIndex := strings.Index(input, responseStart) + len(responseStart)
	if startIndex == -1 {
		return "", fmt.Errorf("could not find start of JSON string")
	}

	endIndex := strings.Index(input[startIndex:], responseEnd) + startIndex
	if endIndex == -1 {
		return "", fmt.Errorf("could not find end of JSON string")
	}

	return input[startIndex:endIndex], nil
}