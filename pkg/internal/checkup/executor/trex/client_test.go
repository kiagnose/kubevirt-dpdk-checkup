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

package trex_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/executor/trex"
)

const (
	testNamespace = "testns"
	podName       = "testpod"
	containerName = "testcontainer"
	verbosePrint  = false
)

func TestGetPortStats(t *testing.T) {
	c := trex.NewClient(&stubPodExecuteClient{}, testNamespace, podName, containerName, verbosePrint)
	stats, err := c.GetPortStats(context.Background(), 0)
	assert.NoError(t, err, "GetPortStats returned an error")
	expected := trex.PortStats{
		ID:      "n5thi6a2",
		Jsonrpc: "2.0",
		Result: trex.PortStatsResult{
			Ibytes:      68625,
			Ierrors:     10,
			Ipackets:    893,
			MCPUUtil:    11,
			MTotalRxBps: 6630.8935546875,
			MTotalRxPps: 9.337542533874512,
			MTotalTxBps: 12,
			MTotalTxPps: 13,
			Obytes:      14,
			Oerrors:     15,
			Opackets:    16,
		},
	}
	assert.Equal(t, expected, stats, "GetPortStats returned unexpected result")
}

func TestGetGlobalStats(t *testing.T) {
	c := trex.NewClient(&stubPodExecuteClient{}, testNamespace, podName, containerName, verbosePrint)
	stats, err := c.GetGlobalStats(context.Background())
	assert.NoError(t, err, "GetGlobalStats returned an error")
	expected := trex.GlobalStats{
		ID:      "ui6k5sf7",
		Jsonrpc: "2.0",
		Result: trex.GlobalStatsResult{
			MPlatformFactor: 1,
			MRxBps:          9416.4375,
			MRxDropBps:      1000,
			MRxPps:          14.424873352050781,
			MTotalRxBytes:   510766,
			MTotalRxPkts:    6600,
		},
	}
	assert.Equal(t, expected, stats, "GetGlobalStats returned unexpected result")
}

type stubPodExecuteClient struct{}

