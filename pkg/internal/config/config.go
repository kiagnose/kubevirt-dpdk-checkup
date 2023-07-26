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
	"crypto/rand"
	"errors"
	"net"
	"regexp"
	"strconv"
	"time"

	kconfig "github.com/kiagnose/kiagnose/kiagnose/config"
)

const (
	NetworkAttachmentDefinitionNameParamName   = "networkAttachmentDefinitionName"
	TrafficGeneratorImageParamName             = "trafficGeneratorImage"
	TrafficGeneratorNodeLabelSelectorParamName = "trafficGeneratorNodeLabelSelector"
	TrafficGeneratorPacketsPerSecondParamName  = "trafficGeneratorPacketsPerSecond"
	TrafficGeneratorEastMacAddressParamName    = "trafficGeneratorEastMacAddress"
	TrafficGeneratorWestMacAddressParamName    = "trafficGeneratorWestMacAddress"
	VMContainerDiskImageParamName              = "vmContainerDiskImage"
	DPDKNodeLabelSelectorParamName             = "DPDKNodeLabelSelector"
	PortBandwidthGBParamName                   = "portBandwidthGB"
	DPDKEastMacAddressParamName                = "DPDKEastMacAddress"
	DPDKWestMacAddressParamName                = "DPDKWestMacAddress"
	TestDurationParamName                      = "testDuration"
	VerboseParamName                           = "verbose"
)

const (
	TrafficGeneratorImageDefault            = "quay.io/kiagnose/kubevirt-dpdk-checkup-traffic-gen:main"
	TrafficGeneratorPacketsPerSecondDefault = "14m"
	VMContainerDiskImageDefault             = "quay.io/kiagnose/kubevirt-dpdk-checkup-vm:main"
	PortBandwidthGBDefault                  = 10
	TestDurationDefault                     = 5 * time.Minute
	VerboseDefault                          = false

	TrafficGeneratorMacAddressPrefixOctet = 0x50
	DPDKMacAddressPrefixOctet             = 0x60
	EastMacAddressSuffixOctet             = 0x01
	WestMacAddressSuffixOctet             = 0x02
)

const (
	VMIUsername = "cloud-user"
	VMIPassword = "0tli-pxem-xknu" // #nosec

	VMIEastNICPCIAddress = "0000:06:00.0"
	VMIWestNICPCIAddress = "0000:07:00.0"
)

var (
	ErrInvalidNetworkAttachmentDefinitionName = errors.New("invalid Network-Attachment-Definition Name")
	ErrIllegalLabelSelectorCombination        = errors.New("illegal Traffic Generator and DPDK Node " +
		"Label Selector combination")
	ErrInvalidTrafficGeneratorPacketsPerSecond = errors.New("invalid Traffic Generator Packets Per Second")
	ErrInvalidTrafficGeneratorEastMacAddress   = errors.New("invalid Traffic Generator East MAC Address")
	ErrInvalidTrafficGeneratorWestMacAddress   = errors.New("invalid Traffic Generator West MAC Address")
	ErrInvalidPortBandwidthGB                  = errors.New("invalid Port Bandwidth [GB]")
	ErrInvalidDPDKEastMacAddress               = errors.New("invalid DPDK East MAC Address")
	ErrInvalidDPDKWestMacAddress               = errors.New("invalid DPDK West MAC Address")
	ErrInvalidTestDuration                     = errors.New("invalid Test Duration")
	ErrInvalidVerbose                          = errors.New("invalid Verbose value [true|false]")
)

type Config struct {
	PodName                           string
	PodUID                            string
	NetworkAttachmentDefinitionName   string
	TrafficGeneratorImage             string
	TrafficGeneratorNodeLabelSelector string
	TrafficGeneratorPacketsPerSecond  string
	TrafficGeneratorEastMacAddress    net.HardwareAddr
	TrafficGeneratorWestMacAddress    net.HardwareAddr
	VMContainerDiskImage              string
	DPDKNodeLabelSelector             string
	PortBandwidthGB                   int
	DPDKEastMacAddress                net.HardwareAddr
	DPDKWestMacAddress                net.HardwareAddr
	TestDuration                      time.Duration
	Verbose                           bool
}

