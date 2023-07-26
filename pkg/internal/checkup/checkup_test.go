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

package checkup_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	assert "github.com/stretchr/testify/require"

	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	kvcorev1 "kubevirt.io/api/core/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

const (
	testPodName                         = "dpdk-checkup-pod"
	testPodUID                          = "0123456789-0123456789"
	testNamespace                       = "target-ns"
	testNetworkAttachmentDefinitionName = "dpdk-network"
	trafficGeneratorEastMacAddress      = "DE:AD:BE:EF:00:01"
	trafficGeneratorWestMacAddress      = "DE:AD:BE:EF:01:00"
	vmiUnderTestEastMacAddress          = "DE:AD:BE:EF:00:02"
	vmiUnderTestWestMacAddress          = "DE:AD:BE:EF:02:00"
)

func TestCheckupShouldSucceed(t *testing.T) {
	testClient := newClientStub()
	testConfig := newTestConfig()
	testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

	assert.NoError(t, testCheckup.Setup(context.Background()))

	assert.NotEmpty(t, testClient.createdConfigMaps)

	vmiUnderTestName := testClient.VMIName(checkup.VMIUnderTestNamePrefix)
	assert.NotEmpty(t, vmiUnderTestName)

	trafficGenName := testClient.VMIName(checkup.TrafficGenNamePrefix)
	assert.NotEmpty(t, trafficGenName)

	assert.NoError(t, testCheckup.Run(context.Background()))
	assert.NoError(t, testCheckup.Teardown(context.Background()))

	assert.Empty(t, testClient.createdVMIs)
	assert.Empty(t, testClient.createdConfigMaps)

	actualResults := testCheckup.Results()
	expectedResults := status.Results{}

	assert.Equal(t, expectedResults, actualResults)
}

func TestVMIAffinity(t *testing.T) {
	t.Run("when node names are not specified", func(t *testing.T) {
		testClient := newClientStub()
		testConfig := newTestConfig()
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})
		assert.NoError(t, testCheckup.Setup(context.Background()))

		vmiUnderTestName := testClient.VMIName(checkup.VMIUnderTestNamePrefix)
		assert.NotEmpty(t, vmiUnderTestName)

		trafficGenName := testClient.VMIName(checkup.TrafficGenNamePrefix)
		assert.NotEmpty(t, trafficGenName)

		assertPodAntiAffinityExists(t, testClient, vmiUnderTestName, testConfig.PodUID)
		assertNodeAffinityDoesNotExist(t, testClient, vmiUnderTestName)

		assertPodAntiAffinityExists(t, testClient, trafficGenName, testConfig.PodUID)
		assertNodeAffinityDoesNotExist(t, testClient, trafficGenName)
	})

	t.Run("when node names are specified", func(t *testing.T) {
		const (
			vmiUnderTestNodeName = "node01"
			trafficGenNodeName   = "node02"
		)

		testClient := newClientStub()
		testConfig := newTestConfig()
		testConfig.VMUnderTestTargetNodeName = vmiUnderTestNodeName
		testConfig.TrafficGenTargetNodeName = trafficGenNodeName

		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})
		assert.NoError(t, testCheckup.Setup(context.Background()))

		vmiUnderTestName := testClient.VMIName(checkup.VMIUnderTestNamePrefix)
		assert.NotEmpty(t, vmiUnderTestName)

		trafficGenName := testClient.VMIName(checkup.TrafficGenNamePrefix)
		assert.NotEmpty(t, trafficGenName)

		assertNodeAffinityExists(t, testClient, vmiUnderTestName, vmiUnderTestNodeName)
		assertPodAntiAffinityDoesNotExist(t, testClient, vmiUnderTestName)

		assertNodeAffinityExists(t, testClient, trafficGenName, trafficGenNodeName)
		assertPodAntiAffinityDoesNotExist(t, testClient, trafficGenName)
	})
}

func TestSetupShouldFail(t *testing.T) {
	t.Run("when Traffic gen ConfigMap creation fails", func(t *testing.T) {
		expectedConfigMapCreationError := errors.New("failed to create ConfigMap")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.configMapCreationFailure = expectedConfigMapCreationError
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedConfigMapCreationError.Error())
		assert.Empty(t, testClient.createdVMIs)
	})

	t.Run("when VMI creation fails", func(t *testing.T) {
		expectedVMICreationFailure := errors.New("failed to create VMI")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.vmiCreationFailure = expectedVMICreationFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedVMICreationFailure.Error())
		assert.Empty(t, testClient.createdVMIs)
	})

	t.Run("when wait for VMI to boot fails", func(t *testing.T) {
		expectedVMIReadFailure := errors.New("failed to read VMI")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.vmiReadFailure = expectedVMIReadFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedVMIReadFailure.Error())
		assert.Empty(t, testClient.createdVMIs)
	})
}

