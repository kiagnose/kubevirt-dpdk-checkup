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
	"path"
	"strings"

	k8scorev1 "k8s.io/api/core/v1"
	kvcorev1 "kubevirt.io/api/core/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/trex"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/vmi"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
)

const (
	VMIUnderTestNamePrefix = "vmi-under-test"
	TrafficGenNamePrefix   = "dpdk-traffic-gen"
)

const DPDKCheckupUIDLabelKey = "kubevirt-dpdk-checkup/uid"

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

func newVMIUnderTest(name string, checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	optionsToApply := baseOptions(checkupConfig)

	optionsToApply = append(optionsToApply,
		vmi.WithAffinity(Affinity(checkupConfig.DPDKNodeLabelSelector, checkupConfig.PodUID)),
		vmi.WithSRIOVInterface(eastNetworkName, checkupConfig.DPDKEastMacAddress.String(), config.VMIEastNICPCIAddress),
		vmi.WithSRIOVInterface(westNetworkName, checkupConfig.DPDKWestMacAddress.String(), config.VMIWestNICPCIAddress),
		vmi.WithContainerDisk(rootDiskName, checkupConfig.VMContainerDiskImage),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, CloudInit(config.VMIUsername, config.VMIPassword, nil)),
	)

	return vmi.New(name, optionsToApply...)
}

func newTrafficGen(name string, checkupConfig config.Config, configMapName string) *kvcorev1.VirtualMachineInstance {
	const configDiskSerial = "DEADBEEF"
	const configVolumeName = "trex-config"

	optionsToApply := baseOptions(checkupConfig)

	optionsToApply = append(optionsToApply,
		vmi.WithAffinity(Affinity(checkupConfig.TrafficGeneratorNodeLabelSelector, checkupConfig.PodUID)),
		vmi.WithSRIOVInterface(eastNetworkName, checkupConfig.TrafficGeneratorEastMacAddress.String(), config.VMIEastNICPCIAddress),
		vmi.WithSRIOVInterface(westNetworkName, checkupConfig.TrafficGeneratorWestMacAddress.String(), config.VMIWestNICPCIAddress),
		vmi.WithContainerDisk(rootDiskName, checkupConfig.TrafficGeneratorImage),
		vmi.WithCloudInitNoCloudVolume(
			cloudInitDiskName,
			CloudInit(config.VMIUsername, config.VMIPassword, trafficGenBootCommands(configDiskSerial)),
		),
		vmi.WithConfigMapVolume(configVolumeName, configMapName),
		vmi.WithConfigMapDisk(configVolumeName, configDiskSerial),
	)

	return vmi.New(name, optionsToApply...)
}

func baseOptions(checkupConfig config.Config) []vmi.Option {
	labels := map[string]string{
		DPDKCheckupUIDLabelKey: checkupConfig.PodUID,
	}

	return []vmi.Option{
		vmi.WithOwnerReference(checkupConfig.PodName, checkupConfig.PodUID),
		vmi.WithLabels(labels),
		vmi.WithoutCRIOCPULoadBalancing(),
		vmi.WithoutCRIOCPUQuota(),
		vmi.WithoutCRIOIRQLoadBalancing(),
		vmi.WithDedicatedCPU(CPUSocketsCount, CPUCoresCount, CPUTreadsCount),
		vmi.WithMemory(hugePageSize, guestMemory),
		vmi.WithNetworkInterfaceMultiQueue(),
		vmi.WithRandomNumberGenerator(),
		vmi.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds),
		vmi.WithMultusNetwork(eastNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithMultusNetwork(westNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithVirtIODisk(rootDiskName),
		vmi.WithVirtIODisk(cloudInitDiskName),
	}
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

func CloudInit(username, password string, bootCommands []string) string {
	sb := strings.Builder{}
	sb.WriteString("#cloud-config\n")
	sb.WriteString(fmt.Sprintf("user: %s\n", username))
	sb.WriteString(fmt.Sprintf("password: %s\n", password))
	sb.WriteString("chpasswd:\n")
	sb.WriteString("  expire: false\n")

	if len(bootCommands) != 0 {
		sb.WriteString("bootcmd:\n")

		for _, command := range bootCommands {
			sb.WriteString(fmt.Sprintf("  - %q\n", command))
		}
	}

	return sb.String()
}

func trafficGenBootCommands(configDiskSerial string) []string {
	const configMountDirectory = "/mnt/app-config"
	const testScriptsDirectory = "/opt/tests"

	return []string{
		fmt.Sprintf("mkdir %s", configMountDirectory),
		fmt.Sprintf("mount /dev/$(lsblk --nodeps -no name,serial | grep %s | cut -f1 -d' ') %s", configDiskSerial, configMountDirectory),
		fmt.Sprintf("cp %s /etc/systemd/system", path.Join(configMountDirectory, trex.SystemdUnitFileName)),
		fmt.Sprintf("cp %s %s", path.Join(configMountDirectory, trex.ExecutionScriptName), trex.BinDirectory),
		fmt.Sprintf("chmod 744 %s", path.Join(trex.BinDirectory, trex.ExecutionScriptName)),
		fmt.Sprintf("cp %s /etc", path.Join(configMountDirectory, trex.CfgFileName)),
		fmt.Sprintf("mkdir -p %s", testScriptsDirectory),
		fmt.Sprintf("cp %s/*.py %s", configMountDirectory, testScriptsDirectory),
	}
}
