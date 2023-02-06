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
	vmiEastMACAddress                          string
	vmiWestNICPCIAddress                       string
	vmiWestMACAddress                          string
	testDuration                               time.Duration
	trafficGeneratorPacketsPerSecondInMillions int
}

func New(client vmiSerialConsoleClient, podClient podExecuteClient, namespace string, cfg config.Config) Executor {
	return Executor{
		client:               client,
		podClient:            podClient,
		namespace:            namespace,
		vmiUsername:          config.VMIUsername,
		vmiPassword:          config.VMIPassword,
		vmiEastNICPCIAddress: config.VMIEastNICPCIAddress,
		vmiEastMACAddress:    cfg.DPDKEastMacAddress.String(),
		vmiWestNICPCIAddress: config.VMIWestNICPCIAddress,
		vmiWestMACAddress:    cfg.DPDKWestMacAddress.String(),
		testDuration:         cfg.TestDuration,
		trafficGeneratorPacketsPerSecondInMillions: cfg.TrafficGeneratorPacketsPerSecondInMillions,
	}
}

func (e Executor) Execute(ctx context.Context, vmiName, podName, podContainerName string) (status.Results, error) {
	if err := console.LoginToCentOS(e.client, e.namespace, vmiName, e.vmiUsername, e.vmiPassword); err != nil {
		return status.Results{}, fmt.Errorf("failed to login to VMI \"%s/%s\": %w", e.namespace, vmiName, err)
	}

	trexClient := NewTrexConsole(e.podClient, e.namespace, podName, podContainerName)

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

	const trafficSourcePort = 0
	log.Printf("Running traffic for %s...", e.testDuration.String())
	_, err = trexClient.StartTraffic(ctx, e.trafficGeneratorPacketsPerSecondInMillions, trafficSourcePort, e.testDuration)
	if err != nil {
		return status.Results{}, fmt.Errorf("failed to run traffic from trex-console on pod \"%s/%s\" side: %w",
			e.namespace, podName, err)
	}

	trafficGeneratorMaxDropRate, err := trexClient.MonitorDropRates(ctx, e.testDuration)
	log.Printf("traffic Generator Max Drop Rate: %fBps", trafficGeneratorMaxDropRate)
	if err != nil {
		return status.Results{}, err
	}

	var trafficGeneratorSrcPortStats portStats
	trafficGeneratorSrcPortStats, err = trexClient.GetPortStats(ctx, trafficSourcePort)
	if err != nil {
		return status.Results{}, err
	}
	log.Printf("traffic Generator port %d Packet output errors: %d", trafficSourcePort, trafficGeneratorSrcPortStats.Result.Oerrors)

	return status.Results{}, err
}

func (e Executor) runTestpmd(vmiName string) error {
	const batchTimeout = 30 * time.Second

	const testpmdPromt = "testpmd> "

	testpmdCmd := buildTestpmdCmd(e.vmiEastNICPCIAddress, e.vmiWestNICPCIAddress, e.vmiEastMACAddress, e.vmiWestMACAddress)

	resp, err := console.SafeExpectBatchWithResponse(e.client, e.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: testpmdCmd + "\n"},
			&expect.BExp{R: testpmdPromt},
			&expect.BSnd{S: "start" + "\n"},
			&expect.BExp{R: testpmdPromt},
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

	const testpmdPromt = "testpmd> "

	const testpmdCmd = "clear fwd stats all"

	_, err := console.SafeExpectBatchWithResponse(e.client, e.namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: testpmdCmd + "\n"},
			&expect.BExp{R: testpmdPromt},
		},
		batchTimeout,
	)

	if err != nil {
		return err
	}

	return nil
}

func buildTestpmdCmd(vmiEastNICPCIAddress, vmiWestNICPCIAddress, vmiEastMACAddress, vmiWestMACAddress string) string {
	const (
		cpuList       = "0-7"
		numberOfCores = 7
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
	sb.WriteString(fmt.Sprintf("--eth-peer=0,%s ", vmiEastMACAddress))
	sb.WriteString(fmt.Sprintf("--eth-peer=1,%s", vmiWestMACAddress))

	return sb.String()
}
