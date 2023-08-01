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

package tests

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/client-go/kubecli"
)

func TestKubevirtDpdkCheckup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KubevirtDpdkCheckup Suite")
}

const (
	namespaceEnvVarName                    = "TEST_NAMESPACE"
	checkupImageEnvVarName                 = "TEST_CHECKUP_IMAGE"
	networkAttachmentDefinitionNameVarName = "NETWORK_ATTACHMENT_DEFINITION_NAME"
	trafficGenContainerDiskImageVarName    = "TRAFFIC_GEN_CONTAINER_DISK_IMAGE"
	vmContainerDiskImageEnvVarName         = "VM_CONTAINER_DISK_IMAGE_URL"
)

const (
	defaultNamespace                       = "kiagnose-demo"
	defaultCheckupImageName                = "quay.io/kiagnose/kubevirt-dpdk-checkup:main"
	defaultNetworkAttachmentDefinitionName = "intel-dpdk-network-1"
)

var (
	virtClient                      kubecli.KubevirtClient
	testNamespace                   string
	testCheckupImageName            string
	networkAttachmentDefinitionName string
	trafficGenContainerDiskImage    string
	vmContainerDiskImage            string
)

var _ = BeforeSuite(func() {
	var err error
	kubeconfig := os.Getenv("KUBECONFIG")
	Expect(kubeconfig).NotTo(BeEmpty(), "KUBECONFIG env var should not be empty")

	virtClient, err = kubecli.GetKubevirtClientFromFlags("", kubeconfig)
	Expect(err).NotTo(HaveOccurred())

	if testNamespace = os.Getenv(namespaceEnvVarName); testNamespace == "" {
		testNamespace = defaultNamespace
	}

	if testCheckupImageName = os.Getenv(checkupImageEnvVarName); testCheckupImageName == "" {
		testCheckupImageName = defaultCheckupImageName
	}

	if networkAttachmentDefinitionName = os.Getenv(networkAttachmentDefinitionNameVarName); networkAttachmentDefinitionName == "" {
		networkAttachmentDefinitionName = defaultNetworkAttachmentDefinitionName
	}

	trafficGenContainerDiskImage = os.Getenv(trafficGenContainerDiskImageVarName)

	vmContainerDiskImage = os.Getenv(vmContainerDiskImageEnvVarName)
})
