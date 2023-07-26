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
	testPodName                      = "my-pod"
	testPodUID                       = "0123456789-0123456789"
	networkAttachmentDefinitionName  = "intel-dpdk-network1"
	testTrafficGenContainerDiskImage = "quay.io/ramlavi/kubevirt-dpdk-checkup-traffic-gen:main"
	testTrafficGenTargetNodeName     = "worker-dpdk1"
	testTrafficGenPacketsPerSecond   = "6m"
	vmContainerDiskImage             = "quay.io/ramlavi/kubevirt-dpdk-checkup-vm:main"
	dpdkNodeLabelSelector            = "node-role.kubernetes.io/worker-dpdk2"
	testDuration                     = "30m"
	portBandwidthGB                  = 100
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
		PodName:                         testPodName,
		PodUID:                          testPodUID,
		NetworkAttachmentDefinitionName: networkAttachmentDefinitionName,
		TrafficGenContainerDiskImage:    config.TrafficGenDefaultContainerDiskImage,
		TrafficGenPacketsPerSecond:      config.TrafficGenDefaultPacketsPerSecond,
		TrafficGeneratorEastMacAddress:  actualConfig.TrafficGeneratorEastMacAddress,
		TrafficGeneratorWestMacAddress:  actualConfig.TrafficGeneratorWestMacAddress,
		VMContainerDiskImage:            config.VMContainerDiskImageDefault,
		DPDKEastMacAddress:              actualConfig.DPDKEastMacAddress,
		DPDKWestMacAddress:              actualConfig.DPDKWestMacAddress,
		TestDuration:                    config.TestDurationDefault,
		PortBandwidthGB:                 config.PortBandwidthGBDefault,
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
				VMContainerDiskImage:            vmContainerDiskImage,
				DPDKNodeLabelSelector:           dpdkNodeLabelSelector,
				TestDuration:                    30 * time.Minute,
				PortBandwidthGB:                 portBandwidthGB,
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
				VMContainerDiskImage:            vmContainerDiskImage,
				TestDuration:                    30 * time.Minute,
				PortBandwidthGB:                 portBandwidthGB,
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
			assert.NotNil(t, actualConfig.TrafficGeneratorEastMacAddress)
			assert.NotNil(t, actualConfig.TrafficGeneratorWestMacAddress)
			assert.NotNil(t, actualConfig.DPDKEastMacAddress)
			assert.NotNil(t, actualConfig.DPDKWestMacAddress)

			testCase.expectedConfig.TrafficGeneratorEastMacAddress = actualConfig.TrafficGeneratorEastMacAddress
			testCase.expectedConfig.TrafficGeneratorWestMacAddress = actualConfig.TrafficGeneratorWestMacAddress
			testCase.expectedConfig.DPDKEastMacAddress = actualConfig.DPDKEastMacAddress
			testCase.expectedConfig.DPDKWestMacAddress = actualConfig.DPDKWestMacAddress

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
			description:    "trafficGenTargetNodeName is missing and DPDKNodeLabelSelector is set",
			key:            config.TrafficGenTargetNodeNameParamName,
			faultyKeyValue: "",
			expectedError:  config.ErrIllegalLabelSelectorCombination,
		},
		{
			description:    "DPDKNodeLabelSelector is missing and trafficGenTargetNodeName is set",
			key:            config.DPDKNodeLabelSelectorParamName,
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
			description:    "PortBandwidthGB is invalid",
			key:            config.PortBandwidthGBParamName,
			faultyKeyValue: "0",
			expectedError:  config.ErrInvalidPortBandwidthGB,
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
	delete(paramsWithOutNodeSelectors, config.DPDKNodeLabelSelectorParamName)
	return paramsWithOutNodeSelectors
}

func getValidUserParameters() map[string]string {
	return map[string]string{
		config.NetworkAttachmentDefinitionNameParamName: networkAttachmentDefinitionName,
		config.TrafficGenContainerDiskImageParamName:    testTrafficGenContainerDiskImage,
		config.TrafficGenTargetNodeNameParamName:        testTrafficGenTargetNodeName,
		config.TrafficGenPacketsPerSecondParamName:      testTrafficGenPacketsPerSecond,
		config.VMContainerDiskImageParamName:            vmContainerDiskImage,
		config.DPDKNodeLabelSelectorParamName:           dpdkNodeLabelSelector,
		config.TestDurationParamName:                    testDuration,
		config.PortBandwidthGBParamName:                 fmt.Sprintf("%d", portBandwidthGB),
		config.VerboseParamName:                         strconv.FormatBool(true),
	}
}