func TestTeardownShouldFailWhen(t *testing.T) {
	type FailTestCase struct {
		description        string
		vmiReadFailure     error
		vmiDeletionFailure error
		expectedFailure    string
	}

	const (
		vmiReadFailureMsg     = "failed to delete VMI"
		vmiDeletionFailureMsg = "failed to read VMI"
	)
	testCases := []FailTestCase{
		{
			description:     "VMI deletion fails",
			vmiReadFailure:  errors.New(vmiReadFailureMsg),
			expectedFailure: vmiReadFailureMsg,
		},
		{
			description:        "wait for VMI deletion fails",
			vmiDeletionFailure: errors.New(vmiDeletionFailureMsg),
			expectedFailure:    vmiDeletionFailureMsg,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			testClient := newClientStub()
			testConfig := newTestConfig()

			testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

			assert.NoError(t, testCheckup.Setup(context.Background()))
			assert.NoError(t, testCheckup.Run(context.Background()))

			testClient.vmiDeletionFailure = testCase.vmiDeletionFailure
			testClient.vmiReadFailure = testCase.vmiReadFailure
			assert.ErrorContains(t, testCheckup.Teardown(context.Background()), testCase.expectedFailure)
		})
	}
}

func TestTrafficGenCMTeardownFailure(t *testing.T) {
	testClient := newClientStub()
	testConfig := newTestConfig()

	testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

	assert.NoError(t, testCheckup.Setup(context.Background()))
	assert.NotEmpty(t, testClient.createdConfigMaps)

	assert.NoError(t, testCheckup.Run(context.Background()))

	expectedCMDeletionFailure := errors.New("failed to delete ConfigMap")
	testClient.configMapDeletionFailure = expectedCMDeletionFailure

	assert.ErrorContains(t, testCheckup.Teardown(context.Background()), expectedCMDeletionFailure.Error())
}

func TestRunFailure(t *testing.T) {
	expectedExecutionFailure := errors.New("failed to execute dpdk checkup")

	testClient := newClientStub()
	testConfig := newTestConfig()
	testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{executeErr: expectedExecutionFailure})

	assert.NoError(t, testCheckup.Setup(context.Background()))

	vmiUnderTestName := testClient.VMIName(checkup.VMIUnderTestNamePrefix)
	assert.NotEmpty(t, vmiUnderTestName)

	assert.Error(t, expectedExecutionFailure, testCheckup.Run(context.Background()))

	assert.NoError(t, testCheckup.Teardown(context.Background()))

	assert.Empty(t, testClient.createdVMIs)

	actualResults := testCheckup.Results()
	expectedResults := status.Results{}

	assert.Equal(t, expectedResults, actualResults)
}

