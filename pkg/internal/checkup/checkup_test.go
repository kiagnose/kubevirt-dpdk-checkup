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
	dpdkEastMacAddress                  = "DE:AD:BE:EF:00:02"
	dpdkWestMacAddress                  = "DE:AD:BE:EF:02:00"
)

func TestCheckupShouldSucceed(t *testing.T) {
	testClient := newClientStub()
	testConfig := newTestConfig()
	testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

	assert.NoError(t, testCheckup.Setup(context.Background()))

	vmiName := testClient.VMIName()
	assert.NotEmpty(t, vmiName)

	podName := testClient.TrafficGeneratorPodName()
	assert.NotEmpty(t, podName)

	assert.NoError(t, testCheckup.Run(context.Background()))
	assert.NoError(t, testCheckup.Teardown(context.Background()))

	_, err := testClient.GetVirtualMachineInstance(context.Background(), testNamespace, vmiName)
	assert.ErrorContains(t, err, "not found")

	_, err = testClient.GetPod(context.Background(), testNamespace, podName)
	assert.ErrorContains(t, err, "not found")

	actualResults := testCheckup.Results()
	expectedResults := status.Results{}

	assert.Equal(t, expectedResults, actualResults)
}

func TestSetupShouldFail(t *testing.T) {
	t.Run("when VMI creation fails", func(t *testing.T) {
		expectedVMICreationFailure := errors.New("failed to create VMI")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.vmiCreationFailure = expectedVMICreationFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedVMICreationFailure.Error())
	})

	t.Run("when wait for VMI to boot fails", func(t *testing.T) {
		expectedVMIReadFailure := errors.New("failed to read VMI")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.vmiReadFailure = expectedVMIReadFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedVMIReadFailure.Error())
	})

	t.Run("when Pod creation fails", func(t *testing.T) {
		expectedPodCreationFailure := errors.New("failed to create Pod")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.podCreationFailure = expectedPodCreationFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedPodCreationFailure.Error())
	})

	t.Run("when wait Pod Running fails on read", func(t *testing.T) {
		expectedPodReadFailure := errors.New("failed to read Pod")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.podReadFailure = expectedPodReadFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{})

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedPodReadFailure.Error())
	})
}

func TestTeardownShouldFailWhen(t *testing.T) {
	type FailTestCase struct {
		description        string
		vmiReadFailure     error
		vmiDeletionFailure error
		podDeletionFailure error
		podReadFailure     error
		expectedFailure    string
	}

	const (
		vmiReadFailureMsg     = "failed to delete VMI"
		vmiDeletionFailureMsg = "failed to read VMI"
		podReadFailureMsg     = "failed to read Pod"
		podDeletionFailureMsg = "failed to delete Pod"
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
		{
			description:        "Traffic generator Pod deletion fails",
			podDeletionFailure: errors.New(podDeletionFailureMsg),
			expectedFailure:    podDeletionFailureMsg,
		},
		{
			description:     "wait for Traffic generator Pod deletion fails",
			podReadFailure:  errors.New(podReadFailureMsg),
			expectedFailure: podReadFailureMsg,
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
			testClient.podDeletionFailure = testCase.podDeletionFailure
			testClient.podReadFailure = testCase.podReadFailure
			assert.ErrorContains(t, testCheckup.Teardown(context.Background()), testCase.expectedFailure)
		})
	}
}

func TestRunFailure(t *testing.T) {
	expectedExecutionFailure := errors.New("failed to execute dpdk checkup")

	testClient := newClientStub()
	testConfig := newTestConfig()
	testCheckup := checkup.New(testClient, testNamespace, testConfig, executorStub{executeErr: expectedExecutionFailure})

	assert.NoError(t, testCheckup.Setup(context.Background()))

	vmiName := testClient.VMIName()
	assert.NotEmpty(t, vmiName)

	podName := testClient.TrafficGeneratorPodName()
	assert.NotEmpty(t, podName)

	assert.Error(t, expectedExecutionFailure, testCheckup.Run(context.Background()))

	assert.NoError(t, testCheckup.Teardown(context.Background()))

	_, err := testClient.GetVirtualMachineInstance(context.Background(), testNamespace, vmiName)
	assert.ErrorContains(t, err, "not found")

	_, err = testClient.GetPod(context.Background(), testNamespace, podName)
	assert.ErrorContains(t, err, "not found")

	actualResults := testCheckup.Results()
	expectedResults := status.Results{}

	assert.Equal(t, expectedResults, actualResults)
}

