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
	NetworkAttachmentDefinitionNameParamName         = "networkAttachmentDefinitionName"
	TrafficGenContainerDiskImageParamName            = "trafficGenContainerDiskImage"
	TrafficGenAlwaysPullContainerDiskImageParamName  = "trafficGenAlwaysPullContainerDiskImage"
	TrafficGenTargetNodeNameParamName                = "trafficGenTargetNodeName"
	TrafficGenPacketsPerSecondParamName              = "trafficGenPacketsPerSecond"
	VMUnderTestContainerDiskImageParamName           = "vmUnderTestContainerDiskImage"
	VMUnderTestAlwaysPullContainerDiskImageParamName = "vmUnderTestAlwaysPullContainerDiskImage"
	VMUnderTestTargetNodeNameParamName               = "vmUnderTestTargetNodeName"
	TestDurationParamName                            = "testDuration"
	PortBandwidthGbpsParamName                       = "portBandwidthGbps"
	VerboseParamName                                 = "verbose"
)

const (
	TrafficGenDefaultContainerDiskImage            = "quay.io/kiagnose/kubevirt-dpdk-checkup-traffic-gen:main"
	TrafficGenAlwaysPullContainerDiskImageDefault  = false
	TrafficGenDefaultPacketsPerSecond              = "8m"
	VMUnderTestDefaultContainerDiskImage           = "quay.io/kiagnose/kubevirt-dpdk-checkup-vm:main"
	VMUnderTestAlwaysPullContainerDiskImageDefault = false
	TestDurationDefault                            = 5 * time.Minute
	PortBandwidthGbpsDefault                       = 10
	VerboseDefault                                 = false

	TrafficGenMACAddressPrefixOctet  = 0x50
	VMUnderTestMACAddressPrefixOctet = 0x60
	EastMACAddressSuffixOctet        = 0x01
	WestMACAddressSuffixOctet        = 0x02
)

const (
	VMIUsername = "cloud-user"
	VMIPassword = "0tli-pxem-xknu" // #nosec

	VMIEastNICPCIAddress = "0000:06:00.0"
	VMIWestNICPCIAddress = "0000:07:00.0"
)

var (
	ErrInvalidNetworkAttachmentDefinitionName         = errors.New("invalid Network-Attachment-Definition Name")
	ErrInvalidTrafficGenAlwaysPullContainerDiskImage  = errors.New("invalid Traffic generator always pull container disk image value")
	ErrIllegalTargetNodeNamesCombination              = errors.New("illegal Traffic Generator and VM under test target node names combination")
	ErrInvalidTrafficGenPacketsPerSecond              = errors.New("invalid Traffic Generator Packets Per Second")
	ErrInvalidVMUnderTestAlwaysPullContainerDiskImage = errors.New("invalid VM under test always pull container disk image value")
	ErrInvalidTestDuration                            = errors.New("invalid Test Duration")
	ErrInvalidPortBandwidthGbps                       = errors.New("invalid Port Bandwidth [Gbps]")
	ErrInvalidVerbose                                 = errors.New("invalid Verbose value [true|false]")
)

type Config struct {
	PodName                                 string
	PodUID                                  string
	NetworkAttachmentDefinitionName         string
	TrafficGenContainerDiskImage            string
	TrafficGenAlwaysPullContainerDiskImage  bool
	TrafficGenTargetNodeName                string
	TrafficGenPacketsPerSecond              string
	TrafficGenEastMacAddress                net.HardwareAddr
	TrafficGenWestMacAddress                net.HardwareAddr
	VMUnderTestContainerDiskImage           string
	VMUnderTestAlwaysPullContainerDiskImage bool
	VMUnderTestTargetNodeName               string
	VMUnderTestEastMacAddress               net.HardwareAddr
	VMUnderTestWestMacAddress               net.HardwareAddr
	TestDuration                            time.Duration
	PortBandwidthGbps                       int
	Verbose                                 bool
}

