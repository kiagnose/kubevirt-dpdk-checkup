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

package config_test

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"

	kconfig "github.com/kiagnose/kiagnose/kiagnose/config"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
)

const (
	testPodName                       = "my-pod"
	testPodUID                        = "0123456789-0123456789"
	networkAttachmentDefinitionName   = "intel-dpdk-network1"
	trafficGeneratorRuntimeClassName  = "dpdk-runtimeclass"
	portBandwidthGB                   = 100
	trafficGeneratorNodeLabelSelector = "node-role.kubernetes.io/worker-dpdk1"
	trafficGeneratorPacketsPerSecond  = "6m"
	dpdkNodeLabelSelector             = "node-role.kubernetes.io/worker-dpdk2"
	trafficGeneratorEastMacAddress    = "DE:AD:BE:EF:00:01"
	trafficGeneratorWestMacAddress    = "DE:AD:BE:EF:01:00"
	dpdkEastMacAddress                = "DE:AD:BE:EF:00:02"
	dpdkWestMacAddress                = "DE:AD:BE:EF:02:00"
	trafficGeneratorImage             = "quay.io/ramlavi/kubevirt-dpdk-checkup-traffic-gen:main"
	vmContainerDiskImage              = "quay.io/ramlavi/kubevirt-dpdk-checkup-vm:main"
	testDuration                      = "30m"
)

func TestNewShouldApplyDefaultsWhenOptionalFieldsAreMissing(t *testing.T) {
	baseConfig := kconfig.Config{
		PodName: testPodName,
		PodUID:  testPodUID,
		Params: map[string]string{
			config.NetworkAttachmentDefinitionNameParamName:  networkAttachmentDefinitionName,
			config.TrafficGeneratorRuntimeClassNameParamName: trafficGeneratorRuntimeClassName,
		},
	}

	actualConfig, err := config.New(baseConfig)
	assert.NoError(t, err)

	assert.NotNil(t, actualConfig.TrafficGeneratorEastMacAddress)
	assert.NotNil(t, actualConfig.TrafficGeneratorWestMacAddress)
	assert.NotNil(t, actualConfig.DPDKEastMacAddress)
	assert.NotNil(t, actualConfig.DPDKWestMacAddress)

	expectedConfig := config.Config{
		PodName:                          testPodName,
		PodUID:                           testPodUID,
		TrafficGeneratorRuntimeClassName: trafficGeneratorRuntimeClassName,
		NetworkAttachmentDefinitionName:  networkAttachmentDefinitionName,
		TrafficGeneratorPacketsPerSecond: config.TrafficGeneratorPacketsPerSecondDefault,
		PortBandwidthGB:                  config.PortBandwidthGBDefault,
		TrafficGeneratorEastMacAddress:   actualConfig.TrafficGeneratorEastMacAddress,
		TrafficGeneratorWestMacAddress:   actualConfig.TrafficGeneratorWestMacAddress,
		DPDKEastMacAddress:               actualConfig.DPDKEastMacAddress,
		DPDKWestMacAddress:               actualConfig.DPDKWestMacAddress,
		TrafficGeneratorImage:            config.TrafficGeneratorImageDefault,
		VMContainerDiskImage:             config.VMContainerDiskImageDefault,
		TestDuration:                     config.TestDurationDefault,
		Verbose:                          config.VerboseDefault,
	}
	assert.Equal(t, expectedConfig, actualConfig)
}

func TestNewShouldApplyUserConfig(t *testing.T) {
	baseConfig := kconfig.Config{
		PodName: testPodName,
		PodUID:  testPodUID,
		Params:  getValidUserParameters(),
	}

	actualConfig, err := config.New(baseConfig)
	assert.NoError(t, err)

	trafficGeneratorEastHWAddress, _ := net.ParseMAC(trafficGeneratorEastMacAddress)
	trafficGeneratorWestHWAddress, _ := net.ParseMAC(trafficGeneratorWestMacAddress)
	dpdkEastHWAddress, _ := net.ParseMAC(dpdkEastMacAddress)
	dpdkWestHWAddress, _ := net.ParseMAC(dpdkWestMacAddress)
	expectedConfig := config.Config{
		PodName:                           testPodName,
		PodUID:                            testPodUID,
		TrafficGeneratorRuntimeClassName:  trafficGeneratorRuntimeClassName,
		PortBandwidthGB:                   portBandwidthGB,
		NetworkAttachmentDefinitionName:   networkAttachmentDefinitionName,
		TrafficGeneratorPacketsPerSecond:  trafficGeneratorPacketsPerSecond,
		TrafficGeneratorNodeLabelSelector: trafficGeneratorNodeLabelSelector,
		DPDKNodeLabelSelector:             dpdkNodeLabelSelector,
		TrafficGeneratorEastMacAddress:    trafficGeneratorEastHWAddress,
		TrafficGeneratorWestMacAddress:    trafficGeneratorWestHWAddress,
		DPDKEastMacAddress:                dpdkEastHWAddress,
		DPDKWestMacAddress:                dpdkWestHWAddress,
		TrafficGeneratorImage:             trafficGeneratorImage,
		VMContainerDiskImage:              vmContainerDiskImage,
		TestDuration:                      30 * time.Minute,
		Verbose:                           true,
	}
	assert.Equal(t, expectedConfig, actualConfig)
}