func TestCloudInitString(t *testing.T) {
	actualString := checkup.CloudInit("user", "password")
	expectedString := `#cloud-config
user: user
password: password
chpasswd:
  expire: false`

	assert.Equal(t, expectedString, actualString)
}

type clientStub struct {
	createdVMIs        map[string]*kvcorev1.VirtualMachineInstance
	createdPods        map[string]*k8scorev1.Pod
	podCreationFailure error
	podReadFailure     error
	podDeletionFailure error
	vmiCreationFailure error
	vmiReadFailure     error
	vmiDeletionFailure error
}

func newClientStub() *clientStub {
	return &clientStub{
		createdVMIs: map[string]*kvcorev1.VirtualMachineInstance{},
		createdPods: map[string]*k8scorev1.Pod{},
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

func (cs *clientStub) CreatePod(_ context.Context, namespace string, pod *k8scorev1.Pod) (*k8scorev1.Pod, error) {
	if cs.podCreationFailure != nil {
		return nil, cs.podCreationFailure
	}

	pod.Namespace = namespace

	podFullName := checkup.ObjectFullName(pod.Namespace, pod.Name)
	pod.Status.Phase = k8scorev1.PodRunning
	cs.createdPods[podFullName] = pod

	return pod, nil
}

func (cs *clientStub) DeletePod(_ context.Context, namespace, name string) error {
	if cs.podDeletionFailure != nil {
		return cs.podDeletionFailure
	}

	podFullName := checkup.ObjectFullName(namespace, name)
	_, exist := cs.createdPods[podFullName]
	if !exist {
		return k8serrors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, name)
	}

	delete(cs.createdPods, podFullName)

	return nil
}

func (cs *clientStub) GetPod(_ context.Context, namespace, name string) (*k8scorev1.Pod, error) {
	if cs.podReadFailure != nil {
		return nil, cs.podReadFailure
	}

	podFullName := checkup.ObjectFullName(namespace, name)
	pod, exist := cs.createdPods[podFullName]
	if !exist {
		return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, name)
	}
	return pod, nil
}

func (cs *clientStub) TrafficGeneratorPodName() string {
	for _, pod := range cs.createdPods {
		if strings.Contains(pod.Name, checkup.TrafficGeneratorPodNamePrefix) {
			return pod.Name
		}
	}

	return ""
}

func (cs *clientStub) VMIName() string {
	for _, vmi := range cs.createdVMIs {
		if strings.Contains(vmi.Name, checkup.VMINamePrefix) {
			return vmi.Name
		}
	}

	return ""
}

type executorStub struct {
	executeErr error
}

func (es executorStub) Execute(_ context.Context, vmiName string) (status.Results, error) {
	if es.executeErr != nil {
		return status.Results{}, es.executeErr
	}

	return status.Results{}, nil
}

func newTestConfig() config.Config {
	trafficGeneratorEastHWAddress, _ := net.ParseMAC(trafficGeneratorEastMacAddress)
	trafficGeneratorWestHWAddress, _ := net.ParseMAC(trafficGeneratorWestMacAddress)
	dpdkEastHWAddress, _ := net.ParseMAC(dpdkEastMacAddress)
	dpdkWestHWAddress, _ := net.ParseMAC(dpdkWestMacAddress)
	return config.Config{
		PodName:                           testPodName,
		PodUID:                            testPodUID,
		NUMASocket:                        0,
		NetworkAttachmentDefinitionName:   testNetworkAttachmentDefinitionName,
		TrafficGeneratorNodeLabelSelector: "",
		DPDKNodeLabelSelector:             "",
		TrafficGeneratorPacketsPerSecondInMillions: config.TrafficGeneratorPacketsPerSecondInMillionsDefault,
		PortBandwidthGB:                config.PortBandwidthGBDefault,
		TrafficGeneratorEastMacAddress: trafficGeneratorEastHWAddress,
		TrafficGeneratorWestMacAddress: trafficGeneratorWestHWAddress,
		DPDKEastMacAddress:             dpdkEastHWAddress,
		DPDKWestMacAddress:             dpdkWestHWAddress,
		TestDuration:                   config.TestDurationDefault,
	}
}
