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
	"errors"
	"fmt"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
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
		vmiPassword:                      config.VMIPassword,
		vmiUnderTestEastNICPCIAddress:    config.VMIEastNICPCIAddress,
		trafficGenEastMACAddress:         cfg.TrafficGenEastMacAddress.String(),
		vmiUnderTestWestNICPCIAddress:    config.VMIWestNICPCIAddress,
		trafficGenWestMACAddress:         cfg.TrafficGenWestMacAddress.String(),
		testDuration:                     cfg.TestDuration,
		verbosePrintsEnabled:             cfg.Verbose,
		trafficGeneratorPacketsPerSecond: cfg.TrafficGenPacketsPerSecond,
	}
}

func (e Executor) Execute(ctx context.Context, vmiUnderTestName, trafficGenVMIName string) (status.Results, error) {
	log.Printf("Login to VMI under test...")
	vmiUnderTestConsoleExpecter := console.NewExpecter(e.vmiSerialClient, e.namespace, vmiUnderTestName)
	if err := vmiUnderTestConsoleExpecter.LoginToCentOSAsRoot(e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, vmiUnderTestName, err)
	}

	log.Printf("Login to traffic generator...")
	trafficGenConsoleExpecter := console.NewExpecter(e.vmiSerialClient, e.namespace, trafficGenVMIName)
	if err := trafficGenConsoleExpecter.LoginToCentOSAsRoot(e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, trafficGenVMIName, err)
	}

	if e.verbosePrintsEnabled {
		vmiUnderTestKernelArgs, _ := vmiUnderTestConsoleExpecter.GetGuestKernelArgs()
		log.Printf("VMI under test guest kernel Args: %s", vmiUnderTestKernelArgs)

		trafficGenKernelArgs, _ := trafficGenConsoleExpecter.GetGuestKernelArgs()
		log.Printf("traffic generator guest kernel Args: %s", trafficGenKernelArgs)
	}

	trexClient := trex.NewClient(
		trafficGenConsoleExpecter,
		e.trafficGeneratorPacketsPerSecond,
		e.testDuration,
		e.verbosePrintsEnabled,
	)

	log.Printf("Starting traffic generator Server Service...")
	if err := trexClient.StartServer(); err != nil {
		return status.Results{}, fmt.Errorf("failed to Start to Trex Service on VMI \"%s/%s\": %w", e.namespace, trafficGenVMIName, err)
	}

	log.Printf("Waiting until traffic generator Server Service is ready...")
	if err := trexClient.WaitForServerToBeReady(ctx); err != nil {
		return status.Results{}, fmt.Errorf("failed to Start to Trex Service on VMI \"%s/%s\": %w", e.namespace, trafficGenVMIName, err)
	}

	testpmdConsole := testpmd.NewTestpmdConsole(
		vmiUnderTestConsoleExpecter,
		e.vmiUnderTestEastNICPCIAddress,
		e.trafficGenEastMACAddress,
		e.vmiUnderTestWestNICPCIAddress,
		e.trafficGenWestMACAddress,
		e.verbosePrintsEnabled,
	)

	log.Printf("Starting testpmd in VMI...")
	if err := testpmdConsole.Run(); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing testpmd stats in VMI...")
	if err := testpmdConsole.ClearStats(); err != nil {
		return status.Results{}, err
	}

	log.Printf("Clearing Trex console stats before test...")
	if _, err := trexClient.ClearStats(); err != nil {
		return status.Results{}, fmt.Errorf("failed to clear trex stats on traffic generator VMI \"%s/%s\" side: %w",
			e.namespace, trafficGenVMIName, err)
	}

	log.Printf("Running traffic for %s...", e.testDuration.String())
	if _, err := trexClient.StartTraffic(trex.SourcePort); err != nil {
		return status.Results{}, fmt.Errorf("failed to run traffic from traffic generator VMI \"%s/%s\" side: %w",
			e.namespace, trafficGenVMIName, err)
	}

	var err error
	trafficGeneratorMaxDropRate, err := e.monitorDropRates(ctx, trexClient)
	if err != nil {
		return status.Results{}, err
	}
	log.Printf("traffic Generator Max Drop Rate: %fBps", trafficGeneratorMaxDropRate)

	return calculateStats(trexClient, testpmdConsole)
}

func calculateStats(trexClient trex.Client, testpmdConsole *testpmd.TestpmdConsole) (status.Results, error) {
	var err error
	results := status.Results{}
	var trafficGeneratorSrcPortStats trex.PortStats
	trafficGeneratorSrcPortStats, err = trexClient.GetPortStats(trex.SourcePort)
	if err != nil {
		return status.Results{}, err
	}

	var trafficGeneratorDstPortStats trex.PortStats
	trafficGeneratorDstPortStats, err = trexClient.GetPortStats(trex.DestPort)
	if err != nil {
		return status.Results{}, err
	}

	results.TrafficGenOutputErrorPackets = trafficGeneratorSrcPortStats.Result.Oerrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trex.SourcePort, results.TrafficGenOutputErrorPackets)
	results.TrafficGenInputErrorPackets = trafficGeneratorDstPortStats.Result.Ierrors
	log.Printf("traffic Generator port %d Packet output errors: %d", trex.DestPort, results.TrafficGenInputErrorPackets)
	results.TrafficGenSentPackets = trafficGeneratorSrcPortStats.Result.Opackets
	log.Printf("traffic Generator packet sent via port %d: %d", trex.SourcePort, results.TrafficGenSentPackets)

	log.Printf("get testpmd stats in VM-Under-Test...")
	var testPmdStats [testpmd.StatsArraySize]testpmd.PortStats
	if testPmdStats, err = testpmdConsole.GetStats(); err != nil {
		return status.Results{}, err
	}
	results.VMUnderTestRxDroppedPackets = testPmdStats[testpmd.StatsSummary].RXDropped
	results.VMUnderTestTxDroppedPackets = testPmdStats[testpmd.StatsSummary].TXDropped
	log.Printf("VMI-Under-Test's side packets Dropped: Rx: %d; TX: %d",
		results.VMUnderTestRxDroppedPackets, results.VMUnderTestTxDroppedPackets)
	results.VMUnderTestReceivedPackets =
		testPmdStats[testpmd.StatsSummary].RXTotal - testPmdStats[testpmd.StatsPort0].TXPackets - testPmdStats[testpmd.StatsPort1].RXPackets
	log.Printf("VMI-Under-Test's side test packets received (including dropped, excluding non-related packets): %d",
		results.VMUnderTestReceivedPackets)

	return results, nil
}

func (e Executor) monitorDropRates(ctx context.Context, trexClient trex.Client) (float64, error) {
	const interval = 10 * time.Second

	log.Printf("Monitoring traffic generator side drop rates every %s during the test duration...", interval)
	maxDropRateBps := float64(0)

	ctxWithNewDeadline, cancel := context.WithTimeout(ctx, e.testDuration)
	defer cancel()

	conditionFn := func(ctx context.Context) (bool, error) {
		statsGlobal, err := trexClient.GetGlobalStats()
		if statsGlobal.Result.MRxDropBps > maxDropRateBps {
			maxDropRateBps = statsGlobal.Result.MRxDropBps
		}
		return false, err
	}

	if err := wait.PollImmediateUntilWithContext(ctxWithNewDeadline, interval, conditionFn); err != nil {
		if !errors.Is(err, wait.ErrWaitTimeout) {
			return 0, fmt.Errorf("failed to poll global stats in trex-console: %w", err)
		}
		log.Printf("finished polling for drop rates")
	}

	return maxDropRateBps, nil
}
