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
	NetworkAttachmentDefinitionNameParamName = "networkAttachmentDefinitionName"
	TrafficGenContainerDiskImageParamName    = "trafficGenContainerDiskImage"
	TrafficGenTargetNodeNameParamName        = "trafficGenTargetNodeName"
	TrafficGenPacketsPerSecondParamName      = "trafficGenPacketsPerSecond"
	VMContainerDiskImageParamName            = "vmContainerDiskImage"
	DPDKNodeLabelSelectorParamName           = "DPDKNodeLabelSelector"
	TestDurationParamName                    = "testDuration"
	PortBandwidthGBParamName                 = "portBandwidthGB"
	VerboseParamName                         = "verbose"
)

const (
	TrafficGenDefaultContainerDiskImage = "quay.io/kiagnose/kubevirt-dpdk-checkup-traffic-gen:main"
	TrafficGenDefaultPacketsPerSecond   = "14m"
	VMContainerDiskImageDefault         = "quay.io/kiagnose/kubevirt-dpdk-checkup-vm:main"
	TestDurationDefault                 = 5 * time.Minute
	PortBandwidthGBDefault              = 10
	VerboseDefault                      = false

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
	ErrInvalidTrafficGenPacketsPerSecond = errors.New("invalid Traffic Generator Packets Per Second")
	ErrInvalidTestDuration               = errors.New("invalid Test Duration")
	ErrInvalidPortBandwidthGB            = errors.New("invalid Port Bandwidth [GB]")
	ErrInvalidVerbose                    = errors.New("invalid Verbose value [true|false]")
)

type Config struct {
	PodName                         string
	PodUID                          string
	NetworkAttachmentDefinitionName string
	TrafficGenContainerDiskImage    string
	TrafficGenTargetNodeName        string
	TrafficGenPacketsPerSecond      string
	TrafficGeneratorEastMacAddress  net.HardwareAddr
	TrafficGeneratorWestMacAddress  net.HardwareAddr
	VMContainerDiskImage            string
	DPDKNodeLabelSelector           string
	DPDKEastMacAddress              net.HardwareAddr
	DPDKWestMacAddress              net.HardwareAddr
	TestDuration                    time.Duration
	PortBandwidthGB                 int
	Verbose                         bool
}

func New(baseConfig kconfig.Config) (Config, error) {
	trafficGeneratorEastMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(
		TrafficGeneratorMacAddressPrefixOctet, EastMacAddressSuffixOctet)
	trafficGeneratorWestMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(
		TrafficGeneratorMacAddressPrefixOctet, WestMacAddressSuffixOctet)
	dpdkEastMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(DPDKMacAddressPrefixOctet, EastMacAddressSuffixOctet)
	dpdkWestMacAddressDefault := generateMacAddressWithPresetPrefixAndSuffix(DPDKMacAddressPrefixOctet, WestMacAddressSuffixOctet)
	newConfig := Config{
		PodName:                         baseConfig.PodName,
		PodUID:                          baseConfig.PodUID,
		NetworkAttachmentDefinitionName: baseConfig.Params[NetworkAttachmentDefinitionNameParamName],
		TrafficGenContainerDiskImage:    TrafficGenDefaultContainerDiskImage,
		TrafficGenTargetNodeName:        baseConfig.Params[TrafficGenTargetNodeNameParamName],
		TrafficGenPacketsPerSecond:      TrafficGenDefaultPacketsPerSecond,
		TrafficGeneratorEastMacAddress:  trafficGeneratorEastMacAddressDefault,
		TrafficGeneratorWestMacAddress:  trafficGeneratorWestMacAddressDefault,
		VMContainerDiskImage:            VMContainerDiskImageDefault,
		DPDKNodeLabelSelector:           baseConfig.Params[DPDKNodeLabelSelectorParamName],
		DPDKEastMacAddress:              dpdkEastMacAddressDefault,
		DPDKWestMacAddress:              dpdkWestMacAddressDefault,
		TestDuration:                    TestDurationDefault,
		PortBandwidthGB:                 PortBandwidthGBDefault,
		Verbose:                         VerboseDefault,
	}

	if newConfig.NetworkAttachmentDefinitionName == "" {
		return Config{}, ErrInvalidNetworkAttachmentDefinitionName
	}

	if newConfig.TrafficGenTargetNodeName == "" && newConfig.DPDKNodeLabelSelector != "" ||
		newConfig.TrafficGenTargetNodeName != "" && newConfig.DPDKNodeLabelSelector == "" {
		return Config{}, ErrIllegalLabelSelectorCombination
	}

	return setOptionalParams(baseConfig, newConfig)
}

func setOptionalParams(baseConfig kconfig.Config, newConfig Config) (Config, error) {
	var err error

	if rawVal := baseConfig.Params[TrafficGenContainerDiskImageParamName]; rawVal != "" {
		newConfig.TrafficGenContainerDiskImage = rawVal
	}

	if rawVal := baseConfig.Params[TrafficGenPacketsPerSecondParamName]; rawVal != "" {
		newConfig.TrafficGenPacketsPerSecond, err = parseTrafficGenPacketsPerSecond(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGenPacketsPerSecond
		}
	}

	if rawVal := baseConfig.Params[VMContainerDiskImageParamName]; rawVal != "" {
		newConfig.VMContainerDiskImage = rawVal
	}

	if rawVal := baseConfig.Params[TestDurationParamName]; rawVal != "" {
		newConfig.TestDuration, err = time.ParseDuration(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTestDuration
		}
	}

	if rawVal := baseConfig.Params[PortBandwidthGBParamName]; rawVal != "" {
		newConfig.PortBandwidthGB, err = parseNonZeroPositiveInt(rawVal)
		if err != nil {
			return Config{}, ErrInvalidPortBandwidthGB
		}
	}

	if rawVal := baseConfig.Params[VerboseParamName]; rawVal != "" {
		newConfig.Verbose, err = strconv.ParseBool(rawVal)
		if err != nil {
			return Config{}, ErrInvalidVerbose
		}
	}

	return newConfig, nil
}

func parseTrafficGenPacketsPerSecond(rawVal string) (string, error) {
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