func New(baseConfig kconfig.Config) (Config, error) {
	trafficGenEastMacAddress := generateMacAddressWithPresetPrefixAndSuffix(
		TrafficGenMACAddressPrefixOctet,
		EastMACAddressSuffixOctet,
	)

	trafficGenWestMacAddress := generateMacAddressWithPresetPrefixAndSuffix(
		TrafficGenMACAddressPrefixOctet,
		WestMACAddressSuffixOctet,
	)

	vmUnderTestEastMACAddress := generateMacAddressWithPresetPrefixAndSuffix(
		VMUnderTestMACAddressPrefixOctet,
		EastMACAddressSuffixOctet,
	)

	vmUnderTestWestMacAddress := generateMacAddressWithPresetPrefixAndSuffix(
		VMUnderTestMACAddressPrefixOctet,
		WestMACAddressSuffixOctet,
	)

	newConfig := Config{
		PodName:                                 baseConfig.PodName,
		PodUID:                                  baseConfig.PodUID,
		NetworkAttachmentDefinitionName:         baseConfig.Params[NetworkAttachmentDefinitionNameParamName],
		TrafficGenContainerDiskImage:            TrafficGenDefaultContainerDiskImage,
		TrafficGenAlwaysPullContainerDiskImage:  TrafficGenAlwaysPullContainerDiskImageDefault,
		TrafficGenTargetNodeName:                baseConfig.Params[TrafficGenTargetNodeNameParamName],
		TrafficGenPacketsPerSecond:              TrafficGenDefaultPacketsPerSecond,
		TrafficGenEastMacAddress:                trafficGenEastMacAddress,
		TrafficGenWestMacAddress:                trafficGenWestMacAddress,
		VMUnderTestContainerDiskImage:           VMUnderTestDefaultContainerDiskImage,
		VMUnderTestAlwaysPullContainerDiskImage: VMUnderTestAlwaysPullContainerDiskImageDefault,
		VMUnderTestTargetNodeName:               baseConfig.Params[VMUnderTestTargetNodeNameParamName],
		VMUnderTestEastMacAddress:               vmUnderTestEastMACAddress,
		VMUnderTestWestMacAddress:               vmUnderTestWestMacAddress,
		TestDuration:                            TestDurationDefault,
		PortBandwidthGbps:                       PortBandwidthGbpsDefault,
		Verbose:                                 VerboseDefault,
	}

	if newConfig.NetworkAttachmentDefinitionName == "" {
		return Config{}, ErrInvalidNetworkAttachmentDefinitionName
	}

	if newConfig.TrafficGenTargetNodeName == "" && newConfig.VMUnderTestTargetNodeName != "" ||
		newConfig.TrafficGenTargetNodeName != "" && newConfig.VMUnderTestTargetNodeName == "" {
		return Config{}, ErrIllegalTargetNodeNamesCombination
	}

	return setOptionalParams(baseConfig, newConfig)
}

func setOptionalParams(baseConfig kconfig.Config, newConfig Config) (Config, error) {
	var err error

	if rawVal := baseConfig.Params[TrafficGenContainerDiskImageParamName]; rawVal != "" {
		newConfig.TrafficGenContainerDiskImage = rawVal
	}

	if rawVal := baseConfig.Params[TrafficGenAlwaysPullContainerDiskImageParamName]; rawVal != "" {
		newConfig.TrafficGenAlwaysPullContainerDiskImage, err = strconv.ParseBool(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGenAlwaysPullContainerDiskImage
		}
	}

	if rawVal := baseConfig.Params[TrafficGenPacketsPerSecondParamName]; rawVal != "" {
		newConfig.TrafficGenPacketsPerSecond, err = parseTrafficGenPacketsPerSecond(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTrafficGenPacketsPerSecond
		}
	}

	if rawVal := baseConfig.Params[VMUnderTestContainerDiskImageParamName]; rawVal != "" {
		newConfig.VMUnderTestContainerDiskImage = rawVal
	}

	if rawVal := baseConfig.Params[VMUnderTestAlwaysPullContainerDiskImageParamName]; rawVal != "" {
		newConfig.VMUnderTestAlwaysPullContainerDiskImage, err = strconv.ParseBool(rawVal)
		if err != nil {
			return Config{}, ErrInvalidVMUnderTestAlwaysPullContainerDiskImage
		}
	}

	if rawVal := baseConfig.Params[TestDurationParamName]; rawVal != "" {
		newConfig.TestDuration, err = time.ParseDuration(rawVal)
		if err != nil {
			return Config{}, ErrInvalidTestDuration
		}
	}

	if rawVal := baseConfig.Params[PortBandwidthGbpsParamName]; rawVal != "" {
		newConfig.PortBandwidthGbps, err = parseNonZeroPositiveInt(rawVal)
		if err != nil {
			return Config{}, ErrInvalidPortBandwidthGbps
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
