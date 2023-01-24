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
	"fmt"
	"net"
	"strconv"
	"strings"
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
	TrafficGeneratorEastMacAddressParamName             = "trafficGeneratorEastMacAddress"
	TrafficGeneratorWestMacAddressParamName             = "trafficGeneratorWestMacAddress"
	DPDKEastMacAddressParamName                         = "DPDKEastMacAddress"
	DPDKWestMacAddressParamName                         = "DPDKWestMacAddress"
	TestDurationParamName                               = "testDuration"
	labelFormat                                         = "label format is key=value"
)

const (
	TrafficGeneratorPacketsPerSecondInMillionsDefault = 14
	PortBandwidthGBDefault                            = 10
	TrafficGeneratorEastMacAddressDefault             = "50:00:00:00:00:01"
	TrafficGeneratorWestMacAddressDefault             = "50:00:00:00:00:02"
	DPDKEastMacAddressDefault                         = "60:00:00:00:00:01"
	DPDKWestMacAddressDefault                         = "60:00:00:00:00:02"
	TestDurationDefault                               = 5 * time.Minute
)

var (
	ErrInvalidNUMASocket                                 = errors.New("invalid NUMA Socket")
	ErrInvalidNetworkAttachmentDefinitionName            = errors.New("invalid Network-Attachment-Definition Name")
	ErrInvalidTrafficGeneratorNodeLabelSelector          = errors.New("invalid Traffic Generator Node Label Selector. " + labelFormat)
	ErrInvalidDPDKNodeLabelSelector                      = errors.New("invalid DPDK Node Label Selector. " + labelFormat)
	ErrInvalidTrafficGeneratorPacketsPerSecondInMillions = errors.New("invalid Traffic Generator Packets Per Second In Millions")
	ErrInvalidPortBandwidthGB                            = errors.New("invalid Port Bandwidth [GB]")
	ErrInvalidTrafficGeneratorEastMacAddress             = errors.New("invalid Traffic Generator East MAC Address")
	ErrInvalidTrafficGeneratorWestMacAddress             = errors.New("invalid Traffic Generator West MAC Address")
	ErrInvalidDPDKEastMacAddress                         = errors.New("invalid DPDK East MAC Address")
	ErrInvalidDPDKWestMacAddress                         = errors.New("invalid DPDK West MAC Address")
	ErrInvalidTestDuration                               = errors.New("invalid Test Duration")
)

type Label struct {
	Key, Value string
}

type Config struct {
	PodName                                    string
	PodUID                                     string
	NUMASocket                                 int
	NetworkAttachmentDefinitionName            string
	TrafficGeneratorNodeLabelSelector          Label
	DPDKNodeLabelSelector                      Label
	TrafficGeneratorPacketsPerSecondInMillions int
	PortBandwidthGB                            int
	TrafficGeneratorEastMacAddress             net.HardwareAddr
	TrafficGeneratorWestMacAddress             net.HardwareAddr
	DPDKEastMacAddress                         net.HardwareAddr
	DPDKWestMacAddress                         net.HardwareAddr
	TestDuration                               time.Duration
}

func New(baseConfig kconfig.Config) (Config, error) {
	trafficGeneratorEastMacAddressDefault, _ := net.ParseMAC(TrafficGeneratorEastMacAddressDefault)
	trafficGeneratorWestMacAddressDefault, _ := net.ParseMAC(TrafficGeneratorWestMacAddressDefault)
	dpdkEastMacAddressDefault, _ := net.ParseMAC(DPDKEastMacAddressDefault)
	dpdkWestMacAddressDefault, _ := net.ParseMAC(DPDKWestMacAddressDefault)
	newConfig := Config{
		PodName:                         baseConfig.PodName,
		PodUID:                          baseConfig.PodUID,
		NetworkAttachmentDefinitionName: baseConfig.Params[NetworkAttachmentDefinitionNameParamName],
		TrafficGeneratorPacketsPerSecondInMillions: TrafficGeneratorPacketsPerSecondInMillionsDefault,
		PortBandwidthGB:                PortBandwidthGBDefault,
		TrafficGeneratorEastMacAddress: trafficGeneratorEastMacAddressDefault,
		TrafficGeneratorWestMacAddress: trafficGeneratorWestMacAddressDefault,
		DPDKEastMacAddress:             dpdkEastMacAddressDefault,
		DPDKWestMacAddress:             dpdkWestMacAddressDefault,
		TestDuration:                   TestDurationDefault,
	}

	var rawNUMASocket string
	if rawNUMASocket = baseConfig.Params[NUMASocketParamName]; rawNUMASocket == "" {
		return Config{}, ErrInvalidNUMASocket
	}
	numaSocket, err := parseNonNegativeInt(rawNUMASocket)
	if err != nil {
		return Config{}, ErrInvalidNUMASocket
	}
	newConfig.NUMASocket = numaSocket

	if newConfig.NetworkAttachmentDefinitionName == "" {
		return Config{}, ErrInvalidNetworkAttachmentDefinitionName
	}

	return setOptionalParams(baseConfig, newConfig)
}

