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
	"time"

	"kubevirt.io/client-go/kubecli"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/executor/console"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/trex"
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
	vmiSerialClient                  vmiSerialConsoleClient
	podClient                        podExecuteClient
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

func New(client vmiSerialConsoleClient, podClient podExecuteClient, namespace string, cfg config.Config) Executor {
	return Executor{
		vmiSerialClient:                  client,
		podClient:                        podClient,
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

func (e Executor) Execute(ctx context.Context, vmiName, podName, podContainerName string) (status.Results, error) {
	if err := console.LoginToCentOS(e.vmiSerialClient, e.namespace, vmiName, e.vmiUsername, e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, vmiName, err)
	}

	trexClient := trex.NewClient(e.podClient, e.namespace, podName, podContainerName, e.verbosePrintsEnabled)

	testpmdConsole := NewTestpmdConsole(e.vmiSerialClient, e.namespace, e.vmiEastNICPCIAddress, e.vmiEastEthPeerMACAddress,
		e.vmiWestNICPCIAddress, e.vmiWestEthPeerMACAddress, e.verbosePrintsEnabled)

	log.Printf("Starting testpmd in VMI...")
	if err := testpmdConsole.Run(vmiName); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing testpmd stats in VMI...")
	if err := testpmdConsole.ClearStats(vmiName); err != nil {
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
	_, err = trexClient.StartTraffic(ctx, e.trafficGeneratorPacketsPerSecond, trafficSourcePort, e.testDuration)
	if err != nil {
		return status.Results{}, fmt.Errorf("failed to run traffic from trex-console on pod \"%s/%s\" side: %w",
			e.namespace, podName, err)
	}

	results := status.Results{}
	trafficGeneratorMaxDropRate, err := trexClient.MonitorDropRates(ctx, e.testDuration)
	log.Printf("traffic Generator Max Drop Rate: %fBps", trafficGeneratorMaxDropRate)
	if err != nil {
		return status.Results{}, err
	}

	var trafficGeneratorSrcPortStats trex.PortStats
	trafficGeneratorSrcPortStats, err = trexClient.GetPortStats(ctx, trafficSourcePort)
	if err != nil {
		return status.Results{}, err
	}

	var trafficGeneratorDstPortStats trex.PortStats
	trafficGeneratorDstPortStats, err = trexClient.GetPortStats(ctx, trafficDestPort)
	if err != nil {
		return status.Results{}, err
	}

	results.TrafficGeneratorOutErrorPackets = trafficGeneratorSrcPortStats.Result.Oerrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trafficSourcePort, results.TrafficGeneratorOutErrorPackets)
	results.TrafficGeneratorInErrorPackets = trafficGeneratorDstPortStats.Result.Ierrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trafficDestPort, results.TrafficGeneratorInErrorPackets)
	results.TrafficGeneratorTxPackets = trafficGeneratorSrcPortStats.Result.Opackets
	log.Printf("traffic Generator packet sent via port %d: %d", trafficSourcePort, results.TrafficGeneratorTxPackets)

	log.Printf("get testpmd stats in DPDK VMI...")
	var testPmdStats [StatsArraySize]PortStats
	if testPmdStats, err = testpmdConsole.GetStats(vmiName); err != nil {
		return status.Results{}, err
	}
	results.DPDKPacketsRxDropped = testPmdStats[StatsSummary].RXDropped
	results.DPDKPacketsTxDropped = testPmdStats[StatsSummary].TXDropped
	log.Printf("DPDK side packets Dropped: Rx: %d; TX: %d", results.DPDKPacketsRxDropped, results.DPDKPacketsTxDropped)
	results.DPDKRxTestPackets =
		testPmdStats[StatsSummary].RXTotal - testPmdStats[StatsPort0].TXPackets - testPmdStats[StatsPort1].RXPackets
	log.Printf("DPDK side test packets received (including dropped, excluding non-related packets): %d", results.DPDKRxTestPackets)

	return results, nil
}
