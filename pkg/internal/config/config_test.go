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
			config.NetworkAttachmentDefinitionNameParamName: networkAttachmentDefinitionName,
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

type SuccessTestCase struct {
	description    string
	params         map[string]string
	expectedConfig config.Config
}

func TestNewShouldApplyUserConfigWhen(t *testing.T) {
	trafficGeneratorEastHWAddress, _ := net.ParseMAC(trafficGeneratorEastMacAddress)
	trafficGeneratorWestHWAddress, _ := net.ParseMAC(trafficGeneratorWestMacAddress)
	dpdkEastHWAddress, _ := net.ParseMAC(dpdkEastMacAddress)
	dpdkWestHWAddress, _ := net.ParseMAC(dpdkWestMacAddress)

	testCases := []SuccessTestCase{
		{
			"config is valid and both Node Selectors are set",
			getValidUserParametersWithNodeSelectors(),
			config.Config{
				PodName:                           testPodName,
				PodUID:                            testPodUID,
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
			},
		},
		{
			"config is valid and both Node Selectors are not set",
			getValidUserParametersWithOutNodeSelectors(),
			config.Config{
				PodName:                          testPodName,
				PodUID:                           testPodUID,
				PortBandwidthGB:                  portBandwidthGB,
				NetworkAttachmentDefinitionName:  networkAttachmentDefinitionName,
				TrafficGeneratorPacketsPerSecond: trafficGeneratorPacketsPerSecond,
				TrafficGeneratorEastMacAddress:   trafficGeneratorEastHWAddress,
				TrafficGeneratorWestMacAddress:   trafficGeneratorWestHWAddress,
				DPDKEastMacAddress:               dpdkEastHWAddress,
				DPDKWestMacAddress:               dpdkWestHWAddress,
				TrafficGeneratorImage:            trafficGeneratorImage,
				VMContainerDiskImage:             vmContainerDiskImage,
				TestDuration:                     30 * time.Minute,
				Verbose:                          true,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			baseConfig := kconfig.Config{
				PodName: testPodName,
				PodUID:  testPodUID,
				Params:  testCase.params,
			}

			actualConfig, err := config.New(baseConfig)
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedConfig, actualConfig)
		})
	}
}

type failureTestCase struct {
	description    string
	key            string
	faultyKeyValue string
	expectedError  error
}

func TestNewShouldFailWhen(t *testing.T) {
	testCases := []failureTestCase{
		{
			description:    "NetworkAttachmentDefinitionName is invalid",
			key:            config.NetworkAttachmentDefinitionNameParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrInvalidNetworkAttachmentDefinitionName,
		},
		{
			description:    "trafficGeneratorNodeLabelSelector is missing and DPDKNodeLabelSelector is set",
			key:            config.TrafficGeneratorNodeLabelSelectorParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrIllegalLabelSelectorCombination,
		},
		{
			description:    "DPDKNodeLabelSelector is missing and trafficGeneratorNodeLabelSelector is set",
			key:            config.DPDKNodeLabelSelectorParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrIllegalLabelSelectorCombination,
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
			runFailureTest(t, testCase)
		})
	}
}

func runFailureTest(t *testing.T, testCase failureTestCase) {
	faultyUserParams := getValidUserParameters()
	faultyUserParams[testCase.key] = testCase.faultyKeyValue

	baseConfig := kconfig.Config{
		PodName: testPodName,
		PodUID:  testPodUID,
		Params:  faultyUserParams,
	}

	_, err := config.New(baseConfig)
	assert.ErrorIs(t, err, testCase.expectedError)
}

func getValidUserParametersWithNodeSelectors() map[string]string {
	return getValidUserParameters()
}

func getValidUserParametersWithOutNodeSelectors() map[string]string {
	paramsWithOutNodeSelectors := getValidUserParameters()
	delete(paramsWithOutNodeSelectors, config.TrafficGeneratorNodeLabelSelectorParamName)
	delete(paramsWithOutNodeSelectors, config.DPDKNodeLabelSelectorParamName)
	return paramsWithOutNodeSelectors
}

func getValidUserParameters() map[string]string {
	return map[string]string{
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
