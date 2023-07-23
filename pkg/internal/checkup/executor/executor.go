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
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/executor/testpmd"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/trex"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

type vmiSerialConsoleClient interface {
	VMISerialConsole(namespace, name string, timeout time.Duration) (kubecli.StreamInterface, error)
}

type Executor struct {
	vmiSerialClient                  vmiSerialConsoleClient
	namespace                        string
	vmiUsername                      string
	vmiPassword                      string
	vmiUnderTestEastNICPCIAddress    string
	trafficGenEastMACAddress         string
	vmiUnderTestWestNICPCIAddress    string
	trafficGenWestMACAddress         string
	testDuration                     time.Duration
	verbosePrintsEnabled             bool
	trafficGeneratorPacketsPerSecond string
}

func New(client vmiSerialConsoleClient, namespace string, cfg config.Config) Executor {
	return Executor{
		vmiSerialClient:                  client,
		namespace:                        namespace,
		vmiUsername:                      config.VMIUsername,
		vmiPassword:                      config.VMIPassword,
		vmiUnderTestEastNICPCIAddress:    config.VMIEastNICPCIAddress,
		trafficGenEastMACAddress:         cfg.TrafficGeneratorEastMacAddress.String(),
		vmiUnderTestWestNICPCIAddress:    config.VMIWestNICPCIAddress,
		trafficGenWestMACAddress:         cfg.TrafficGeneratorWestMacAddress.String(),
		testDuration:                     cfg.TestDuration,
		verbosePrintsEnabled:             cfg.Verbose,
		trafficGeneratorPacketsPerSecond: cfg.TrafficGeneratorPacketsPerSecond,
	}
}

func (e Executor) Execute(ctx context.Context, vmiUnderTestName, trafficGenVMIName string) (status.Results, error) {
	log.Printf("Login to VMI under test...")
	if err := console.LoginToCentOS(e.vmiSerialClient, e.namespace, vmiUnderTestName, e.vmiUsername, e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, vmiUnderTestName, err)
	}

	log.Printf("Login to traffic generator...")
	if err := console.LoginToCentOS(e.vmiSerialClient, e.namespace, trafficGenVMIName, e.vmiUsername, e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, trafficGenVMIName, err)
	}

	log.Printf("Starting traffic generator Server Service...")
	if err := trex.StartTrexService(e.vmiSerialClient, e.namespace, trafficGenVMIName); err != nil {
		return status.Results{}, fmt.Errorf("failed to Start to Trex Service on VMI \"%s/%s\": %w", e.namespace, trafficGenVMIName, err)
	}

	trexClient := trex.NewClient(e.vmiSerialClient, e.namespace, e.trafficGeneratorPacketsPerSecond, e.testDuration, e.verbosePrintsEnabled)

	log.Printf("Waiting until traffic generator Server Service is ready...")
	if err := trexClient.WaitForServerToBeReady(ctx, trafficGenVMIName); err != nil {
		return status.Results{}, fmt.Errorf("failed to Start to Trex Service on VMI \"%s/%s\": %w", e.namespace, trafficGenVMIName, err)
	}

	testpmdConsole := testpmd.NewTestpmdConsole(e.vmiSerialClient, e.namespace, e.vmiUnderTestEastNICPCIAddress, e.trafficGenEastMACAddress,
		e.vmiUnderTestWestNICPCIAddress, e.trafficGenWestMACAddress, e.verbosePrintsEnabled)

	log.Printf("Starting testpmd in VMI...")
	if err := testpmdConsole.Run(vmiUnderTestName); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing testpmd stats in VMI...")
	if err := testpmdConsole.ClearStats(vmiUnderTestName); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing Trex console stats before test...")
	if _, err := trexClient.ClearStats(trafficGenVMIName); err != nil {
		return status.Results{}, fmt.Errorf("failed to clear trex stats on traffic generator VMI \"%s/%s\" side: %w",
			e.namespace, trafficGenVMIName, err)
	}

	results := status.Results{}
	var trafficGeneratorSrcPortStats trex.PortStats
	var trafficGeneratorDstPortStats trex.PortStats

	results.TrafficGeneratorOutErrorPackets = trafficGeneratorSrcPortStats.Result.Oerrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trex.SourcePort, results.TrafficGeneratorOutErrorPackets)
	results.TrafficGeneratorInErrorPackets = trafficGeneratorDstPortStats.Result.Ierrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trex.DestPort, results.TrafficGeneratorInErrorPackets)
	results.TrafficGeneratorTxPackets = trafficGeneratorSrcPortStats.Result.Opackets
	log.Printf("traffic Generator packet sent via port %d: %d", trex.SourcePort, results.TrafficGeneratorTxPackets)

	log.Printf("get testpmd stats in DPDK VMI...")
	var testPmdStats [testpmd.StatsArraySize]testpmd.PortStats
	var err error
	if testPmdStats, err = testpmdConsole.GetStats(vmiUnderTestName); err != nil {
		return status.Results{}, err
	}
	results.DPDKPacketsRxDropped = testPmdStats[testpmd.StatsSummary].RXDropped
	results.DPDKPacketsTxDropped = testPmdStats[testpmd.StatsSummary].TXDropped
	log.Printf("DPDK side packets Dropped: Rx: %d; TX: %d", results.DPDKPacketsRxDropped, results.DPDKPacketsTxDropped)
	results.DPDKRxTestPackets =
		testPmdStats[testpmd.StatsSummary].RXTotal - testPmdStats[testpmd.StatsPort0].TXPackets - testPmdStats[testpmd.StatsPort1].RXPackets
	log.Printf("DPDK side test packets received (including dropped, excluding non-related packets): %d", results.DPDKRxTestPackets)

	return results, nil
}
