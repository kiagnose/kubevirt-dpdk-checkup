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
	"context"
	"fmt"

	k8srand "k8s.io/apimachinery/pkg/util/rand"

	kvcorev1 "kubevirt.io/api/core/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/vmi"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

type kubeVirtVMIClient interface {
	CreateVirtualMachineInstance(ctx context.Context,
		namespace string,
		vmi *kvcorev1.VirtualMachineInstance) (*kvcorev1.VirtualMachineInstance, error)
}

type Checkup struct {
	client    kubeVirtVMIClient
	namespace string
	vmi       *kvcorev1.VirtualMachineInstance
}

const VMINamePrefix = "dpdk-vmi"

func New(client kubeVirtVMIClient, namespace string, checkupConfig config.Config) *Checkup {
	return &Checkup{
		client:    client,
		namespace: namespace,
		vmi:       newDPDKVMI(checkupConfig),
	}
}

func (c *Checkup) Setup(ctx context.Context) error {
	createdVMI, err := c.client.CreateVirtualMachineInstance(ctx, c.namespace, c.vmi)
	if err != nil {
		return err
	}
	c.vmi = createdVMI

	return nil
}

func (c *Checkup) Run(ctx context.Context) error {
	return nil
}

func (c *Checkup) Teardown(ctx context.Context) error {
	return nil
}

func (c *Checkup) Results() status.Results {
	return status.Results{}
}

func newDPDKVMI(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	const (
		CPUSocketsCount   = 1
		CPUCoresCount     = 4
		CPUTreadsCount    = 2
		rootDiskName      = "rootdisk"
		cloudInitDiskName = "cloudinitdisk"
		userData          = `#cloud-config
user: cloud-user
password: 0tli-pxem-xknu
chpasswd:
  expire: false`

		eastNetworkName   = "nic-east"
		eastNICPCIAddress = "0000:06:00.0"
		westNetworkName   = "nic-west"
		westNICPCIAddress = "0000:07:00.0"

		terminationGracePeriodSeconds = 180
	)

	return vmi.New(randomizeName(VMINamePrefix),
		vmi.WithoutCRIOCPULoadBalancing(),
		vmi.WithoutCRIOCPUQuota(),
		vmi.WithoutCRIOIRQLoadBalancing(),
		vmi.WithDedicatedCPU(CPUSocketsCount, CPUCoresCount, CPUTreadsCount),
		vmi.WithSRIOVInterface(eastNetworkName, checkupConfig.DPDKEastMacAddress.String(), eastNICPCIAddress),
		vmi.WithMultusNetwork(eastNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithSRIOVInterface(westNetworkName, checkupConfig.DPDKWestMacAddress.String(), westNICPCIAddress),
		vmi.WithMultusNetwork(westNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithNetworkInterfaceMultiQueue(),
		vmi.WithRandomNumberGenerator(),
		vmi.WithHugePages(),
		vmi.WithMemoryRequest("8Gi"),
		vmi.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds),
		vmi.WithNodeSelector(checkupConfig.DPDKNodeLabelSelector),
		vmi.WithPVCVolume(rootDiskName, "rhel8-yummy-gorilla"),
		vmi.WithVirtIODisk(rootDiskName),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, userData),
		vmi.WithVirtIODisk(cloudInitDiskName),
	)
}

func randomizeName(prefix string) string {
	const randomStringLen = 5

	return fmt.Sprintf("%s-%s", prefix, k8srand.String(randomStringLen))
}