func setOptionalParams(baseConfig kconfig.Config, newConfig Config) (Config, error) {
	var err error

	if newConfig, err = setLabelSelectorParams(baseConfig, newConfig); err != nil {
		return Config{}, err
	}

	if newConfig, err = setMacAddressParams(baseConfig, newConfig); err != nil {
		return Config{}, err
	}

	if rawVal :=
		baseConfig.Params[TrafficGeneratorPacketsPerSecondInMillionsParamName]; rawVal != "" {
		newConfig.TrafficGeneratorPacketsPerSecondInMillions, err = parseNonNegativeInt(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGeneratorPacketsPerSecondInMillions
		}
	}

	if rawVal := baseConfig.Params[PortBandwidthGBParamName]; rawVal != "" {
		newConfig.PortBandwidthGB, err = parseNonZeroPositiveInt(rawVal)
		if err != nil {
			return Config{}, ErrInvalidPortBandwidthGB
		}
	}

	if rawVal := baseConfig.Params[TestDurationParamName]; rawVal != "" {
		newConfig.TestDuration, err = time.ParseDuration(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTestDuration
		}
	}

	return newConfig, nil
}

func setLabelSelectorParams(baseConfig kconfig.Config, newConfig Config) (Config, error) {
	var err error
	if rawVal := baseConfig.Params[TrafficGeneratorNodeLabelSelectorParamName]; rawVal != "" {
		newConfig.TrafficGeneratorNodeLabelSelector, err = ParseLabel(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGeneratorNodeLabelSelector
		}
	}

	if rawVal := baseConfig.Params[DPDKNodeLabelSelectorParamName]; rawVal != "" {
		newConfig.DPDKNodeLabelSelector, err = ParseLabel(rawVal)
		if err != nil {
			return Config{}, ErrInvalidDPDKNodeLabelSelector
		}
	}
	return newConfig, nil
}

func setMacAddressParams(baseConfig kconfig.Config, newConfig Config) (Config, error) {
	var err error
	if rawVal := baseConfig.Params[TrafficGeneratorEastMacAddressParamName]; rawVal != "" {
		newConfig.TrafficGeneratorEastMacAddress, err = net.ParseMAC(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGeneratorEastMacAddress
		}
	}

	if rawVal := baseConfig.Params[TrafficGeneratorWestMacAddressParamName]; rawVal != "" {
		newConfig.TrafficGeneratorWestMacAddress, err = net.ParseMAC(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGeneratorWestMacAddress
		}
	}

	if rawVal := baseConfig.Params[DPDKEastMacAddressParamName]; rawVal != "" {
		newConfig.DPDKEastMacAddress, err = net.ParseMAC(rawVal)
		if err != nil {
			return Config{}, ErrInvalidDPDKEastMacAddress
		}
	}

	if rawVal := baseConfig.Params[DPDKWestMacAddressParamName]; rawVal != "" {
		newConfig.DPDKWestMacAddress, err = net.ParseMAC(rawVal)
		if err != nil {
			return Config{}, ErrInvalidDPDKWestMacAddress
		}
	}
	return newConfig, nil
}

func parseNonZeroPositiveInt(rawVal string) (int, error) {
	val, err := strconv.Atoi(rawVal)
	if err != nil || val <= 0 {
		return 0, errors.New("parameter is zero or negative")
	}
	return val, nil
}

func parseNonNegativeInt(rawVal string) (int, error) {
	val, err := strconv.Atoi(rawVal)
	if err != nil || val < 0 {
		return 0, errors.New("parameter is negative")
	}
	return val, nil
}

func ParseLabel(str string) (Label, error) {
	const labelArgumentsNum = 2
	s := strings.Split(str, "=")
	if len(s) != labelArgumentsNum {
		return Label{}, fmt.Errorf("invalid label format")
	}
	return Label{s[0], s[1]}, nil
}
