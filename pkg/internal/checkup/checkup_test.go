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

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
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
	dpdkEastMacAddress                  = "DE:AD:BE:EF:00:02"
	dpdkWestMacAddress                  = "DE:AD:BE:EF:02:00"
)

func TestCheckupShouldSucceed(t *testing.T) {
	testClient := newClientStub()
	testConfig := newTestConfig()
	testCheckup := checkup.New(testClient, testNamespace, testConfig)
	testClient.returnNetAttachDef = newNetAttachDef(testConfig.NetworkAttachmentDefinitionName)

	assert.NoError(t, testCheckup.Setup(context.Background()))
	assert.NoError(t, testCheckup.Run(context.Background()))
	assert.NoError(t, testCheckup.Teardown(context.Background()))

	vmiName := testClient.VMIName()
	assert.NotEmpty(t, vmiName)
	_, err := testClient.GetVirtualMachineInstance(context.Background(), testNamespace, vmiName)
	assert.ErrorContains(t, err, "not found")

	podName := testClient.TrafficGeneratorPodName()
	assert.NotEmpty(t, podName)
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
		testCheckup := checkup.New(testClient, testNamespace, testConfig)
		testClient.returnNetAttachDef = newNetAttachDef(testConfig.NetworkAttachmentDefinitionName)

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedVMICreationFailure.Error())
	})

	t.Run("when Pod creation fails", func(t *testing.T) {
		expectedPodCreationFailure := errors.New("failed to create Pod")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.podCreationFailure = expectedPodCreationFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig)
		testClient.returnNetAttachDef = newNetAttachDef(testConfig.NetworkAttachmentDefinitionName)

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedPodCreationFailure.Error())
	})

	t.Run("when wait Pod Running fails on read", func(t *testing.T) {
		expectedPodReadFailure := errors.New("failed to read Pod")

		testClient := newClientStub()
		testConfig := newTestConfig()
		testClient.podReadFailure = expectedPodReadFailure
		testCheckup := checkup.New(testClient, testNamespace, testConfig)
		testClient.returnNetAttachDef = newNetAttachDef(testConfig.NetworkAttachmentDefinitionName)

		assert.ErrorContains(t, testCheckup.Setup(context.Background()), expectedPodReadFailure.Error())
	})
}

func TestTeardownShouldFailWhen(t *testing.T) {
	type FailTestCase struct {
		description        string
		vmiReadFailure     error
		vmiDeletionFailure error
		podDeletionFailure error
		expectedFailure    string
	}

	const (
		vmiReadFailureMsg     = "failed to delete VMI"
		vmiDeletionFailureMsg = "failed to read VMI"
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
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			testClient := newClientStub()
			testConfig := newTestConfig()

			testCheckup := checkup.New(testClient, testNamespace, testConfig)
			testClient.returnNetAttachDef = newNetAttachDef(testConfig.NetworkAttachmentDefinitionName)

			assert.NoError(t, testCheckup.Setup(context.Background()))
			assert.NoError(t, testCheckup.Run(context.Background()))

			testClient.vmiDeletionFailure = testCase.vmiDeletionFailure
			testClient.vmiReadFailure = testCase.vmiReadFailure
			testClient.podDeletionFailure = testCase.podDeletionFailure
			assert.ErrorContains(t, testCheckup.Teardown(context.Background()), testCase.expectedFailure)
		})
	}
}

type clientStub struct {
	createdVMIs        map[string]*kvcorev1.VirtualMachineInstance
	createdPods        map[string]*k8scorev1.Pod
	returnNetAttachDef *networkv1.NetworkAttachmentDefinition
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

	vmiFullName := checkup.ObjectFullName(namespace, vmi.Name)
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

	return vmi, nil
}

func (cs *clientStub) DeleteVirtualMachineInstance(_ context.Context, namespace, name string) error {
	if cs.vmiDeletionFailure != nil {
		return cs.vmiDeletionFailure
	}

	vmiFullName := checkup.ObjectFullName(namespace, name)
	delete(cs.createdVMIs, vmiFullName)

	return nil
}

func (cs *clientStub) GetNetworkAttachmentDefinition(_ context.Context, _, _ string) (*networkv1.NetworkAttachmentDefinition, error) {
	return cs.returnNetAttachDef, nil
}

func (cs *clientStub) CreatePod(_ context.Context, namespace string, pod *k8scorev1.Pod) (*k8scorev1.Pod, error) {
	if cs.podCreationFailure != nil {
		return nil, cs.podCreationFailure
	}

	podFullName := checkup.ObjectFullName(namespace, pod.Name)
	pod.Status.Phase = k8scorev1.PodRunning
	cs.createdPods[podFullName] = pod

	return pod, nil
}

func (cs *clientStub) DeletePod(_ context.Context, namespace, name string) error {
	if cs.podDeletionFailure != nil {
		return cs.podDeletionFailure
	}

	podFullName := checkup.ObjectFullName(namespace, name)
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
	for podName := range cs.createdPods {
		if strings.Contains(podName, checkup.TrexPodNamePrefix) {
			return podName
		}
	}

	return ""
}

func newNetAttachDef(name string) *networkv1.NetworkAttachmentDefinition {
	return &networkv1.NetworkAttachmentDefinition{
		ObjectMeta: k8smetav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
	}
}

func (cs *clientStub) VMIName() string {
	for vmiName := range cs.createdVMIs {
		if strings.Contains(vmiName, checkup.VMINamePrefix) {
			return vmiName
		}
	}

	return ""
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
