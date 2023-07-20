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
	"net"
	"testing"

	assert "github.com/stretchr/testify/require"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/trex"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
)

func TestGetTrexCfgFile(t *testing.T) {
	cfgs := createSampleConfigs()
	cfgFile := cfgs.GenerateCfgFile()

	const expectedCfgFile = `- port_limit: 2
  version: 2
  interfaces:
    - "0000:06:00.0"
    - "0000:07:00.0"
  port_bandwidth_gb: 40
  port_info:
    - ip: 10.10.10.2
      default_gw: 10.10.10.1
    - ip: 10.10.20.2
      default_gw: 10.10.20.1
  platform:
    master_thread_id: 0
    latency_thread_id: 1
    dual_if:
      - socket: 0
        threads: [2,3,4,5,6,7]
`
	assert.Equal(t, expectedCfgFile, cfgFile)
}

func TestGetTestpmdStreamPyFile(t *testing.T) {
	cfgs := createSampleConfigs()
	pyFile := cfgs.GenerateStreamPyFile()

	const expectedPyFile = `from trex_stl_lib.api import *

from testpmd_addr import *

# Wild local MACs
mac_localport0="00:00:00:00:00:00"
mac_localport1="00:00:00:00:00:01"

class STLS1(object):

    def __init__ (self):
        self.fsize  =64; # the size of the packet
        self.number = 0

    def create_stream (self, direction = 0):
        size = self.fsize - 4; # HW will add 4 bytes ethernet FCS
        dport = 1026 + self.number
        self.number = self.number + 1
        if direction == 0:
            base_pkt =  Ether(dst=mac_telco0,src=mac_localport0)/IP(src="16.0.0.1",dst=ip_telco0)/UDP(dport=dport,sport=1026)
        else:
            base_pkt =  Ether(dst=mac_telco1,src=mac_localport1)/IP(src="16.1.0.1",dst=ip_telco1)/UDP(dport=dport,sport=1026)
        pad = (60 - len(base_pkt)) * 'x'

        return STLStream(
            packet =
            STLPktBuilder(
                pkt = base_pkt / pad
            ),
            mode = STLTXCont())


    def get_streams (self, direction = 0, **kwargs):
        # create multiple streams, one stream per core generating traffic...
        s = []
        for i in range(6):
            s.append(self.create_stream(direction = direction))
        return s

# dynamic load - used for trex console or simulator
def register():
    return STLS1()
`
	assert.Equal(t, expectedPyFile, pyFile)
}

func TestGetTestpmdStreamAddrPyFile(t *testing.T) {
	cfgs := createSampleConfigs()
	addrPyFile := cfgs.GenerateStreamAddrPyFile()

	const expectedAddrPyFile = `# wild first XL710 mac
mac_telco0 = "00:00:00:00:00:02"
# wild second XL710 mac
mac_telco1 = "00:00:00:00:00:03"
# we don’t care of the IP in this phase
ip_telco0  = '10.0.0.1'
ip_telco1 = '10.1.1.1'
`
	assert.Equal(t, expectedAddrPyFile, addrPyFile)
}

func TestExecutionScript(t *testing.T) {
	trexConfig := createSampleConfigs()

	actualExecutionScript := trexConfig.GenerateExecutionScript()

	expextedExecutionScript := `#!/usr/bin/env bash
./t-rex-64 --no-ofed-check --no-scapy-server --no-hw-flow-stat -i -c 6 --iom 0
`

	assert.Equal(t, expextedExecutionScript, actualExecutionScript)
}

func createSampleConfigs() trex.Config {
	trafficGeneratorEastMacAddress, _ := net.ParseMAC("00:00:00:00:00:00")
	trafficGeneratorWestMacAddress, _ := net.ParseMAC("00:00:00:00:00:01")
	DPDKEastMacAddress, _ := net.ParseMAC("00:00:00:00:00:02")
	DPDKWestMacAddress, _ := net.ParseMAC("00:00:00:00:00:03")
	cfg := config.Config{
		PortBandwidthGB:                40,
		TrafficGeneratorEastMacAddress: trafficGeneratorEastMacAddress,
		TrafficGeneratorWestMacAddress: trafficGeneratorWestMacAddress,
		DPDKEastMacAddress:             DPDKEastMacAddress,
		DPDKWestMacAddress:             DPDKWestMacAddress,
	}
	return trex.NewConfig(cfg)
}