func TestNewShouldFailWhen(t *testing.T) {
	type failureTestCase struct {
		description    string
		key            string
		faultyKeyValue string
		expectedError  error
	}

	testCases := []failureTestCase{
		{
			description:    "Traffic Generator Runtimeclass Name is invalid",
			key:            config.TrafficGeneratorRuntimeClassNameParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrInvalidTrafficGeneratorRuntimeClassName,
		},
		{
			description:    "NetworkAttachmentDefinitionName is invalid",
			key:            config.NetworkAttachmentDefinitionNameParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrInvalidNetworkAttachmentDefinitionName,
		},
		{
			description:    "TrafficGeneratorPacketsPerSecond is invalid",
			key:            config.TrafficGeneratorPacketsPerSecondParamName,
			faultyKeyValue: "-14",
			expectedError:  config.ErrInvalidTrafficGeneratorPacketsPerSecond,
		},
		{
			description:    "TrafficGeneratorPacketsPerSecond is invalid",
			key:            config.TrafficGeneratorPacketsPerSecondParamName,
			faultyKeyValue: "15f",
			expectedError:  config.ErrInvalidTrafficGeneratorPacketsPerSecond,
		},
		{
			description:    "PortBandwidthGB is invalid",
			key:            config.PortBandwidthGBParamName,
			faultyKeyValue: "0",
			expectedError:  config.ErrInvalidPortBandwidthGB,
		},
		{
			description:    "TrafficGeneratorEastMacAddress is invalid",
			key:            config.TrafficGeneratorEastMacAddressParamName,
			faultyKeyValue: "AB:CD:EF:GH:IJ:KH",
			expectedError:  config.ErrInvalidTrafficGeneratorEastMacAddress,
		},
		{
			description:    "TrafficGeneratorWestMacAddress is invalid",
			key:            config.TrafficGeneratorWestMacAddressParamName,
			faultyKeyValue: "AB:CD:EF:GH:IJ:KH",
			expectedError:  config.ErrInvalidTrafficGeneratorWestMacAddress,
		},
		{
			description:    "DPDKEastMacAddress is invalid",
			key:            config.DPDKEastMacAddressParamName,
			faultyKeyValue: "AB:CD:EF:GH:IJ:KH",
			expectedError:  config.ErrInvalidDPDKEastMacAddress,
		},
		{
			description:    "DPDKWestMacAddress is invalid",
			key:            config.DPDKWestMacAddressParamName,
			faultyKeyValue: "AB:CD:EF:GH:IJ:KH",
			expectedError:  config.ErrInvalidDPDKWestMacAddress,
		},
		{
			description:    "TestDuration is invalid",
			key:            config.TestDurationParamName,
			faultyKeyValue: "invalid value",
			expectedError:  config.ErrInvalidTestDuration,
		},
		{
			description:    "Verbose is invalid",
			key:            config.VerboseParamName,
			faultyKeyValue: "maybe",
			expectedError:  config.ErrInvalidVerbose,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			faultyUserParams := getValidUserParameters()
			faultyUserParams[testCase.key] = testCase.faultyKeyValue

			baseConfig := kconfig.Config{
				PodName: testPodName,
				PodUID:  testPodUID,
				Params:  faultyUserParams,
			}

			_, err := config.New(baseConfig)
			assert.ErrorIs(t, err, testCase.expectedError)
		})
	}
}

func getValidUserParameters() map[string]string {
	return map[string]string{
		config.TrafficGeneratorRuntimeClassNameParamName:  trafficGeneratorRuntimeClassName,
		config.NetworkAttachmentDefinitionNameParamName:   networkAttachmentDefinitionName,
		config.PortBandwidthGBParamName:                   fmt.Sprintf("%d", portBandwidthGB),
		config.TrafficGeneratorNodeLabelSelectorParamName: trafficGeneratorNodeLabelSelector,
		config.TrafficGeneratorPacketsPerSecondParamName:  trafficGeneratorPacketsPerSecond,
		config.DPDKNodeLabelSelectorParamName:             dpdkNodeLabelSelector,
		config.TrafficGeneratorEastMacAddressParamName:    trafficGeneratorEastMacAddress,
		config.TrafficGeneratorWestMacAddressParamName:    trafficGeneratorWestMacAddress,
		config.DPDKEastMacAddressParamName:                dpdkEastMacAddress,
		config.DPDKWestMacAddressParamName:                dpdkWestMacAddress,
		config.TrafficGeneratorImageParamName:             trafficGeneratorImage,
		config.VMContainerDiskImageParamName:              vmContainerDiskImage,
		config.TestDurationParamName:                      testDuration,
		config.VerboseParamName:                           strconv.FormatBool(true),
	}
}
