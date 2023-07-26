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

package trex

import (
	"fmt"
	"path"
	"strings"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
)

const (
	CfgFileName                = "trex_cfg.yaml"
	StreamPyFileName           = "testpmd.py"
	StreamPeerParamsPyFileName = "testpmd_addr.py"
	ExecutionScriptName        = "run_trex_daemon"
	BinDirectory               = "/opt/trex"
	SystemdUnitFileName        = "trex.service"
)

type Config struct {
	masterCPU                      string
	latencyCPU                     string
	trafficCPUs                    string
	numOfTrafficCPUs               string
	portBandwidthGB                string
	trafficGeneratorEastMacAddress string
	trafficGeneratorWestMacAddress string
	DPDKEastMacAddress             string
	DPDKWestMacAddress             string
}

func NewConfig(cfg config.Config) Config {
	const (
		masterCPU        = "0"
		latencyCPU       = "1"
		trafficCPUs      = "2,3,4,5,6,7"
		numOfTrafficCPUs = "6"
	)
	return Config{
		masterCPU:                      masterCPU,
		latencyCPU:                     latencyCPU,
		trafficCPUs:                    trafficCPUs,
		numOfTrafficCPUs:               numOfTrafficCPUs,
		portBandwidthGB:                fmt.Sprintf("%d", cfg.PortBandwidthGB),
		trafficGeneratorEastMacAddress: cfg.TrafficGenEastMacAddress.String(),
		trafficGeneratorWestMacAddress: cfg.TrafficGenWestMacAddress.String(),
		DPDKEastMacAddress:             cfg.DPDKEastMacAddress.String(),
		DPDKWestMacAddress:             cfg.DPDKWestMacAddress.String(),
	}
}

func (c Config) GenerateCfgFile() string {
	const cfgTemplate = `- port_limit: 2
  version: 2
  interfaces:
    - %q
    - %q
  port_bandwidth_gb: %s
  port_info:
    - ip: 10.10.10.2
      default_gw: 10.10.10.1
    - ip: 10.10.20.2
      default_gw: 10.10.20.1
  platform:
    master_thread_id: %s
    latency_thread_id: %s
    dual_if:
      - socket: 0
        threads: [%s]
`
	return fmt.Sprintf(cfgTemplate,
		config.VMIEastNICPCIAddress,
		config.VMIWestNICPCIAddress,
		c.portBandwidthGB,
		c.masterCPU,
		c.latencyCPU,
		c.trafficCPUs,
	)
}

func (c Config) GenerateStreamPyFile() string {
	const streamPyTemplate = `from trex_stl_lib.api import *

from testpmd_addr import *

# Wild local MACs
mac_localport0=%q
mac_localport1=%q

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
        for i in range(%s):
            s.append(self.create_stream(direction = direction))
        return s

# dynamic load - used for trex console or simulator
def register():
    return STLS1()
`

	return fmt.Sprintf(streamPyTemplate,
		c.trafficGeneratorEastMacAddress,
		c.trafficGeneratorWestMacAddress,
		c.numOfTrafficCPUs,
	)
}

func (c Config) GenerateStreamAddrPyFile() string {
	const streamAddrPyTemplate = `# wild first XL710 mac
mac_telco0 = %q
# wild second XL710 mac
mac_telco1 = %q
# we don’t care of the IP in this phase
ip_telco0  = '10.0.0.1'
ip_telco1 = '10.1.1.1'
`
	return fmt.Sprintf(streamAddrPyTemplate,
		c.DPDKEastMacAddress,
		c.DPDKWestMacAddress,
	)
}

func (c Config) GenerateExecutionScript() string {
	sb := strings.Builder{}

	sb.WriteString("#!/usr/bin/env bash\n")
	sb.WriteString(fmt.Sprintf("./t-rex-64 --no-ofed-check --no-scapy-server --no-hw-flow-stat -i -c %s --iom 0\n", c.numOfTrafficCPUs))

	return sb.String()
}

func GenerateSystemdUnitFile() string {
	sb := strings.Builder{}

	sb.WriteString("[Unit]\n")
	sb.WriteString("Description=TRex Server\n")
	sb.WriteString("[Service]\n")
	sb.WriteString(fmt.Sprintf("WorkingDirectory=%s\n", BinDirectory))
	sb.WriteString(fmt.Sprintf("ExecStart=%s\n", path.Join(BinDirectory, ExecutionScriptName)))
	sb.WriteString("Restart=no\n")
	sb.WriteString("User=root\n")
	sb.WriteString("Group=root\n")

	return sb.String()
}
