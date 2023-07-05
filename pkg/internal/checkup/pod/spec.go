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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// PodOption represents a pod action that enables an option.
type PodOption func(pod *corev1.Pod)

const (
	// Based on annotation names from:
	// https://github.com/cri-o/cri-o/blob/fa0fa5de1c17ddd7b6fcdbc030b6b571ce37e643/pkg/annotations/annotations.go

	// CRIOCPULoadBalancingAnnotation indicates that load balancing should be disabled for CPUs used by the container
	CRIOCPULoadBalancingAnnotation = "cpu-load-balancing.crio.io"

	// CRIOCPUQuotaAnnotation indicates that CPU quota should be disabled for CPUs used by the container
	CRIOCPUQuotaAnnotation = "cpu-quota.crio.io"

	// CRIOIRQLoadBalancingAnnotation indicates that IRQ load balancing should be disabled for CPUs used by the container
	CRIOIRQLoadBalancingAnnotation = "irq-load-balancing.crio.io"
)

const Disable = "disable"

func NewPod(name string, opts ...PodOption) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	for _, f := range opts {
		f(pod)
	}

	return pod
}

func pointer[T any](v T) *T {
	return &v
}

func WithOwnerReference(ownerName, ownerUID string) PodOption {
	return func(pod *corev1.Pod) {
		if ownerUID != "" && ownerName != "" {
			pod.ObjectMeta.OwnerReferences = append(pod.ObjectMeta.OwnerReferences, metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "Pod",
				Name:       ownerName,
				UID:        types.UID(ownerUID),
			})
		}
	}
}

func WithLabels(labels map[string]string) PodOption {
	return func(pod *corev1.Pod) {
		if pod.Labels == nil {
			pod.Labels = map[string]string{}
		}
		for key, value := range labels {
			pod.Labels[key] = value
		}
	}
}

func WithServiceAccountName(name string) PodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.ServiceAccountName = name
	}
}

// WithAffinity adds the given affinity.
func WithAffinity(affinity *corev1.Affinity) PodOption {
	return func(pod *corev1.Pod) {
		if affinity != nil {
			pod.Spec.Affinity = affinity
		}
	}
}

func WithHugepagesVolume() PodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.Volumes = append(pod.Spec.Volumes,
			corev1.Volume{
				Name: "hugepages",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium: "HugePages",
					},
				},
			})
	}
}

func WithLibModulesVolume() PodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.Volumes = append(pod.Spec.Volumes,
			corev1.Volume{
				Name: "modules",
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/lib/modules",
						Type: pointer(corev1.HostPathDirectory),
					},
				},
			})
	}
}

func WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds int64) PodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.TerminationGracePeriodSeconds = &terminationGracePeriodSeconds
	}
}

func WithoutCRIOCPULoadBalancing() PodOption {
	return func(pod *corev1.Pod) {
		if pod.ObjectMeta.Annotations == nil {
			pod.ObjectMeta.Annotations = map[string]string{}
		}

		pod.ObjectMeta.Annotations[CRIOCPULoadBalancingAnnotation] = Disable
	}
}

func WithoutCRIOCPUQuota() PodOption {
	return func(pod *corev1.Pod) {
		if pod.ObjectMeta.Annotations == nil {
			pod.ObjectMeta.Annotations = map[string]string{}
		}

		pod.ObjectMeta.Annotations[CRIOCPUQuotaAnnotation] = Disable
	}
}

func WithoutCRIOIRQLoadBalancing() PodOption {
	return func(pod *corev1.Pod) {
		if pod.ObjectMeta.Annotations == nil {
			pod.ObjectMeta.Annotations = map[string]string{}
		}

		pod.ObjectMeta.Annotations[CRIOIRQLoadBalancingAnnotation] = Disable
	}
}

func WithRuntimeClassName(runtimeClassName string) PodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.RuntimeClassName = pointer(runtimeClassName)
	}
}

func WithPodContainer(container *corev1.Container) PodOption {
	return func(pod *corev1.Pod) {
		pod.Spec.Containers = append(pod.Spec.Containers, *container)
	}
}

func PodInRunningPhase(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodRunning
}
