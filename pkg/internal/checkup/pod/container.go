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

package pod

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ContainerOption represents a container action that enables an option.
type ContainerOption func(pod *corev1.Container)

const ContainerDiskImage = "quay.io/schseba/trex:2.87"

func NewPodContainer(name string, opts ...ContainerOption) *corev1.Container {
	container := &corev1.Container{
		Name: name,
	}

	for _, f := range opts {
		f(container)
	}
	return container
}

func WithContainerImage(image string) ContainerOption {
	return func(container *corev1.Container) {
		container.Image = image
		container.ImagePullPolicy = corev1.PullAlways
	}
}

func WithContainerCommand(command []string) ContainerOption {
	return func(container *corev1.Container) {
		container.Command = command
	}
}

func WithContainerSecurityContext(sc *corev1.SecurityContext) ContainerOption {
	return func(container *corev1.Container) {
		container.SecurityContext = sc
	}
}

func WithContainerEnvVars(envVars map[string]string) ContainerOption {
	return func(container *corev1.Container) {
		for key, val := range envVars {
			envVar := corev1.EnvVar{
				Name:  key,
				Value: val,
			}
			container.Env = append(container.Env, envVar)
		}
	}
}

func WithContainerCPUsStrict(cpusNumString string) ContainerOption {
	return func(container *corev1.Container) {
		if container.Resources.Requests == nil {
			container.Resources.Requests = make(corev1.ResourceList)
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = make(corev1.ResourceList)
		}
		numCpusQuantity := resource.MustParse(cpusNumString)
		container.Resources.Requests[corev1.ResourceCPU] = numCpusQuantity
		container.Resources.Limits[corev1.ResourceCPU] = numCpusQuantity
	}
}

func WithContainerHugepagesResources(hugepagesNumString string) ContainerOption {
	return func(container *corev1.Container) {
		const memorySize = "1Gi"
		memoryQuantity := resource.MustParse(memorySize)
		container.Resources.Requests[corev1.ResourceMemory] = memoryQuantity
		container.Resources.Limits[corev1.ResourceMemory] = memoryQuantity

		hugepagesQuantity := resource.MustParse(hugepagesNumString)
		container.Resources.Requests[corev1.ResourceHugePagesPrefix+memorySize] = hugepagesQuantity
		container.Resources.Limits[corev1.ResourceHugePagesPrefix+memorySize] = hugepagesQuantity
	}
}

func WithContainerHugepagesVolumeMount() ContainerOption {
	return func(container *corev1.Container) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "hugepages",
			MountPath: "/mnt/huge",
		})
	}
}

func WithContainerLibModulesVolumeMount() ContainerOption {
	return func(container *corev1.Container) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "modules",
			MountPath: "/lib/modules",
			ReadOnly:  true,
		})
	}
}

func NewSecurityContext(user int64, allowPrivilegeEscalation bool, addCapabilities []corev1.Capability) *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: pointer(allowPrivilegeEscalation),
		Capabilities: &corev1.Capabilities{
			Add: addCapabilities,
		},
		RunAsUser: pointer(user),
	}
}
