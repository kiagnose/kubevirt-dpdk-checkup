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

package executor

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	expect "github.com/google/goexpect"

	"kubevirt.io/client-go/kubecli"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/console"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

type vmiSerialConsoleClient interface {
	VMISerialConsole(namespace, name string, timeout time.Duration) (kubecli.StreamInterface, error)
}

type podExecuteClient interface {
	ExecuteCommandOnPod(ctx context.Context, namespace, name, containerName string, command []string) (stdout, stderr string, err error)
}

type Executor struct {
	client                                     vmiSerialConsoleClient
	podClient                                  podExecuteClient
	namespace                                  string
	vmiUsername                                string
	vmiPassword                                string
	vmiEastNICPCIAddress                       string
	vmiEastEthPeerMACAddress                   string
	vmiWestNICPCIAddress                       string
	vmiWestEthPeerMACAddress                   string
	testDuration                               time.Duration
	verbosePrintsEnabled                       bool
	trafficGeneratorPacketsPerSecondInMillions int
}

const testpmdPrompt = "testpmd> "

func New(client vmiSerialConsoleClient, podClient podExecuteClient, namespace string, cfg config.Config) Executor {
	return Executor{
		client:                   client,
		podClient:                podClient,
		namespace:                namespace,
		vmiUsername:              config.VMIUsername,
		vmiPassword:              config.VMIPassword,
		vmiEastNICPCIAddress:     config.VMIEastNICPCIAddress,
		vmiEastEthPeerMACAddress: cfg.TrafficGeneratorEastMacAddress.String(),
		vmiWestNICPCIAddress:     config.VMIWestNICPCIAddress,
		vmiWestEthPeerMACAddress: cfg.TrafficGeneratorWestMacAddress.String(),
		testDuration:             cfg.TestDuration,
		verbosePrintsEnabled:     cfg.Verbose,
		trafficGeneratorPacketsPerSecondInMillions: cfg.TrafficGeneratorPacketsPerSecondInMillions,
	}
}

func (e Executor) Execute(ctx context.Context, vmiName, podName, podContainerName string) (status.Results, error) {
	if err := console.LoginToCentOS(e.client, e.namespace, vmiName, e.vmiUsername, e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, vmiName, err)
	}

	trexClient := NewTrexConsole(e.podClient, e.namespace, podName, podContainerName, e.verbosePrintsEnabled)

	log.Printf("Starting testpmd in VMI...")
	if err := e.runTestpmd(vmiName); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing testpmd stats in VMI...")
	if err := e.clearStatsTestpmd(vmiName); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing Trex console stats before test...")
	_, err := trexClient.ClearStats(ctx)
	if err != nil {
		return status.Results{}, fmt.Errorf("failed to clear trex stats on pod \"%s/%s\" side: %w", e.namespace, podName, err)
	}

	const (
		trafficSourcePort = 0
		trafficDestPort   = 1
	)

	log.Printf("Running traffic for %s...", e.testDuration.String())
	_, err = trexClient.StartTraffic(ctx, e.trafficGeneratorPacketsPerSecondInMillions, trafficSourcePort, e.testDuration)
	if err != nil {
		return status.Results{}, fmt.Errorf("failed to run traffic from trex-console on pod \"%s/%s\" side: %w",
			e.namespace, podName, err)
	}

	results := status.Results{}
	results.TrafficGeneratorMaxDropRate, err = trexClient.MonitorDropRates(ctx, e.testDuration)
	log.Printf("traffic Generator Max Drop Rate: %fBps", results.TrafficGeneratorMaxDropRate)
	if err != nil {
		return status.Results{}, err
	}

	var trafficGeneratorSrcPortStats portStats
	trafficGeneratorSrcPortStats, err = trexClient.GetPortStats(ctx, trafficSourcePort)
	if err != nil {
		return status.Results{}, err
	}

	_, err = trexClient.GetPortStats(ctx, trafficDestPort)
	if err != nil {
		return status.Results{}, err
	}

	results.TrafficGeneratorOutErrorPackets = trafficGeneratorSrcPortStats.Result.Oerrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trafficSourcePort, results.TrafficGeneratorOutErrorPackets)

	log.Printf("get testpmd stats in DPDK VMI...")
	var testPmdStats map[string]int64
	if testPmdStats, err = e.getStatsTestpmd(vmiName); err != nil {
		return status.Results{}, err
	}
	const (
		TXDropped = "TX-dropped"
		RXDropped = "RX-dropped"
	)
	results.DPDKPacketsRxDropped = testPmdStats[RXDropped]
	results.DPDKPacketsTxDropped = testPmdStats[TXDropped]
	log.Printf("DPDK side packets Dropped: Rx: %d; TX: %d", results.DPDKPacketsRxDropped, results.DPDKPacketsTxDropped)

	return results, nil
}

