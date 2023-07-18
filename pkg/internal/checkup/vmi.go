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

package checkup

import (
	"fmt"
	"strings"

	k8scorev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	kvcorev1 "kubevirt.io/api/core/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/vmi"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
)

const (
	VMIUnderTestNamePrefix = "vmi-under-test"
	TrafficGenNamePrefix   = "dpdk-traffic-gen"
)

const DPDKCheckupUIDLabelKey = "kubevirt-dpdk-checkup/uid"

func newVMIUnderTest(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	vmiConfig := DPDKVMIConfig{
		NamePrefix:                      VMIUnderTestNamePrefix,
		OwnerName:                       checkupConfig.PodName,
		OwnerUID:                        checkupConfig.PodUID,
		Affinity:                        Affinity(checkupConfig.DPDKNodeLabelSelector, checkupConfig.PodUID),
		ContainerDiskImage:              checkupConfig.VMContainerDiskImage,
		NetworkAttachmentDefinitionName: checkupConfig.NetworkAttachmentDefinitionName,
		NICEastMACAddress:               checkupConfig.DPDKEastMacAddress.String(),
		NICEastPCIAddress:               config.VMIEastNICPCIAddress,
		NICWestMACAddress:               checkupConfig.DPDKWestMacAddress.String(),
		NICWestPCIAddress:               config.VMIWestNICPCIAddress,
		Username:                        config.VMIUsername,
		Password:                        config.VMIPassword,
	}

	return NewDPDKVMI(vmiConfig)
}

func newTrafficGen(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	vmiConfig := DPDKVMIConfig{
		NamePrefix:                      TrafficGenNamePrefix,
		OwnerName:                       checkupConfig.PodName,
		OwnerUID:                        checkupConfig.PodUID,
		Affinity:                        Affinity(checkupConfig.TrafficGeneratorNodeLabelSelector, checkupConfig.PodUID),
		ContainerDiskImage:              checkupConfig.TrafficGeneratorImage,
		NetworkAttachmentDefinitionName: checkupConfig.NetworkAttachmentDefinitionName,
		NICEastMACAddress:               checkupConfig.TrafficGeneratorEastMacAddress.String(),
		NICEastPCIAddress:               config.VMIEastNICPCIAddress,
		NICWestMACAddress:               checkupConfig.TrafficGeneratorWestMacAddress.String(),
		NICWestPCIAddress:               config.VMIWestNICPCIAddress,
		Username:                        config.VMIUsername,
		Password:                        config.VMIPassword,
	}

	return NewDPDKVMI(vmiConfig)
}

type DPDKVMIConfig struct {
	NamePrefix                      string
	OwnerName                       string
	OwnerUID                        string
	Affinity                        *k8scorev1.Affinity
	ContainerDiskImage              string
	NetworkAttachmentDefinitionName string
	NICEastMACAddress               string
	NICEastPCIAddress               string
	NICWestMACAddress               string
	NICWestPCIAddress               string
	Username                        string
	Password                        string
}

func NewDPDKVMI(vmiConfig DPDKVMIConfig) *kvcorev1.VirtualMachineInstance {
	const (
		CPUSocketsCount   = 1
		CPUCoresCount     = 4
		CPUTreadsCount    = 2
		hugePageSize      = "1Gi"
		guestMemory       = "4Gi"
		rootDiskName      = "rootdisk"
		cloudInitDiskName = "cloudinitdisk"
		eastNetworkName   = "nic-east"
		westNetworkName   = "nic-west"

		terminationGracePeriodSeconds = 0
	)

	labels := map[string]string{
		DPDKCheckupUIDLabelKey: vmiConfig.OwnerUID,
	}

	return vmi.New(RandomizeName(vmiConfig.NamePrefix),
		vmi.WithOwnerReference(vmiConfig.OwnerName, vmiConfig.OwnerUID),
		vmi.WithLabels(labels),
		vmi.WithAffinity(vmiConfig.Affinity),
		vmi.WithoutCRIOCPULoadBalancing(),
		vmi.WithoutCRIOCPUQuota(),
		vmi.WithoutCRIOIRQLoadBalancing(),
		vmi.WithDedicatedCPU(CPUSocketsCount, CPUCoresCount, CPUTreadsCount),
		vmi.WithSRIOVInterface(eastNetworkName, vmiConfig.NICEastMACAddress, vmiConfig.NICEastPCIAddress),
		vmi.WithMultusNetwork(eastNetworkName, vmiConfig.NetworkAttachmentDefinitionName),
		vmi.WithSRIOVInterface(westNetworkName, vmiConfig.NICWestMACAddress, vmiConfig.NICWestPCIAddress),
		vmi.WithMultusNetwork(westNetworkName, vmiConfig.NetworkAttachmentDefinitionName),
		vmi.WithNetworkInterfaceMultiQueue(),
		vmi.WithRandomNumberGenerator(),
		vmi.WithMemory(hugePageSize, guestMemory),
		vmi.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds),
		vmi.WithContainerDisk(rootDiskName, vmiConfig.ContainerDiskImage),
		vmi.WithVirtIODisk(rootDiskName),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, CloudInit(vmiConfig.Username, vmiConfig.Password)),
		vmi.WithVirtIODisk(cloudInitDiskName),
	)
}

func Affinity(nodeName, ownerUID string) *k8scorev1.Affinity {
	var affinity k8scorev1.Affinity
	if nodeName != "" {
		affinity.NodeAffinity = vmi.NewRequiredNodeAffinity(nodeName)
	} else {
		affinity.PodAntiAffinity = vmi.NewPreferredPodAntiAffinity(DPDKCheckupUIDLabelKey, ownerUID)
	}

	return &affinity
}

func CloudInit(username, password string) string {
	sb := strings.Builder{}
	sb.WriteString("#cloud-config\n")
	sb.WriteString(fmt.Sprintf("user: %s\n", username))
	sb.WriteString(fmt.Sprintf("password: %s\n", password))
	sb.WriteString("chpasswd:\n")
	sb.WriteString("  expire: false")

	return sb.String()
}

func RandomizeName(prefix string) string {
	const randomStringLen = 5

	return fmt.Sprintf("%s-%s", prefix, rand.String(randomStringLen))
}