func assertPodAntiAffinityExists(t *testing.T, testClient *clientStub, vmiName, ownerUID string) {
	actualVMI, err := testClient.GetVirtualMachineInstance(context.Background(), testNamespace, vmiName)
	assert.NoError(t, err)

	expectedAffinity := &k8scorev1.Affinity{
		PodAntiAffinity: &k8scorev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []k8scorev1.WeightedPodAffinityTerm{
				{
					Weight: 1,
					PodAffinityTerm: k8scorev1.PodAffinityTerm{
						TopologyKey: k8scorev1.LabelHostname,
						LabelSelector: &k8smetav1.LabelSelector{
							MatchExpressions: []k8smetav1.LabelSelectorRequirement{
								{
									Operator: k8smetav1.LabelSelectorOpIn,
									Key:      checkup.DPDKCheckupUIDLabelKey,
									Values:   []string{ownerUID},
								},
							},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, expectedAffinity, actualVMI.Spec.Affinity)
}

func assertPodAntiAffinityDoesNotExist(t *testing.T, testClient *clientStub, vmiName string) {
	actualVmi, err := testClient.GetVirtualMachineInstance(context.Background(), testNamespace, vmiName)
	assert.NoError(t, err)

	assert.Nil(t, actualVmi.Spec.Affinity.PodAntiAffinity)
}

func assertNodeAffinityExists(t *testing.T, testClient *clientStub, vmiName, nodeName string) {
	actualVMI, err := testClient.GetVirtualMachineInstance(context.Background(), testNamespace, vmiName)
	assert.NoError(t, err)

	expectedAffinity := &k8scorev1.Affinity{
		NodeAffinity: &k8scorev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &k8scorev1.NodeSelector{
				NodeSelectorTerms: []k8scorev1.NodeSelectorTerm{
					{
						MatchExpressions: []k8scorev1.NodeSelectorRequirement{
							{
								Key:      k8scorev1.LabelHostname,
								Operator: k8scorev1.NodeSelectorOpIn,
								Values:   []string{nodeName}},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, expectedAffinity, actualVMI.Spec.Affinity)
}

func assertNodeAffinityDoesNotExist(t *testing.T, testClient *clientStub, vmiName string) {
	actualVmi, err := testClient.GetVirtualMachineInstance(context.Background(), testNamespace, vmiName)
	assert.NoError(t, err)

	assert.Nil(t, actualVmi.Spec.Affinity.NodeAffinity)
}

type clientStub struct {
	createdVMIs              map[string]*kvcorev1.VirtualMachineInstance
	vmiCreationFailure       error
	vmiReadFailure           error
	vmiDeletionFailure       error
	createdConfigMaps        map[string]*k8scorev1.ConfigMap
	configMapCreationFailure error
	configMapDeletionFailure error
}

func newClientStub() *clientStub {
	return &clientStub{
		createdVMIs:       map[string]*kvcorev1.VirtualMachineInstance{},
		createdConfigMaps: map[string]*k8scorev1.ConfigMap{},
	}
}

func (cs *clientStub) CreateVirtualMachineInstance(_ context.Context,
	namespace string,
	vmi *kvcorev1.VirtualMachineInstance) (*kvcorev1.VirtualMachineInstance, error) {
	if cs.vmiCreationFailure != nil {
		return nil, cs.vmiCreationFailure
	}

	vmi.Namespace = namespace

	vmiFullName := checkup.ObjectFullName(vmi.Namespace, vmi.Name)
	cs.createdVMIs[vmiFullName] = vmi

	return vmi, nil
}

func (cs *clientStub) GetVirtualMachineInstance(_ context.Context, namespace, name string) (*kvcorev1.VirtualMachineInstance, error) {
	if cs.vmiReadFailure != nil {
		return nil, cs.vmiReadFailure
	}

	vmiFullName := checkup.ObjectFullName(namespace, name)
	vmi, exist := cs.createdVMIs[vmiFullName]
	if !exist {
		return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachineinstances"}, name)
	}

	vmi.Status.Conditions = append(vmi.Status.Conditions, kvcorev1.VirtualMachineInstanceCondition{
		Type:   kvcorev1.VirtualMachineInstanceAgentConnected,
		Status: k8scorev1.ConditionTrue,
	})

	return vmi, nil
}

func (cs *clientStub) DeleteVirtualMachineInstance(_ context.Context, namespace, name string) error {
	if cs.vmiDeletionFailure != nil {
		return cs.vmiDeletionFailure
	}

	vmiFullName := checkup.ObjectFullName(namespace, name)
	_, exist := cs.createdVMIs[vmiFullName]
	if !exist {
		return k8serrors.NewNotFound(schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachineinstances"}, name)
	}

	delete(cs.createdVMIs, vmiFullName)

	return nil
}

func (cs *clientStub) CreateConfigMap(_ context.Context, namespace string, configMap *k8scorev1.ConfigMap) (*k8scorev1.ConfigMap, error) {
	if cs.configMapCreationFailure != nil {
		return nil, cs.configMapCreationFailure
	}

	configMap.Namespace = namespace

	configMapFullName := checkup.ObjectFullName(configMap.Namespace, configMap.Name)
	cs.createdConfigMaps[configMapFullName] = configMap

	return configMap, nil
}

func (cs *clientStub) DeleteConfigMap(_ context.Context, namespace, name string) error {
	if cs.configMapDeletionFailure != nil {
		return cs.configMapDeletionFailure
	}

	configMapFullName := checkup.ObjectFullName(namespace, name)
	_, exist := cs.createdConfigMaps[configMapFullName]
	if !exist {
		return k8serrors.NewNotFound(schema.GroupResource{Group: "", Resource: "configmaps"}, name)
	}

	delete(cs.createdConfigMaps, configMapFullName)

	return nil
}

func (cs *clientStub) VMIName(namePrefix string) string {
	for _, vmi := range cs.createdVMIs {
		if strings.Contains(vmi.Name, namePrefix) {
			return vmi.Name
		}
	}

	return ""
}

type executorStub struct {
	executeErr error
}

func (es executorStub) Execute(_ context.Context, vmiUnderTestName, trafficGenVMIName string) (status.Results, error) {
	if es.executeErr != nil {
		return status.Results{}, es.executeErr
	}

	return status.Results{}, nil
}

func newTestConfig() config.Config {
	trafficGeneratorEastHWAddress, _ := net.ParseMAC(trafficGeneratorEastMacAddress)
	trafficGeneratorWestHWAddress, _ := net.ParseMAC(trafficGeneratorWestMacAddress)
	vmiUnderTestEastHWAddress, _ := net.ParseMAC(vmiUnderTestEastMacAddress)
	vmiUnderTestWestHWAddress, _ := net.ParseMAC(vmiUnderTestWestMacAddress)
	return config.Config{
		PodName:                         testPodName,
		PodUID:                          testPodUID,
		NetworkAttachmentDefinitionName: testNetworkAttachmentDefinitionName,
		TrafficGenTargetNodeName:        "",
		VMUnderTestTargetNodeName:       "",
		TrafficGenPacketsPerSecond:      config.TrafficGenDefaultPacketsPerSecond,
		PortBandwidthGbps:               config.PortBandwidthGbpsDefault,
		TrafficGenEastMacAddress:        trafficGeneratorEastHWAddress,
		TrafficGenWestMacAddress:        trafficGeneratorWestHWAddress,
		VMUnderTestEastMacAddress:       vmiUnderTestEastHWAddress,
		VMUnderTestWestMacAddress:       vmiUnderTestWestHWAddress,
		TestDuration:                    config.TestDurationDefault,
	}
}