func (e Executor) runTestpmd(vmiName string) error {
	const batchTimeout = 30 * time.Second

	testpmdCmd := buildTestpmdCmd(e.vmiEastNICPCIAddress, e.vmiWestNICPCIAddress, e.vmiEastEthPeerMACAddress, e.vmiWestEthPeerMACAddress)

	resp, err := console.SafeExpectBatchWithResponse(e.client, e.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: testpmdCmd + "\n"},
			&expect.BExp{R: testpmdPrompt},
			&expect.BSnd{S: "start" + "\n"},
			&expect.BExp{R: testpmdPrompt},
		},
		batchTimeout,
	)

	if err != nil {
		return err
	}

	log.Printf("%v", resp)

	return nil
}

func (e Executor) clearStatsTestpmd(vmiName string) error {
	const batchTimeout = 30 * time.Second

	const testpmdCmd = "clear fwd stats all"

	_, err := console.SafeExpectBatchWithResponse(e.client, e.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: testpmdCmd + "\n"},
			&expect.BExp{R: testpmdPrompt},
		},
		batchTimeout,
	)

	if err != nil {
		return err
	}

	return nil
}

func (e Executor) getStatsTestpmd(vmiName string) (map[string]int64, error) {
	const batchTimeout = 30 * time.Second

	const testpmdPromt = "testpmd> "

	testpmdCmd := "show fwd stats all"

	resp, err := console.SafeExpectBatchWithResponse(e.client, e.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: testpmdCmd + "\n"},
			&expect.BExp{R: testpmdPromt},
		},
		batchTimeout,
	)

	if err != nil {
		return nil, err
	}

	if e.verbosePrintsEnabled {
		log.Printf("testpmd stats: %v", resp)
	}

	StatisticsSummaryString, err := extractSummaryStatistics(resp[0].Output)
	if err != nil {
		return nil, err
	}

	return parseTestpmdStats(StatisticsSummaryString)
}

func extractSummaryStatistics(input string) (string, error) {
	const summaryStart = "+++++++++++++++ Accumulated forward statistics for all ports+++++++++++++++"
	const summaryEnd = "++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++"

	startIndex := strings.Index(input, summaryStart) + len(summaryStart)
	if startIndex == -1 {
		return "", fmt.Errorf("could not find start of JSON string")
	}

	endIndex := strings.Index(input[startIndex:], summaryEnd) + startIndex
	if endIndex == -1 {
		return "", fmt.Errorf("could not find end of JSON string")
	}

	return input[startIndex:endIndex], nil
}

func parseTestpmdStats(input string) (map[string]int64, error) {
	params := make(map[string]int64)
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		keyValuePairs := strings.Split(line, "  ")
		for _, pair := range keyValuePairs {
			if pair == "" {
				continue
			}
			fieldsWithoutDuplicateSpaces := strings.Fields(pair)
			formattedKeyValuePairString := strings.Join(fieldsWithoutDuplicateSpaces, " ")
			parts := strings.Split(formattedKeyValuePairString, ": ")

			key := strings.TrimSpace(parts[0])
			valueString := strings.TrimSpace(parts[1])

			value, err := strconv.ParseInt(valueString, 10, 64)
			if err != nil {
				return nil, err
			}
			params[key] = value
		}
	}
	return params, nil
}

func buildTestpmdCmd(vmiEastNICPCIAddress, vmiWestNICPCIAddress, eastEthPeerMACAddress, westEthPeerMACAddress string) string {
	const (
		cpuList       = "2-7"
		numberOfCores = 5
	)

	sb := strings.Builder{}
	sb.WriteString("dpdk-testpmd ")
	sb.WriteString(fmt.Sprintf("-l %s ", cpuList))
	sb.WriteString(fmt.Sprintf("-a %s ", vmiEastNICPCIAddress))
	sb.WriteString(fmt.Sprintf("-a %s ", vmiWestNICPCIAddress))
	sb.WriteString("-- ")
	sb.WriteString("-i ")
	sb.WriteString(fmt.Sprintf("--nb-cores=%d ", numberOfCores))
	sb.WriteString("--rxd=2048 ")
	sb.WriteString("--txd=2048 ")
	sb.WriteString("--forward-mode=mac ")
	sb.WriteString(fmt.Sprintf("--eth-peer=0,%s ", eastEthPeerMACAddress))
	sb.WriteString(fmt.Sprintf("--eth-peer=1,%s", westEthPeerMACAddress))

	return sb.String()
}
