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
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	expect "github.com/google/goexpect"

	"kubevirt.io/client-go/kubecli"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/console"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/trex"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

type vmiSerialConsoleClient interface {
	VMISerialConsole(namespace, name string, timeout time.Duration) (kubecli.StreamInterface, error)
}

type testPmdPortStats struct {
	RXPackets int64
	RXDropped int64
	RXTotal   int64
	TXPackets int64
	TXDropped int64
	TXTotal   int64
}

type testPmdStatsIdx int

const (
	testPmdStatsPort0 testPmdStatsIdx = iota
	testPmdStatsPort1
	testPmdPortStatsSummary
	testPmdPortStatsSize
)

type Executor struct {
	client                           vmiSerialConsoleClient
	namespace                        string
	vmiUsername                      string
	vmiPassword                      string
	vmiEastNICPCIAddress             string
	vmiEastEthPeerMACAddress         string
	vmiWestNICPCIAddress             string
	vmiWestEthPeerMACAddress         string
	testDuration                     time.Duration
	verbosePrintsEnabled             bool
	trafficGeneratorPacketsPerSecond string
}

const testpmdPrompt = "testpmd> "

func New(client vmiSerialConsoleClient, namespace string, cfg config.Config) Executor {
	return Executor{
		client:                           client,
		namespace:                        namespace,
		vmiUsername:                      config.VMIUsername,
		vmiPassword:                      config.VMIPassword,
		vmiEastNICPCIAddress:             config.VMIEastNICPCIAddress,
		vmiEastEthPeerMACAddress:         cfg.TrafficGeneratorEastMacAddress.String(),
		vmiWestNICPCIAddress:             config.VMIWestNICPCIAddress,
		vmiWestEthPeerMACAddress:         cfg.TrafficGeneratorWestMacAddress.String(),
		testDuration:                     cfg.TestDuration,
		verbosePrintsEnabled:             cfg.Verbose,
		trafficGeneratorPacketsPerSecond: cfg.TrafficGeneratorPacketsPerSecond,
	}
}

func (e Executor) Execute(ctx context.Context, vmiName string) (status.Results, error) {
	if err := console.LoginToCentOS(e.client, e.namespace, vmiName, e.vmiUsername, e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, vmiName, err)
	}

	const (
		trafficSourcePort = 0
		trafficDestPort   = 1
	)

	log.Printf("Starting testpmd in VMI...")
	if err := e.runTestpmd(vmiName); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing testpmd stats in VMI...")
	if err := e.clearStatsTestpmd(vmiName); err != nil {
		return status.Results{}, err
	}

	results := status.Results{}
	var trafficGeneratorSrcPortStats trex.PortStats
	var trafficGeneratorDstPortStats trex.PortStats

	results.TrafficGeneratorOutErrorPackets = trafficGeneratorSrcPortStats.Result.Oerrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trafficSourcePort, results.TrafficGeneratorOutErrorPackets)
	results.TrafficGeneratorInErrorPackets = trafficGeneratorDstPortStats.Result.Ierrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trafficDestPort, results.TrafficGeneratorInErrorPackets)
	results.TrafficGeneratorTxPackets = trafficGeneratorSrcPortStats.Result.Opackets
	log.Printf("traffic Generator packet sent via port %d: %d", trafficSourcePort, results.TrafficGeneratorTxPackets)

	log.Printf("get testpmd stats in DPDK VMI...")
	var testPmdStats [testPmdPortStatsSize]testPmdPortStats
	var err error
	if testPmdStats, err = e.getStatsTestpmd(vmiName); err != nil {
		return status.Results{}, err
	}
	results.DPDKPacketsRxDropped = testPmdStats[testPmdPortStatsSummary].RXDropped
	results.DPDKPacketsTxDropped = testPmdStats[testPmdPortStatsSummary].TXDropped
	log.Printf("DPDK side packets Dropped: Rx: %d; TX: %d", results.DPDKPacketsRxDropped, results.DPDKPacketsTxDropped)
	results.DPDKRxTestPackets =
		testPmdStats[testPmdPortStatsSummary].RXTotal - testPmdStats[testPmdStatsPort0].TXPackets - testPmdStats[testPmdStatsPort1].RXPackets
	log.Printf("DPDK side test packets received (including dropped, excluding non-related packets): %d", results.DPDKRxTestPackets)

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

