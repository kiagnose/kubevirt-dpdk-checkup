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

func newVMIUnderTest(name string, checkupConfig config.Config, configMapName string) *kvcorev1.VirtualMachineInstance {
	const (
		configDiskSerial = "DEADBEEF"
		configVolumeName = "vmi-under-test-config"
	)

	optionsToApply := baseOptions(checkupConfig)

	optionsToApply = append(optionsToApply,
		vmi.WithAffinity(Affinity(checkupConfig.VMUnderTestTargetNodeName, checkupConfig.PodUID)),
		vmi.WithSRIOVInterface(eastNetworkName, checkupConfig.VMUnderTestEastMacAddress.String(), config.VMIEastNICPCIAddress),
		vmi.WithSRIOVInterface(westNetworkName, checkupConfig.VMUnderTestWestMacAddress.String(), config.VMIWestNICPCIAddress),
		vmi.WithContainerDisk(rootDiskName, checkupConfig.VMUnderTestContainerDiskImage),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, CloudInit(vmiUnderTestBootCommands(configDiskSerial))),
		vmi.WithConfigMapVolume(configVolumeName, configMapName),
		vmi.WithConfigMapDisk(configVolumeName, configDiskSerial),
	)

	return vmi.New(name, optionsToApply...)
}

func newTrafficGen(name string, checkupConfig config.Config, configMapName string) *kvcorev1.VirtualMachineInstance {
	const configDiskSerial = "DEADBEEF"
	const configVolumeName = "trex-config"

	optionsToApply := baseOptions(checkupConfig)

	optionsToApply = append(optionsToApply,
		vmi.WithAffinity(Affinity(checkupConfig.TrafficGenTargetNodeName, checkupConfig.PodUID)),
		vmi.WithSRIOVInterface(eastNetworkName, checkupConfig.TrafficGenEastMacAddress.String(), config.VMIEastNICPCIAddress),
		vmi.WithSRIOVInterface(westNetworkName, checkupConfig.TrafficGenWestMacAddress.String(), config.VMIWestNICPCIAddress),
		vmi.WithContainerDisk(rootDiskName, checkupConfig.TrafficGenContainerDiskImage),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, CloudInit(trafficGenBootCommands(configDiskSerial))),
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

func generateBootScript() string {
	const isolatedCores = "2-7"
	sb := strings.Builder{}

	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("set -x\n")
	sb.WriteString("\n")
	sb.WriteString("checkup_tuned_adm_set_marker_full_path=" + config.BootScriptTunedAdmSetMarkerFileFullPath + "\n")
	sb.WriteString("\n")
	sb.WriteString("if [ ! -f \"$checkup_tuned_adm_set_marker_full_path\" ]; then\n")
	sb.WriteString("  echo \"isolated_cores=" + isolatedCores + "\" > /etc/tuned/cpu-partitioning-variables.conf\n")
	sb.WriteString("  tuned-adm profile cpu-partitioning\n\n")
	sb.WriteString("  touch $checkup_tuned_adm_set_marker_full_path\n")
	sb.WriteString("  reboot\n")
	sb.WriteString("fi\n")
	sb.WriteString("\n")
	sb.WriteString("driverctl set-override " + config.VMIEastNICPCIAddress + " vfio-pci\n")
	sb.WriteString("driverctl set-override " + config.VMIWestNICPCIAddress + " vfio-pci\n")

	return sb.String()
}

func CloudInit(bootCommands []string) string {
	sb := strings.Builder{}
	sb.WriteString("#cloud-config\n")

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

	return []string{
		fmt.Sprintf("mkdir %s", configMountDirectory),
		fmt.Sprintf("mount /dev/$(lsblk --nodeps -no name,serial | grep %s | cut -f1 -d' ') %s", configDiskSerial, configMountDirectory),
		fmt.Sprintf("cp %s /etc/systemd/system", path.Join(configMountDirectory, trex.SystemdUnitFileName)),
		fmt.Sprintf("cp %s %s", path.Join(configMountDirectory, trex.ExecutionScriptName), trex.BinDirectory),
		fmt.Sprintf("chmod 744 %s", path.Join(trex.BinDirectory, trex.ExecutionScriptName)),
		fmt.Sprintf("cp %s /etc", path.Join(configMountDirectory, trex.CfgFileName)),
		fmt.Sprintf("mkdir -p %s", trex.StreamsPyPath),
		fmt.Sprintf("cp %s/*.py %s", configMountDirectory, trex.StreamsPyPath),
		fmt.Sprintf("cp %s %s", path.Join(configMountDirectory, config.BootScriptName), config.BootScriptBinDirectory),
		fmt.Sprintf("chmod 744 %s", path.Join(config.BootScriptBinDirectory, config.BootScriptName)),
		path.Join(config.BootScriptBinDirectory, config.BootScriptName),
	}
}

func vmiUnderTestBootCommands(configDiskSerial string) []string {
	const configMountDirectory = "/mnt/app-config"

	return []string{
		fmt.Sprintf("mkdir %s", configMountDirectory),
		fmt.Sprintf("mount /dev/$(lsblk --nodeps -no name,serial | grep %s | cut -f1 -d' ') %s", configDiskSerial, configMountDirectory),
		fmt.Sprintf("cp %s %s", path.Join(configMountDirectory, config.BootScriptName), config.BootScriptBinDirectory),
		fmt.Sprintf("chmod 744 %s", path.Join(config.BootScriptBinDirectory, config.BootScriptName)),
		path.Join(config.BootScriptBinDirectory, config.BootScriptName),
	}
}
