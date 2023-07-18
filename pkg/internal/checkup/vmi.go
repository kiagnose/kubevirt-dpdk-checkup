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
	kvcorev1 "kubevirt.io/api/core/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/vmi"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
)

const (
	VMIUnderTestNamePrefix = "vmi-under-test"
	TrafficGenNamePrefix   = "dpdk-traffic-gen"
)

func newVMIUnderTest(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	vmiConfig := vmi.DPDKVMIConfig{
		NamePrefix:                      VMIUnderTestNamePrefix,
		OwnerName:                       checkupConfig.PodName,
		OwnerUID:                        checkupConfig.PodUID,
		Affinity:                        vmi.Affinity(checkupConfig.DPDKNodeLabelSelector, checkupConfig.PodUID),
		ContainerDiskImage:              checkupConfig.VMContainerDiskImage,
		NetworkAttachmentDefinitionName: checkupConfig.NetworkAttachmentDefinitionName,
		NICEastMACAddress:               checkupConfig.DPDKEastMacAddress.String(),
		NICEastPCIAddress:               config.VMIEastNICPCIAddress,
		NICWestMACAddress:               checkupConfig.DPDKWestMacAddress.String(),
		NICWestPCIAddress:               config.VMIWestNICPCIAddress,
		Username:                        config.VMIUsername,
		Password:                        config.VMIPassword,
	}

	return vmi.NewDPDKVMI(vmiConfig)
}

func newTrafficGen(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	vmiConfig := vmi.DPDKVMIConfig{
		NamePrefix:                      TrafficGenNamePrefix,
		OwnerName:                       checkupConfig.PodName,
		OwnerUID:                        checkupConfig.PodUID,
		Affinity:                        vmi.Affinity(checkupConfig.TrafficGeneratorNodeLabelSelector, checkupConfig.PodUID),
		ContainerDiskImage:              checkupConfig.TrafficGeneratorImage,
		NetworkAttachmentDefinitionName: checkupConfig.NetworkAttachmentDefinitionName,
		NICEastMACAddress:               checkupConfig.TrafficGeneratorEastMacAddress.String(),
		NICEastPCIAddress:               config.VMIEastNICPCIAddress,
		NICWestMACAddress:               checkupConfig.TrafficGeneratorWestMacAddress.String(),
		NICWestPCIAddress:               config.VMIWestNICPCIAddress,
		Username:                        config.VMIUsername,
		Password:                        config.VMIPassword,
	}

	return vmi.NewDPDKVMI(vmiConfig)
}
