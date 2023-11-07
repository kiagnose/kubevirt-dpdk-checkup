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

package testpmd_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	expect "github.com/google/goexpect"
	assert "github.com/stretchr/testify/require"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/executor/testpmd"
)

const (
	vmiUnderTestEastNICPCIAddress = "0000:06:00.0"
	trafficGenEastMACAddress      = "60:94:19:c9:ac:01"
	vmiUnderTestWestNICPCIAddress = "0000:07:00.0"
	trafficGenWestMACAddress      = "60:94:19:c9:ac:02"
	verbosePrintsEnabled          = false
)

func TestGetPortStatsSuccess(t *testing.T) {
	expecter := expecterStub{}
	c := testpmd.NewTestpmdConsole(
		expecter,
		vmiUnderTestEastNICPCIAddress,
		trafficGenEastMACAddress,
		vmiUnderTestWestNICPCIAddress,
		trafficGenWestMACAddress,
		verbosePrintsEnabled,
	)

	stats, err := c.GetStats()
	assert.NoError(t, err, "GetStats returned an error")
	expected := [testpmd.StatsArraySize]testpmd.PortStats{
		{
			RXPackets: 480000001,
			RXDropped: 2,
			RXTotal:   480000003,
			TXPackets: 4,
			TXDropped: 5,
			TXTotal:   6,
		},
		{
			RXPackets: 7,
			RXDropped: 8,
			RXTotal:   9,
			TXPackets: 480000010,
			TXDropped: 11,
			TXTotal:   480000012,
		},
		{
			RXPackets: 480000013,
			RXDropped: 14,
			RXTotal:   480000015,
			TXPackets: 480000016,
			TXDropped: 17,
			TXTotal:   480000018,
		},
	}
	assert.Equal(t, expected, stats, "GetStats returned unexpected result")
}

func TestGetPortStatsFailure(t *testing.T) {
	t.Run("when batch execution fails", func(t *testing.T) {
		expectedBatchErr := errors.New("failed to run batch")
		expecter := &expecterStub{
			expectBatchErr: expectedBatchErr,
		}

		c := testpmd.NewTestpmdConsole(
			expecter,
			vmiUnderTestEastNICPCIAddress,
			trafficGenEastMACAddress,
			vmiUnderTestWestNICPCIAddress,
			trafficGenWestMACAddress,
			verbosePrintsEnabled,
		)

		stats, err := c.GetStats()
		assert.ErrorContains(t, err, expectedBatchErr.Error())
		assert.Empty(t, stats)
	})
	t.Run("when batch times out", func(t *testing.T) {
		expectedTimeoutErr := errors.New("failed on timeout")
		expecter := &expecterStub{
			timeoutErr: expectedTimeoutErr,
		}
		c := testpmd.NewTestpmdConsole(
			expecter,
			vmiUnderTestEastNICPCIAddress,
			trafficGenEastMACAddress,
			vmiUnderTestWestNICPCIAddress,
			trafficGenWestMACAddress,
			verbosePrintsEnabled,
		)
		stats, err := c.GetStats()

		assert.ErrorContains(t, err, expectedTimeoutErr.Error())
		assert.Empty(t, stats)
	})
}

type expecterStub struct {
	expectBatchErr error
	timeoutErr     error
}

const (
	getStatsCmd    = "show fwd stats all\n"
	getStatsOutput = "" +
		"  ------- Forward Stats for RX Port= 0/Queue= 0 -> TX Port= 1/Queue= 0 -------\n" +
		"  RX-packets: 160000000      TX-packets: 160000000      TX-dropped: 0             \n" +
		"\n" +
		"  ------- Forward Stats for RX Port= 0/Queue= 1 -> TX Port= 1/Queue= 1 -------\n" +
		"  RX-packets: 80000000       TX-packets: 80000000       TX-dropped: 0             \n" +
		"\n" +
		"  ------- Forward Stats for RX Port= 0/Queue= 2 -> TX Port= 1/Queue= 2 -------\n" +
		"  RX-packets: 80000000       TX-packets: 80000000       TX-dropped: 0             \n" +
		"\n" +
		"  ------- Forward Stats for RX Port= 0/Queue= 3 -> TX Port= 1/Queue= 3 -------\n" +
		"  RX-packets: 160000000      TX-packets: 160000000      TX-dropped: 0" +
		"  ---------------------- Forward statistics for port 0  ----------------------\n" +
		"  RX-packets: 480000001     RX-dropped: 2             RX-total: 480000003\n" +
		"  TX-packets: 4              TX-dropped: 5             TX-total: 6\n" +
		"  ----------------------------------------------------------------------------\n" +
		"\n" +
		"  ---------------------- Forward statistics for port 1  ----------------------\n" +
		"  RX-packets: 7              RX-dropped: 8             RX-total: 9\n" +
		"  TX-packets: 480000010     TX-dropped: 11             TX-total: 480000012\n" +
		"  ----------------------------------------------------------------------------\n" +
		"\n" +
		"  +++++++++++++++ Accumulated forward statistics for all ports+++++++++++++++\n" +
		"  RX-packets: 480000013     RX-dropped: 14             RX-total: 480000015\n" +
		"  TX-packets: 480000016     TX-dropped: 17             TX-total: 480000018\n" +
		"  ++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++\n" +
		"testpmd> "
)

func (es expecterStub) SafeExpectBatchWithResponse(expected []expect.Batcher, _ time.Duration) ([]expect.BatchRes, error) {
	if es.expectBatchErr != nil {
		return nil, es.expectBatchErr
	}
	if es.timeoutErr != nil {
		return nil, es.timeoutErr
	}

	var batchRes []expect.BatchRes
	switch expected[0].Arg() {
	case getStatsCmd:
		batchRes = append(batchRes,
			expect.BatchRes{
				Idx:    1,
				Output: getStatsOutput,
			})
	default:
		return nil, fmt.Errorf("command not recognized: %s", expected[0].Arg())
	}

	return batchRes, nil
}
