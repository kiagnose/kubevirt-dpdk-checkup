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

package config

import (
	"errors"
	"net"
	"strconv"
	"time"

	kconfig "github.com/kiagnose/kiagnose/kiagnose/config"
)

const (
	NUMASocketParamName                                 = "NUMASocket"
	NetworkAttachmentDefinitionNameParamName            = "networkAttachmentDefinitionName"
	TrafficGeneratorNodeLabelSelectorParamName          = "trafficGeneratorNodeLabelSelector"
	DPDKNodeLabelSelectorParamName                      = "DPDKNodeLabelSelector"
	TrafficGeneratorPacketsPerSecondInMillionsParamName = "trafficGeneratorPacketsPerSecondInMillions"
	PortBandwidthGBParamName                            = "portBandwidthGB"
	TrafficGeneratorMacAddressParamName                 = "trafficGeneratorMacAddress"
	DPDKMacAddressParamName                             = "DPDKMacAddress"
	TestDurationParamName                               = "testDuration"
)

const (
	TrafficGeneratorPacketsPerSecondInMillionsDefault = 14
	PortBandwidthGBDefault                            = 10
	TrafficGeneratorMacAddressDefault                 = "50:00:00:00:00:01"
	DPDKMacAddressDefault                             = "60:00:00:00:00:01"
	TestDurationDefault                               = 5 * time.Minute
)

var (
	ErrInvalidNUMASocket                                 = errors.New("invalid NUMA Socket")
	ErrInvalidNetworkAttachmentDefinitionName            = errors.New("invalid Network-Attachment-Definition Name")
	ErrInvalidTrafficGeneratorNodeLabelSelector          = errors.New("invalid Traffic Generator Node Label Selector")
	ErrInvalidDPDKNodeLabelSelector                      = errors.New("invalid DPDK Node Label Selector")
	ErrInvalidTrafficGeneratorPacketsPerSecondInMillions = errors.New("invalid Traffic Generator Packets Per Second In Millions")
	ErrInvalidPortBandwidthGB                            = errors.New("invalid Port Bandwidth [GB]")
	ErrInvalidTrafficGeneratorMacAddress                 = errors.New("invalid Traffic Generator MAC Address")
	ErrInvalidDPDKMacAddress                             = errors.New("invalid DPDK MAC Address")
	ErrInvalidTestDuration                               = errors.New("invalid Test Duration")
)

type Config struct {
	PodName                                    string
	PodUID                                     string
	NUMASocket                                 int
	NetworkAttachmentDefinitionName            string
	TrafficGeneratorNodeLabelSelector          string
	DPDKNodeLabelSelector                      string
	TrafficGeneratorPacketsPerSecondInMillions int
	PortBandwidthGB                            int
	TrafficGeneratorMacAddress                 net.HardwareAddr
	DPDKMacAddress                             net.HardwareAddr
	TestDuration                               time.Duration
}

func New(baseConfig kconfig.Config) (Config, error) {
	trafficGeneratorMacAddressDefault, _ := net.ParseMAC(TrafficGeneratorMacAddressDefault)
	dpdkMacAddressDefault, _ := net.ParseMAC(DPDKMacAddressDefault)
	newConfig := Config{
		PodName:                           baseConfig.PodName,
		PodUID:                            baseConfig.PodUID,
		NetworkAttachmentDefinitionName:   baseConfig.Params[NetworkAttachmentDefinitionNameParamName],
		TrafficGeneratorNodeLabelSelector: baseConfig.Params[TrafficGeneratorNodeLabelSelectorParamName],
		DPDKNodeLabelSelector:             baseConfig.Params[DPDKNodeLabelSelectorParamName],
		TrafficGeneratorPacketsPerSecondInMillions: TrafficGeneratorPacketsPerSecondInMillionsDefault,
		PortBandwidthGB:            PortBandwidthGBDefault,
		TrafficGeneratorMacAddress: trafficGeneratorMacAddressDefault,
		DPDKMacAddress:             dpdkMacAddressDefault,
		TestDuration:               TestDurationDefault,
	}

	var rawNUMASocket string
	if rawNUMASocket = baseConfig.Params[NUMASocketParamName]; rawNUMASocket == "" {
		return Config{}, ErrInvalidNUMASocket
	}
	numaSocket, err := strconv.Atoi(rawNUMASocket)
	if err != nil || numaSocket < 0 {
		return Config{}, ErrInvalidNUMASocket
	}
	newConfig.NUMASocket = numaSocket

	if newConfig.NetworkAttachmentDefinitionName == "" {
		return Config{}, ErrInvalidNetworkAttachmentDefinitionName
	}

	return setOptionalParams(baseConfig, newConfig)
}

func setOptionalParams(baseConfig kconfig.Config, newConfig Config) (Config, error) {
	if rawTrafficGeneratorPacketsPerSecondInMillions :=
		baseConfig.Params[TrafficGeneratorPacketsPerSecondInMillionsParamName]; rawTrafficGeneratorPacketsPerSecondInMillions != "" {
		trafficGeneratorPacketsPerSecondInMillions, err := strconv.Atoi(rawTrafficGeneratorPacketsPerSecondInMillions)
		if err != nil || trafficGeneratorPacketsPerSecondInMillions < 0 {
			return Config{}, ErrInvalidTrafficGeneratorPacketsPerSecondInMillions
		}
		newConfig.TrafficGeneratorPacketsPerSecondInMillions = trafficGeneratorPacketsPerSecondInMillions
	}

	if rawPortBandwidthGB := baseConfig.Params[PortBandwidthGBParamName]; rawPortBandwidthGB != "" {
		portBandwidthGB, err := strconv.Atoi(rawPortBandwidthGB)
		if err != nil || portBandwidthGB <= 0 {
			return Config{}, ErrInvalidPortBandwidthGB
		}
		newConfig.PortBandwidthGB = portBandwidthGB
	}

	if rawTrafficGeneratorMacAddress := baseConfig.Params[TrafficGeneratorMacAddressParamName]; rawTrafficGeneratorMacAddress != "" {
		trafficGeneratorMacAddress, err := net.ParseMAC(rawTrafficGeneratorMacAddress)
		if err != nil {
			return Config{}, ErrInvalidTrafficGeneratorMacAddress
		}
		newConfig.TrafficGeneratorMacAddress = trafficGeneratorMacAddress
	}

	if rawDPDKMacAddress := baseConfig.Params[DPDKMacAddressParamName]; rawDPDKMacAddress != "" {
		dpdkMacAddress, err := net.ParseMAC(rawDPDKMacAddress)
		if err != nil {
			return Config{}, ErrInvalidDPDKMacAddress
		}
		newConfig.DPDKMacAddress = dpdkMacAddress
	}

	if rawTestDuration := baseConfig.Params[TestDurationParamName]; rawTestDuration != "" {
		testDuration, err := time.ParseDuration(rawTestDuration)
		if err != nil {
			return Config{}, ErrInvalidTestDuration
		}
		newConfig.TestDuration = testDuration
	}

	return newConfig, nil
}