func (e Executor) getStatsTestpmd(vmiName string) ([testPmdPortStatsSize]testPmdPortStats, error) {
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
		return [testPmdPortStatsSize]testPmdPortStats{}, err
	}

	if e.verbosePrintsEnabled {
		log.Printf("testpmd stats: %v", resp)
	}

	return parseTestpmdStats(resp[0].Output)
}

func extractSectionStatistics(input, sectionStart, sectionEnd string) (string, error) {
	lines := strings.Split(input, "\n")
	var startLineIdx, endLineIdx int

	startLineIdx = findStringLineIndex(lines, sectionStart)
	endLineIdx = startLineIdx + findStringLineIndex(lines[startLineIdx:], sectionEnd)

	if l := len(lines); startLineIdx >= l || endLineIdx >= l {
		return "", fmt.Errorf("could not extract statistics section. found start: %v; found end: %v", startLineIdx < l, endLineIdx < l)
	}

	return strings.Join(lines[startLineIdx+1:endLineIdx], "\n"), nil
}

func findStringLineIndex(lines []string, substring string) int {
	for lineIdx := range lines {
		if strings.Contains(lines[lineIdx], substring) {
			return lineIdx
		}
	}
	return len(lines)
}

func parseTestpmdStats(input string) ([testPmdPortStatsSize]testPmdPortStats, error) {
	var statistics [testPmdPortStatsSize]testPmdPortStats
	const (
		port0SectionStart   = "Forward statistics for port 0"
		port0SectionEnd     = "----------------------------------------------------------------------------"
		port1SectionStart   = "Forward statistics for port 1"
		port1SectionEnd     = port0SectionEnd
		SummarySectionStart = "Accumulated forward statistics for all ports"
		SummarySectionEnd   = "++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++"
	)

	startSections := [testPmdPortStatsSize]string{port0SectionStart, port1SectionStart, SummarySectionStart}
	endSections := [testPmdPortStatsSize]string{port0SectionEnd, port1SectionEnd, SummarySectionEnd}
	for statsIdx := range startSections {
		sectionString, err := extractSectionStatistics(input, startSections[statsIdx], endSections[statsIdx])
		if err != nil {
			return [testPmdPortStatsSize]testPmdPortStats{}, fmt.Errorf("failed parsing section on port %d: %w", statsIdx, err)
		}
		err = parseTestpmdStatsSection(&statistics[statsIdx], sectionString)
		if err != nil {
			return [testPmdPortStatsSize]testPmdPortStats{}, err
		}
	}

	return statistics, nil
}

func parseTestpmdStatsSection(stats *testPmdPortStats, section string) error {
	const (
		RXPacketsIndex = 1
		RXDroppedIndex = 3
		RXTotalIndex   = 5
		TXPacketsIndex = 1
		TXDroppedIndex = 3
		TXTotalIndex   = 5
	)
	lines := strings.Split(section, "\n")
	for i := range lines {
		if lines[i] == "" {
			continue
		} else if strings.Contains(lines[i], "RX-packets") {
			fields := strings.Fields(lines[i])
			stats.RXPackets, _ = strconv.ParseInt(fields[RXPacketsIndex], 10, 64)
			stats.RXDropped, _ = strconv.ParseInt(fields[RXDroppedIndex], 10, 64)
			stats.RXTotal, _ = strconv.ParseInt(fields[RXTotalIndex], 10, 64)
		} else if strings.Contains(lines[i], "TX-packets") {
			fields := strings.Fields(lines[i])
			stats.TXPackets, _ = strconv.ParseInt(fields[TXPacketsIndex], 10, 64)
			stats.TXDropped, _ = strconv.ParseInt(fields[TXDroppedIndex], 10, 64)
			stats.TXTotal, _ = strconv.ParseInt(fields[TXTotalIndex], 10, 64)
		} else {
			return fmt.Errorf("parse fail. Unknown line format %s", lines[i])
		}
	}
	return nil
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
