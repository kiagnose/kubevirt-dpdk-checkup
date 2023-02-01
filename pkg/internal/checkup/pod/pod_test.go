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

package pod_test

import (
	"testing"

	assert "github.com/stretchr/testify/require"

	k8scorev1 "k8s.io/api/core/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/pod"
)

const testPodName = "my-pod"
const testContainerName = "my-container"

func TestCreateNetworkRequestAnnotationShouldSucceed(t *testing.T) {
	type successTestCase struct {
		description                 string
		networkSelectionElementList []networkv1.NetworkSelectionElement
		expectedNetworksRequests    []string
	}

	testCases := []successTestCase{
		{
			description:                 "when the input map is empty",
			networkSelectionElementList: []networkv1.NetworkSelectionElement{},
			expectedNetworksRequests:    []string{"[]"},
		},
		{
			description: "when the input map is not empty",
			networkSelectionElementList: []networkv1.NetworkSelectionElement{
				{Name: "name1", Namespace: "namespace1", MacRequest: "AA:AA:AA:AA:AA:01"},
				{Name: "name2", Namespace: "namespace2", MacRequest: "AA:AA:AA:AA:AA:02"},
			},
			expectedNetworksRequests: []string{
				"{\"name\":\"name1\",\"namespace\":\"namespace1\",\"mac\":\"AA:AA:AA:AA:AA:01\"}",
				"{\"name\":\"name2\",\"namespace\":\"namespace2\",\"mac\":\"AA:AA:AA:AA:AA:02\"}",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			actualNetworksRequest, err := pod.CreateNetworksRequest(testCase.networkSelectionElementList)
			assert.NoError(t, err)
			for _, expectedRequest := range testCase.expectedNetworksRequests {
				assert.Contains(t, actualNetworksRequest, expectedRequest)
			}
		})
	}
}

func TestWithNewPod(t *testing.T) {
	actualPod := pod.NewPod(testPodName)
	expectedPod := newBasePod(actualPod.Name)

	assert.Equal(t, expectedPod, actualPod)
}

func TestWithOwnerReference(t *testing.T) {
	actualPod := pod.NewPod(testPodName, pod.WithOwnerReference("name", "id"))

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.OwnerReferences = []k8smetav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       "name",
			UID:        "id",
		},
	}

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithLabels(t *testing.T) {
	labels := map[string]string{"key1": "value1", "key2": "value2"}
	actualPod := pod.NewPod(testPodName, pod.WithLabels(labels))

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.Labels = labels

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithHugepagesVolume(t *testing.T) {
	actualPod := pod.NewPod(testPodName, pod.WithHugepagesVolume())

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.Spec.Volumes = []k8scorev1.Volume{
		{
			Name: "hugepages",
			VolumeSource: k8scorev1.VolumeSource{
				EmptyDir: &k8scorev1.EmptyDirVolumeSource{
					Medium: "HugePages",
				},
			},
		},
	}

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithLibModulesVolume(t *testing.T) {
	actualPod := pod.NewPod(testPodName, pod.WithLibModulesVolume())

	directoryHostPath := k8scorev1.HostPathDirectory
	expectedPod := newBasePod(actualPod.Name)
	expectedPod.Spec.Volumes = []k8scorev1.Volume{
		{
			Name: "modules",
			VolumeSource: k8scorev1.VolumeSource{
				HostPath: &k8scorev1.HostPathVolumeSource{
					Path: "/lib/modules",
					Type: &directoryHostPath,
				},
			},
		},
	}

	assert.Equal(t, expectedPod, actualPod)
}

func TestWithTerminationGracePeriodSeconds(t *testing.T) {
	terminationGracePeriodSeconds := int64(1024)
	actualPod := pod.NewPod(testPodName, pod.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds))

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.Spec.TerminationGracePeriodSeconds = &terminationGracePeriodSeconds

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithoutCRIOCPULoadBalancing(t *testing.T) {
	actualPod := pod.NewPod(testPodName, pod.WithoutCRIOCPULoadBalancing())

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.ObjectMeta.Annotations = map[string]string{
		pod.CRIOCPULoadBalancingAnnotation: pod.Disable,
	}

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithoutCRIOCPUQuota(t *testing.T) {
	actualPod := pod.NewPod(testPodName, pod.WithoutCRIOCPUQuota())

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.ObjectMeta.Annotations = map[string]string{
		pod.CRIOCPUQuotaAnnotation: pod.Disable,
	}

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithoutCRIOIRQLoadBalancing(t *testing.T) {
	actualPod := pod.NewPod(testPodName, pod.WithoutCRIOIRQLoadBalancing())

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.ObjectMeta.Annotations = map[string]string{
		pod.CRIOIRQLoadBalancingAnnotation: pod.Disable,
	}

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithNodeSelector(t *testing.T) {
	actualPod := pod.NewPod(testPodName, pod.WithNodeSelector("node-name"))

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.Spec.NodeSelector = map[string]string{
		k8scorev1.LabelHostname: "node-name",
	}

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithRuntimeClassName(t *testing.T) {
	runtimeClass := "my-runtime-class"
	actualPod := pod.NewPod(testPodName, pod.WithRuntimeClassName(runtimeClass))

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.Spec.RuntimeClassName = &runtimeClass

	assert.Equal(t, expectedPod, actualPod)
}

func TestNewPodWithContainer(t *testing.T) {
	container := newContainer(testContainerName)
	actualPod := pod.NewPod(testPodName, pod.WithPodContainer(&container))

	expectedPod := newBasePod(actualPod.Name)
	expectedPod.Spec.Containers = []k8scorev1.Container{container}

	assert.Equal(t, expectedPod, actualPod)
}

func newContainer(name string) k8scorev1.Container {
	user := int64(0)
	falseBool := false
	return k8scorev1.Container{
		Name:            name,
		Image:           "image",
		ImagePullPolicy: k8scorev1.PullAlways,
		Command:         []string{"/bin/bash", "-c", "sleep INF"},
		VolumeMounts: []k8scorev1.VolumeMount{
			{
				Name:      "modules",
				MountPath: "/lib/modules",
				ReadOnly:  true,
			},
		},
		SecurityContext: &k8scorev1.SecurityContext{
			AllowPrivilegeEscalation: &falseBool,
			Capabilities: &k8scorev1.Capabilities{
				Add: []k8scorev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"},
			},
			RunAsUser: &user,
		},
	}
}

func newBasePod(name string) *k8scorev1.Pod {
	return &k8scorev1.Pod{
		ObjectMeta: k8smetav1.ObjectMeta{
			Name: name,
		},
		Spec: k8scorev1.PodSpec{},
	}
}
