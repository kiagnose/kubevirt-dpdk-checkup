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
	"errors"
	"fmt"
	"testing"
	"time"

	expect "github.com/google/goexpect"

	assert "github.com/stretchr/testify/require"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/trex"
)

const (
	trafficGeneratorPacketsPerSecond = "1m"
	testDuration                     = time.Second
	verbosePrintsEnabled             = false

	portIdx = trex.SourcePort
)

func TestGetPortStatsSuccess(t *testing.T) {
	expecter := expecterStub{}
	c := trex.NewClient(expecter, trafficGeneratorPacketsPerSecond, testDuration, verbosePrintsEnabled)

	stats, err := c.GetPortStats(portIdx)
	assert.NoError(t, err, "GetPortStats returned an error")
	expected := trex.PortStats{
		ID:      "razdt1qe",
		Jsonrpc: "2.0",
		Result: trex.PortStatsResult{
			Ibytes:      68625,
			Ierrors:     10,
			Ipackets:    893,
			MCPUUtil:    11,
			MTotalRxBps: 6630.8935546875,
			MTotalRxPps: 9.337542533874512,
			MTotalTxBps: 1820482048,
			MTotalTxPps: 3346474.25,
			Obytes:      32640000000,
			Oerrors:     15,
			Opackets:    480000000,
		},
	}
	assert.Equal(t, expected, stats, "GetPortStats returned unexpected result")
}

func TestGetPortStatsFailure(t *testing.T) {
	t.Run("when batch execution fails", func(t *testing.T) {
		expectedBatchErr := errors.New("failed to run batch")
		expecter := &expecterStub{
			expectBatchErr: expectedBatchErr,
		}

		c := trex.NewClient(expecter, trafficGeneratorPacketsPerSecond, testDuration, verbosePrintsEnabled)

		stats, err := c.GetPortStats(portIdx)
		assert.ErrorContains(t, err, expectedBatchErr.Error())
		assert.Empty(t, stats)
	})
	t.Run("when batch times out", func(t *testing.T) {
		expectedTimeoutErr := errors.New("failed on timeout")
		expecter := &expecterStub{
			timeoutErr: expectedTimeoutErr,
		}
		c := trex.NewClient(expecter, trafficGeneratorPacketsPerSecond, testDuration, verbosePrintsEnabled)

		stats, err := c.GetPortStats(portIdx)
		assert.ErrorContains(t, err, expectedTimeoutErr.Error())
		assert.Empty(t, stats)
	})
}

func TestGetGlobalStatsSuccess(t *testing.T) {
	expecter := expecterStub{}
	c := trex.NewClient(expecter, trafficGeneratorPacketsPerSecond, testDuration, verbosePrintsEnabled)

	stats, err := c.GetGlobalStats()
	assert.NoError(t, err, "GetGlobalStats returned an error")

	expected := trex.GlobalStats{
		ID:      "10vw9s8b",
		Jsonrpc: "2.0",
		Result: trex.GlobalStatsResult{
			MActiveFlows:            1.0,
			MActiveSockets:          2,
			MBwPerCore:              6.808984279632568,
			MCPUUtil:                21.275409698486328,
			MCPUUtilRaw:             20.66666603088379,
			MOpenFlows:              3.0,
			MPlatformFactor:         1.0,
			MRxBps:                  4090272768.0,
			MRxCorePps:              4.0,
			MRxCPUUtil:              5.0,
			MRxDropBps:              6.0,
			MRxPps:                  7988813.0,
			MSocketUtil:             7.0,
			MTotalAllocError:        8,
			MTotalClients:           9,
			MTotalNatActive:         10,
			MTotalNatLearnError:     11,
			MTotalNatNoFid:          12,
			MTotalNatOpen:           13,
			MTotalNatSynWait:        14,
			MTotalNatTimeOut:        15,
			MTotalNatTimeOutWaitAck: 16,
			MTotalQueueDrop:         17,
			MTotalQueueFull:         18,
			MTotalRxBytes:           27642288128,
			MTotalRxPkts:            431910752,
			MTotalServers:           19,
			MTotalTxBytes:           29364454280,
			MTotalTxPkts:            431830210,
			MTxBps:                  4345917952.0,
			MTxCps:                  20.0,
			MTxExpectedBps:          21.0,
			MTxExpectedCps:          22.0,
			MTxExpectedPps:          23.0,
			MTxPps:                  7988831.0,
		},
	}

	assert.Equal(t, expected, stats, "GetGlobalStats returned unexpected result")
}