func New(baseConfig kconfig.Config) (Config, error) {
	trafficGeneratorEastMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(
		TrafficGeneratorMacAddressPrefixOctet, EastMacAddressSuffixOctet)
	trafficGeneratorWestMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(
		TrafficGeneratorMacAddressPrefixOctet, WestMacAddressSuffixOctet)
	dpdkEastMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(DPDKMacAddressPrefixOctet, EastMacAddressSuffixOctet)
	dpdkWestMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(DPDKMacAddressPrefixOctet, WestMacAddressSuffixOctet)
	newConfig := Config{
		PodName:                           baseConfig.PodName,
		PodUID:                            baseConfig.PodUID,
		NetworkAttachmentDefinitionName:   baseConfig.Params[NetworkAttachmentDefinitionNameParamName],
		TrafficGeneratorImage:             TrafficGeneratorImageDefault,
		TrafficGeneratorNodeLabelSelector: baseConfig.Params[TrafficGeneratorNodeLabelSelectorParamName],
		TrafficGeneratorPacketsPerSecond:  TrafficGeneratorPacketsPerSecondDefault,
		TrafficGeneratorEastMacAddress:    trafficGeneratorEastMacAddressDefault,
		TrafficGeneratorWestMacAddress:    trafficGeneratorWestMacAddressDefault,
		VMContainerDiskImage:              VMContainerDiskImageDefault,
		DPDKNodeLabelSelector:             baseConfig.Params[DPDKNodeLabelSelectorParamName],
		PortBandwidthGB:                   PortBandwidthGBDefault,
		DPDKEastMacAddress:                dpdkEastMacAddressDefault,
		DPDKWestMacAddress:                dpdkWestMacAddressDefault,
		TestDuration:                      TestDurationDefault,
		Verbose:                           VerboseDefault,
	}

	if newConfig.NetworkAttachmentDefinitionName == "" {
		return Config{}, ErrInvalidNetworkAttachmentDefinitionName
	}

	if newConfig.TrafficGeneratorNodeLabelSelector == "" && newConfig.DPDKNodeLabelSelector != "" ||
		newConfig.TrafficGeneratorNodeLabelSelector != "" && newConfig.DPDKNodeLabelSelector == "" {
		return Config{}, ErrIllegalLabelSelectorCombination
	}

	return setOptionalParams(baseConfig, newConfig)
}

func setOptionalParams(baseConfig kconfig.Config, newConfig Config) (Config, error) {
	var err error

	if rawVal := baseConfig.Params[TrafficGeneratorImageParamName]; rawVal != "" {
		newConfig.TrafficGeneratorImage = rawVal
	}

	if rawVal := baseConfig.Params[TrafficGeneratorPacketsPerSecondParamName]; rawVal != "" {
		newConfig.TrafficGeneratorPacketsPerSecond, err = parseTrafficGeneratorPacketsPerSecond(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGeneratorPacketsPerSecond
		}
	}

	if rawVal := baseConfig.Params[VMContainerDiskImageParamName]; rawVal != "" {
		newConfig.VMContainerDiskImage = rawVal
	}

	if rawVal := baseConfig.Params[VerboseParamName]; rawVal != "" {
		newConfig.Verbose, err = strconv.ParseBool(rawVal)
		if err != nil {
			return Config{}, ErrInvalidVerbose
		}
	}

	if rawVal := baseConfig.Params[PortBandwidthGBParamName]; rawVal != "" {
		newConfig.PortBandwidthGB, err = parseNonZeroPositiveInt(rawVal)
		if err != nil {
			return Config{}, ErrInvalidPortBandwidthGB
		}
	}

	newConfig, err = setMacAddressParams(baseConfig, newConfig)
	if err != nil {
		return Config{}, err
	}

	if rawVal := baseConfig.Params[TestDurationParamName]; rawVal != "" {
		newConfig.TestDuration, err = time.ParseDuration(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTestDuration
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

func parseTrafficGeneratorPacketsPerSecond(rawVal string) (string, error) {
	validFormat := regexp.MustCompile(`^[1-9]\d*([km])?$`)
	if !validFormat.MatchString(rawVal) {
		return "", errors.New("parameter has invalid format")
	}
	return rawVal, nil
}

func parseNonZeroPositiveInt(rawVal string) (int, error) {
	val, err := strconv.Atoi(rawVal)
	if err != nil || val <= 0 {
		return 0, errors.New("parameter is zero or negative")
	}
	return val, nil
}

func generateMacAddressWithPresetPrefixAndSuffix(prefixOctet, suffixOctet byte) net.HardwareAddr {
	const (
		MACOctetsCount = 6
		prefixOctetIdx = 0
		suffixOctetIdx = 5
	)
	address := make([]byte, MACOctetsCount)
	_, _ = rand.Read(address)
	address[prefixOctetIdx] = prefixOctet
	address[suffixOctetIdx] = suffixOctet
	return address
}