func (c stubPodExecuteClient) ExecuteCommandOnPod(ctx context.Context, namespace, name, containerName string,
	command []string) (stdout, stderr string, err error) {
	if len(command) == 0 {
		return "", "", fmt.Errorf("command is empty")
	}

	const (
		globalStatsOutput = "Using 'python3' as Python interpeter\n\n\n-=TRex Console v3.0=-\n\n" +
			"Type 'help' or '?' for supported actions\n\ntrex>\nverbose set to on\n\n\n\n[verbose] Sending Request To" +
			" Server:\n\n{\n    \"id\": \"ui6k5sf7\",\n    \"jsonrpc\": \"2.0\",\n    \"method\": \"get_global_stats\"" +
			",\n    \"params\": {\n        \"api_h\": \"kGQyN8we\"\n    }\n}\n\n\n\n" +
			"[verbose] Server Response:\n\n" +
			`{	"id":"ui6k5sf7",
				"jsonrpc":"2.0",
				"result":{
					"m_active_flows":0.0,
					"m_active_sockets":0,
					"m_bw_per_core":0.0,
					"m_cpu_util":0.0,
					"m_cpu_util_raw":0.0,
					"m_open_flows":0.0,
					"m_platform_factor":1.0,
					"m_rx_bps":9416.4375,
					"m_rx_core_pps":0.0,
					"m_rx_cpu_util":0.0,
					"m_rx_drop_bps":1000.0,
					"m_rx_pps":14.424873352050781,
					"m_socket_util":0.0,
					"m_total_alloc_error":0,
					"m_total_clients":0,
					"m_total_nat_active ":0,
					"m_total_nat_learn_error":0,
					"m_total_nat_no_fid ":0,
					"m_total_nat_open   ":0,
					"m_total_nat_syn_wait":0,
					"m_total_nat_time_out":0,
					"m_total_nat_time_out_wait_ack":0,
					"m_total_queue_drop":0,
					"m_total_queue_full":0,
					"m_total_rx_bytes":510766,
					"m_total_rx_pkts":6600,
					"m_total_servers":0,
					"m_total_tx_bytes":0,
					"m_total_tx_pkts":0,
					"m_tx_bps":0.0,
					"m_tx_cps":0.0,
					"m_tx_expected_bps":0.0,
					"m_tx_expected_cps":0.0,
					"m_tx_expected_pps":0.0,
					"m_tx_pps":0.0
					}
			}` +
			"\n\nGlobal Statistitcs\n\nconnection   : localhost, Port 4501                       total_tx_L2  : 0 bps" +
			"                          \nversion      : STL @ v2.87                                total_tx_L1  : 0 b" +
			"ps                          \ncpu_util.    : 0.0% @ 6 cores (6 per dual port)           total_rx     : 9" +
			".42 Kbps                      \nrx_cpu_util. : 0.0% / 0 pps                               total_pps    :" +
			" 0 pps                          \nasync_util.  : 0% / 0 bps                                 drop_rate   " +
			" : 0 bps                          \ntotal_cps.   : 0 cps                                      queue_full" +
			"   : 0 pkts                         \n\ntrex>Shutting down RPC client\n"
		portStatsOutput = "Using 'python3' as Python interpeter\n\n\n-=TRex Console v3.0=-\n\nType 'help' or '?' for supported actions\n\ntr" +
			"ex>\nverbose set to on\n\n\n\n[verbose] Sending Request To Server:\n\n[\n    {\n        \"id\": \"n5thi6a2\",\n        \"jsonrp" +
			"c\": \"2.0\",\n        \"method\": \"get_port_stats\",\n        \"params\": {\n            \"api_h\": \"kGQyN8we\",\n          " +
			"  \"port_id\": 0\n        }\n    }\n]\n\n\n\n" +
			"[verbose] Server Response:\n\n" +
			`{	"id": "n5thi6a2",
				"jsonrpc": "2.0",
				"result": {
					"ibytes": 68625,
					"ierrors": 10,
					"ipackets": 893,
					"m_cpu_util": 11.0,
					"m_total_rx_bps": 6630.8935546875,
					"m_total_rx_pps": 9.337542533874512,
					"m_total_tx_bps": 12.0,
					"m_total_tx_pps": 13.0,
					"obytes": 14,
					"oerrors": 15,
					"opackets": 16
				}
			}` +
			"\n\nPort Statistics\n\n   port    |         0         \n-----------+------------------\nowner      |              root \nlink  " +
			"     |                UP \nstate      |              IDLE \nspeed      |           10 Gb/s \nCPU util.  |              0.0% \n-" +
			"-         |                   \nTx bps L2  |             0 bps \nTx bps L1  |             0 bps \nTx pps     |             0 pp" +
			"s \nLine Util. |               0 % \n---        |                   \nRx bps     |         6.63 Kbps \nRx pps     |          9." +
			"34 pps \n----       |                   \nopackets   |                 0 \nipackets   |                 0 \nobytes     |       " +
			"          0 \nibytes     |                 0 \ntx-pkts    |            0 pkts \nrx-pkts    |            0 pkts \ntx-bytes   |  " +
			"             0 B \nrx-bytes   |               0 B \n-----      |                   \noerrors    |                 0 \nierrors  " +
			"  |                 0 \n\ntrex>Shutting down RPC client"
	)

	globalStatsCommand := composeTrexConsoleRequest("verbose on;stats -g")
	portStatsCommand := composeTrexConsoleRequest("verbose on;stats --port 0 -p")
	switch strings.Join(command, " ") {
	case globalStatsCommand:
		return globalStatsOutput, "", nil
	case portStatsCommand:
		return portStatsOutput, "", nil
	default:
		return "", "", fmt.Errorf("unknown command: %v", command)
	}
}

func composeTrexConsoleRequest(command string) string {
	return fmt.Sprintf("/bin/sh -c echo %q | ./trex-console -q", command)
}