const (
	portStatsCmd    = "cd /opt/trex && echo \"verbose on;stats --port 0 -p\" | ./trex-console -q\n"
	portStatsOutput = "Using 'python3' as Python interpeter\r\n\r\n\r\n-=TRex Console v3.0=-\r\n\r\nType 'help' or '?' for supported act" +
		"ions\r\n\r\ntrex>\r\n\x1b[1m\x1b[32mverbose set to on\x1b[39m\x1b[22m\r\n\r\n\r\n\r\n[verbose] Sending Request To Server:\r\n\r" +
		"\n[\r\n    {\r\n        \"id\": \x1b[31m\"razdt1qe\"\x1b[0m,\r\n        \"jsonrpc\": \x1b[31m\"2.0\"\x1b[0m,\r\n        \"metho" +
		"d\": \x1b[31m\"get_port_stats\"\x1b[0m,\r\n        \"params\": {\r\n            \"api_h\": \x1b[31m\"hu7wm7qq\"\x1b[0m,\r\n    " +
		"        \"port_id\": 0\r\n        }\r\n    }\r\n]\r\n\r\n\r\n\r\n" +
		"[verbose] Server Response:\r\n\r\n" +
		"{\r\n" +
		"    \"id\": \x1b[31m\"razdt1qe\"\x1b[0m,\r\n" +
		"    \"jsonrpc\": \x1b[31m\"2.0\"\x1b[0m,\r\n" +
		"    \"result\": {\r\n" +
		"        \"ibytes\": 68625,\r\n" +
		"        \"ierrors\": 10,\r\n" +
		"        \"ipackets\": 893,\r\n" +
		"        \"m_cpu_util\": 11.0,\r\n" +
		"        \"m_total_rx_bps\": 6630.8935546875,\r\n" +
		"        \"m_total_rx_pps\": 9.337542533874512,\r\n" +
		"        \"m_total_tx_bps\": \x1b[94m1820482048\x1b[0m.0,\r\n" +
		"        \"m_total_tx_pps\": \x1b[94m3346474\x1b[0m.25,\r\n" +
		"        \"obytes\": \x1b[94m32640000000,\x1b[0m\r\n" +
		"        \"oerrors\": 15,\r\n" +
		"        \"opackets\": \x1b[94m480000000\r\n" +
		"\x1b[0m    }" +
		"\r\n}" +
		"\r\n\r\n\x1b[4m\x1b[36mPort Statistics\x1b[39m\x1b[24m\r\n\r\n   port    |         0         \r\n-----------+------------------" +
		"\r\nowner      |              \x1b[32mroot\x1b[39m \r\nlink       |                UP \r\nstate      |              \x1b[1mIDLE" +
		"\x1b[22m \r\nspeed      |           10 Gb/s \r\nCPU util.  |              \x1b[32m0.0\x1b[39m% \r\n--         |                " +
		"   \r\nTx bps L2  |         1.82 Gbps \r\nTx bps L1  |         2.36 Gbps \r\nTx pps     |         3.35 Mpps \r\nLine Util. |   " +
		"        \x1b[1m23.56 %\x1b[22m \r\n---        |                   \r\nRx bps     |             0 bps \r\nRx pps     |          " +
		"   0 pps \r\n----       |                   \r\nopackets   |                 0 \r\nipackets   |                 0 \r\nobytes   " +
		"  |                 0 \r\nibytes     |                 0 \r\ntx-pkts    |            0 pkts \r\nrx-pkts    |            0 pkts " +
		"\r\ntx-bytes   |               0 B \r\nrx-bytes   |               0 B \r\n-----      |                   \r\noerrors    |      " +
		"           \x1b[32m0\x1b[39m \r\nierrors    |                 \x1b[32m0\x1b[39m \r\n\r\ntrex>Shutting down RPC client\r\n\r\n[r" +
		"oot@dpdk-traffic-gen-jscpt trex]# "

	globalStatsCmd    = "cd /opt/trex && echo \"verbose on;stats -g\" | ./trex-console -q\n"
	globalStatsOutput = "Using 'python3' as Python interpeter\r\n\r\n\r\n-=TRex Console v3.0=-\r\n\r\nType 'help' or '?' for supported a" +
		"ctions\r\n\r\ntrex>\r\n\x1b[1m\x1b[32mverbose set to on\x1b[39m\x1b[22m\r\n\r\n\r\n\r\n[verbose] Sending Request To Server:\r\n" +
		"\r\n{\r\n    \"id\": \x1b[31m\"10vw9s8b\"\x1b[0m,\r\n    \"jsonrpc\": \x1b[31m\"2.0\"\x1b[0m,\r\n    \"method\": \x1b[31m\"get_" +
		"global_stats\"\x1b[0m,\r\n    \"params\": {\r\n        \"api_h\": \x1b[31m\"hu7wm7qq\"\x1b[0m\r\n    }\r\n}\r\n\r\n\r\n\r\n" +
		"[verbose] Server Response:\r\n\r\n" +
		"{\r\n" +
		"    \"id\": \x1b[31m\"10vw9s8b\"\x1b[0m,\r\n" +
		"    \"jsonrpc\": \x1b[31m\"2.0\"\x1b[0m,\r\n" +
		"    \"result\": {\r\n" +
		"        \"m_active_flows\": 1.0,\r\n" +
		"        \"m_active_sockets\": 2,\r\n" +
		"        \"m_bw_per_core\": \x1b[35m6.808984279632568\x1b[0m,\r\n" +
		"        \"m_cpu_util\": \x1b[94m21\x1b[0m.275409698486328,\r\n" +
		"        \"m_cpu_util_raw\": \x1b[94m20\x1b[0m.66666603088379,\r\n" +
		"        \"m_open_flows\": 3.0,\r\n" +
		"        \"m_platform_factor\": \x1b[35m1.0\x1b[0m,\r\n" +
		"        \"m_rx_bps\": \x1b[94m4090272768\x1b[0m.0,\r\n" +
		"        \"m_rx_core_pps\": 4.0,\r\n" +
		"        \"m_rx_cpu_util\": 5.0,\r\n" +
		"        \"m_rx_drop_bps\": 6.0,\r\n" +
		"        \"m_rx_pps\": \x1b[94m7988813\x1b[0m.0,\r\n" +
		"        \"m_socket_util\": 7.0,\r\n" +
		"        \"m_total_alloc_error\": 8,\r\n" +
		"        \"m_total_clients\": 9,\r\n" +
		"        \"m_total_nat_active\": 10,\r\n" +
		"        \"m_total_nat_learn_error\": 11,\r\n" +
		"        \"m_total_nat_no_fid\": 12,\r\n" +
		"        \"m_total_nat_open\": 13,\r\n" +
		"        \"m_total_nat_syn_wait\": 14,\r\n" +
		"        \"m_total_nat_time_out\": 15,\r\n" +
		"        \"m_total_nat_time_out_wait_ack\": 16,\r\n" +
		"        \"m_total_queue_drop\": 17,\r\n" +
		"        \"m_total_queue_full\": 18,\r\n" +
		"        \"m_total_rx_bytes\": \x1b[94m27642288128,\x1b[0m\r\n" +
		"        \"m_total_rx_pkts\": \x1b[94m431910752,\x1b[0m\r\n" +
		"        \"m_total_servers\": 19,\r\n" +
		"        \"m_total_tx_bytes\": \x1b[94m29364454280,\x1b[0m\r\n" +
		"        \"m_total_tx_pkts\": \x1b[94m431830210,\x1b[0m\r\n" +
		"        \"m_tx_bps\": \x1b[94m4345917952\x1b[0m.0,\r\n" +
		"        \"m_tx_cps\": 20.0,\r\n" +
		"        \"m_tx_expected_bps\": 21.0,\r\n" +
		"        \"m_tx_expected_cps\": 22.0,\r\n" +
		"        \"m_tx_expected_pps\": 23.0,\r\n" +
		"        \"m_tx_pps\": \x1b[94m7988831\x1b[0m.0\r\n" +
		"    }\r\n" +
		"}\r\n\r\n" +
		"\x1b[4m\x1b[36mGlobal Statistics\x1b[39m\x1b[24m\r\n\r\nconnection   : localhost, Port 4501                       total_tx_L2  " +
		": 4.35 Gbps                      \r\nversion      : STL @ v3.03                                total_tx_L1  : 5.62 Gbps        " +
		"              \r\ncpu_util.    : \x1b[32m21.28\x1b[39m% @ 6 cores (6 per dual port)         total_rx     : 4.09 Gbps           " +
		"           \r\nrx_cpu_util. : \x1b[32m0.0\x1b[39m% / 0 pps                               total_pps    : 7.99 Mpps              " +
		"        \r\nasync_util.  : \x1b[32m0\x1b[39m% / 0 bps                                 drop_rate    : \x1b[32m0 bps\x1b[39m     " +
		"                     \r\ntotal_cps.   : 0 cps                                      queue_full   : \x1b[32m0 pkts\x1b[39m       " +
		"                  \r\n\r\n" +
		"trex>Shutting down RPC client" +
		"\r\n\r\n" +
		"[root@dpdk-traffic-gen-jscpt trex]# "
)

type expecterStub struct {
	expectBatchErr error
	timeoutErr     error
}

func (es expecterStub) SafeExpectBatchWithResponse(expected []expect.Batcher, _ time.Duration) ([]expect.BatchRes, error) {
	if es.expectBatchErr != nil {
		return nil, es.expectBatchErr
	}
	if es.timeoutErr != nil {
		return nil, es.timeoutErr
	}

	var batchRes []expect.BatchRes
	switch expected[0].Arg() {
	case portStatsCmd:
		batchRes = append(batchRes,
			expect.BatchRes{
				Idx:    1,
				Output: portStatsOutput,
			})
	case globalStatsCmd:
		batchRes = append(batchRes,
			expect.BatchRes{
				Idx:    1,
				Output: globalStatsOutput,
			})
	default:
		return nil, fmt.Errorf("command not recognized: %s", expected[0].Arg())
	}

	return batchRes, nil
}
