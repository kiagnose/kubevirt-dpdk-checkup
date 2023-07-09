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

package vmi

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kvcorev1 "kubevirt.io/api/core/v1"
)

// Based on annotation names from:
// https://github.com/cri-o/cri-o/blob/fa0fa5de1c17ddd7b6fcdbc030b6b571ce37e643/pkg/annotations/annotations.go

const (
	// CRIOCPULoadBalancingAnnotation indicates that load balancing should be disabled for CPUs used by the container
	CRIOCPULoadBalancingAnnotation = "cpu-load-balancing.crio.io"

	// CRIOCPUQuotaAnnotation indicates that CPU quota should be disabled for CPUs used by the container
	CRIOCPUQuotaAnnotation = "cpu-quota.crio.io"

	// CRIOIRQLoadBalancingAnnotation indicates that IRQ load balancing should be disabled for CPUs used by the container
	CRIOIRQLoadBalancingAnnotation = "irq-load-balancing.crio.io"
)

const Disable = "disable"

type Option func(vmi *kvcorev1.VirtualMachineInstance)

func New(name string, options ...Option) *kvcorev1.VirtualMachineInstance {
	newVMI := &kvcorev1.VirtualMachineInstance{
		TypeMeta: metav1.TypeMeta{
			Kind:       kvcorev1.VirtualMachineInstanceGroupVersionKind.Kind,
			APIVersion: kvcorev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	for _, f := range options {
		f(newVMI)
	}

	return newVMI
}

func WithOwnerReference(ownerName, ownerUID string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		if ownerUID != "" && ownerName != "" {
			vmi.ObjectMeta.OwnerReferences = append(vmi.ObjectMeta.OwnerReferences, metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "Pod",
				Name:       ownerName,
				UID:        types.UID(ownerUID),
			})
		}
	}
}

func WithoutCRIOCPULoadBalancing() Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		if vmi.ObjectMeta.Annotations == nil {
			vmi.ObjectMeta.Annotations = map[string]string{}
		}

		vmi.ObjectMeta.Annotations[CRIOCPULoadBalancingAnnotation] = Disable
	}
}

func WithoutCRIOCPUQuota() Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		if vmi.ObjectMeta.Annotations == nil {
			vmi.ObjectMeta.Annotations = map[string]string{}
		}

		vmi.ObjectMeta.Annotations[CRIOCPUQuotaAnnotation] = Disable
	}
}

func WithoutCRIOIRQLoadBalancing() Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		if vmi.ObjectMeta.Annotations == nil {
			vmi.ObjectMeta.Annotations = map[string]string{}
		}

		vmi.ObjectMeta.Annotations[CRIOIRQLoadBalancingAnnotation] = Disable
	}
}

func WithDedicatedCPU(socketsCount, coresCount, threadsCount uint32) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		vmi.Spec.Domain.CPU = &kvcorev1.CPU{
			Sockets:               socketsCount,
			Cores:                 coresCount,
			Threads:               threadsCount,
			DedicatedCPUPlacement: true,
		}
	}
}

func WithVirtIODisk(name string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		vmi.Spec.Domain.Devices.Disks = append(vmi.Spec.Domain.Devices.Disks, kvcorev1.Disk{
			Name: name,
			DiskDevice: kvcorev1.DiskDevice{
				Disk: &kvcorev1.DiskTarget{Bus: kvcorev1.DiskBusVirtio},
			},
		})
	}
}

func WithSRIOVInterface(name, macAddress, pciAddress string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		vmi.Spec.Domain.Devices.Interfaces = append(vmi.Spec.Domain.Devices.Interfaces, kvcorev1.Interface{
			Name:       name,
			Model:      "virtio",
			MacAddress: macAddress,
			PciAddress: pciAddress,
			InterfaceBindingMethod: kvcorev1.InterfaceBindingMethod{
				SRIOV: &kvcorev1.InterfaceSRIOV{},
			},
		})
	}
}

func WithNetworkInterfaceMultiQueue() Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		vmi.Spec.Domain.Devices.NetworkInterfaceMultiQueue = Pointer(true)
	}
}

func WithRandomNumberGenerator() Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		vmi.Spec.Domain.Devices.Rng = &kvcorev1.Rng{}
	}
}

func WithMemory(hugePageSize, guestMemory string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		guestMemoryQuantity := resource.MustParse(guestMemory)
		vmi.Spec.Domain.Memory = &kvcorev1.Memory{
			Hugepages: &kvcorev1.Hugepages{PageSize: hugePageSize},
			Guest:     &guestMemoryQuantity,
		}
	}
}

func WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds int64) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		vmi.Spec.TerminationGracePeriodSeconds = Pointer(terminationGracePeriodSeconds)
	}
}

func WithMultusNetwork(name, networkAttachmentDefinitionName string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		vmi.Spec.Networks = append(vmi.Spec.Networks, kvcorev1.Network{
			Name: name,
			NetworkSource: kvcorev1.NetworkSource{
				Multus: &kvcorev1.MultusNetwork{
					NetworkName: networkAttachmentDefinitionName,
				},
			},
		})
	}
}

func WithContainerDisk(volumeName, imageName string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		newVolume := kvcorev1.Volume{
			Name: volumeName,
			VolumeSource: kvcorev1.VolumeSource{
				ContainerDisk: &kvcorev1.ContainerDiskSource{
					Image:           imageName,
					ImagePullPolicy: corev1.PullAlways,
				},
			},
		}

		vmi.Spec.Volumes = append(vmi.Spec.Volumes, newVolume)
	}
}

func WithCloudInitNoCloudVolume(name, userData string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		newVolume := kvcorev1.Volume{
			Name: name,
			VolumeSource: kvcorev1.VolumeSource{
				CloudInitNoCloud: &kvcorev1.CloudInitNoCloudSource{
					UserData: userData,
				},
			},
		}

		vmi.Spec.Volumes = append(vmi.Spec.Volumes, newVolume)
	}
}

// WithLabels adds the given labels.
func WithLabels(labels map[string]string) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		if vmi.ObjectMeta.Labels == nil {
			vmi.ObjectMeta.Labels = map[string]string{}
		}

		for key, val := range labels {
			vmi.ObjectMeta.Labels[key] = val
		}
	}
}

// WithAffinity adds the given affinity.
func WithAffinity(affinity *corev1.Affinity) Option {
	return func(vmi *kvcorev1.VirtualMachineInstance) {
		if affinity != nil {
			vmi.Spec.Affinity = affinity
		}
	}
}

func Pointer[T any](v T) *T {
	return &v
}
