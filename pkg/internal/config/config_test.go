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
	testTrafficGenContainerDiskImage  = "quay.io/ramlavi/kubevirt-dpdk-checkup-traffic-gen:main"
	testTrafficGenTargetNodeName      = "worker-dpdk1"
	testTrafficGenPacketsPerSecond    = "6m"
	testVMUnderTestContainerDiskImage = "quay.io/ramlavi/kubevirt-dpdk-checkup-vm:main"
	testVMUnderTestTargetNodeName     = "worker-dpdk2"
	testDuration                      = "30m"
	testPortBandwidthGbps             = 100
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

	assert.NotNil(t, actualConfig.TrafficGenEastMacAddress)
	assert.NotNil(t, actualConfig.TrafficGenWestMacAddress)
	assert.NotNil(t, actualConfig.VMUnderTestEastMacAddress)
	assert.NotNil(t, actualConfig.VMUnderTestWestMacAddress)

	expectedConfig := config.Config{
		PodName:                         testPodName,
		PodUID:                          testPodUID,
		NetworkAttachmentDefinitionName: networkAttachmentDefinitionName,
		TrafficGenContainerDiskImage:    config.TrafficGenDefaultContainerDiskImage,
		TrafficGenPacketsPerSecond:      config.TrafficGenDefaultPacketsPerSecond,
		TrafficGenEastMacAddress:        actualConfig.TrafficGenEastMacAddress,
		TrafficGenWestMacAddress:        actualConfig.TrafficGenWestMacAddress,
		VMUnderTestContainerDiskImage:   config.VMUnderTestDefaultContainerDiskImage,
		VMUnderTestEastMacAddress:       actualConfig.VMUnderTestEastMacAddress,
		VMUnderTestWestMacAddress:       actualConfig.VMUnderTestWestMacAddress,
		TestDuration:                    config.TestDurationDefault,
		PortBandwidthGbps:               config.PortBandwidthGbpsDefault,
		Verbose:                         config.VerboseDefault,
	}
	assert.Equal(t, expectedConfig, actualConfig)
}

type SuccessTestCase struct {
	description    string
	params         map[string]string
	expectedConfig config.Config
}

func TestNewShouldApplyUserConfigWhen(t *testing.T) {
	testCases := []SuccessTestCase{
		{
			"config is valid and both Node Selectors are set",
			getValidUserParametersWithNodeSelectors(),
			config.Config{
				PodName:                         testPodName,
				PodUID:                          testPodUID,
				NetworkAttachmentDefinitionName: networkAttachmentDefinitionName,
				TrafficGenContainerDiskImage:    testTrafficGenContainerDiskImage,
				TrafficGenTargetNodeName:        testTrafficGenTargetNodeName,
				TrafficGenPacketsPerSecond:      testTrafficGenPacketsPerSecond,
				VMUnderTestContainerDiskImage:   testVMUnderTestContainerDiskImage,
				VMUnderTestTargetNodeName:       testVMUnderTestTargetNodeName,
				TestDuration:                    30 * time.Minute,
				PortBandwidthGbps:               testPortBandwidthGbps,
				Verbose:                         true,
			},
		},
		{
			"config is valid and both Node Selectors are not set",
			getValidUserParametersWithOutNodeSelectors(),
			config.Config{
				PodName:                         testPodName,
				PodUID:                          testPodUID,
				NetworkAttachmentDefinitionName: networkAttachmentDefinitionName,
				TrafficGenContainerDiskImage:    testTrafficGenContainerDiskImage,
				TrafficGenPacketsPerSecond:      testTrafficGenPacketsPerSecond,
				VMUnderTestContainerDiskImage:   testVMUnderTestContainerDiskImage,
				TestDuration:                    30 * time.Minute,
				PortBandwidthGbps:               testPortBandwidthGbps,
				Verbose:                         true,
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
			assert.NotNil(t, actualConfig.TrafficGenEastMacAddress)
			assert.NotNil(t, actualConfig.TrafficGenWestMacAddress)
			assert.NotNil(t, actualConfig.VMUnderTestEastMacAddress)
			assert.NotNil(t, actualConfig.VMUnderTestWestMacAddress)

			testCase.expectedConfig.TrafficGenEastMacAddress = actualConfig.TrafficGenEastMacAddress
			testCase.expectedConfig.TrafficGenWestMacAddress = actualConfig.TrafficGenWestMacAddress
			testCase.expectedConfig.VMUnderTestEastMacAddress = actualConfig.VMUnderTestEastMacAddress
			testCase.expectedConfig.VMUnderTestWestMacAddress = actualConfig.VMUnderTestWestMacAddress

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
			description:    "trafficGenTargetNodeName is missing and vmUnderTestTargetNodeName is set",
			key:            config.TrafficGenTargetNodeNameParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrIllegalLabelSelectorCombination,
		},
		{
			description:    "vmUnderTestTargetNodeName is missing and trafficGenTargetNodeName is set",
			key:            config.VMUnderTestTargetNodeNameParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrIllegalLabelSelectorCombination,
		},
		{
			description:    "TrafficGenPacketsPerSecond is invalid",
			key:            config.TrafficGenPacketsPerSecondParamName,
			faultyKeyValue: "-14",
			expectedError:  config.ErrInvalidTrafficGenPacketsPerSecond,
		},
		{
			description:    "TrafficGenPacketsPerSecond is invalid",
			key:            config.TrafficGenPacketsPerSecondParamName,
			faultyKeyValue: "15f",
			expectedError:  config.ErrInvalidTrafficGenPacketsPerSecond,
		},
		{
			description:    "TestDuration is invalid",
			key:            config.TestDurationParamName,
			faultyKeyValue: "invalid value",
			expectedError:  config.ErrInvalidTestDuration,
		},
		{
			description:    "PortBandwidthGbps is invalid",
			key:            config.PortBandwidthGbpsParamName,
			faultyKeyValue: "0",
			expectedError:  config.ErrInvalidPortBandwidthGbps,
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
	delete(paramsWithOutNodeSelectors, config.TrafficGenTargetNodeNameParamName)
	delete(paramsWithOutNodeSelectors, config.VMUnderTestTargetNodeNameParamName)
	return paramsWithOutNodeSelectors
}

func getValidUserParameters() map[string]string {
	return map[string]string{
		config.NetworkAttachmentDefinitionNameParamName: networkAttachmentDefinitionName,
		config.TrafficGenContainerDiskImageParamName:    testTrafficGenContainerDiskImage,
		config.TrafficGenTargetNodeNameParamName:        testTrafficGenTargetNodeName,
		config.TrafficGenPacketsPerSecondParamName:      testTrafficGenPacketsPerSecond,
		config.VMUnderTestContainerDiskImageParamName:   testVMUnderTestContainerDiskImage,
		config.VMUnderTestTargetNodeNameParamName:       testVMUnderTestTargetNodeName,
		config.TestDurationParamName:                    testDuration,
		config.PortBandwidthGbpsParamName:               fmt.Sprintf("%d", testPortBandwidthGbps),
		config.VerboseParamName:                         strconv.FormatBool(true),
	}
}
