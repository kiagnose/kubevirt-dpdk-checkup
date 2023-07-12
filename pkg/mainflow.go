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

package pkg

import (
	"context"
	"fmt"
	"log"

	kconfig "github.com/kiagnose/kiagnose/kiagnose/config"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/executor"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/client"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/launcher"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/reporter"
)

func Run(rawEnv map[string]string, namespace string) error {
	c, err := client.New()
	if err != nil {
		return err
	}

	baseConfig, err := kconfig.Read(c, rawEnv)
	if err != nil {
		return err
	}

	cfg, err := config.New(baseConfig)
	if err != nil {
		return err
	}

	printConfig(cfg)

	dpdkCheckupExecutor := executor.New(c, namespace, cfg)
	l := launcher.New(
		checkup.New(c, namespace, cfg, dpdkCheckupExecutor),
		reporter.New(c, baseConfig.ConfigMapNamespace, baseConfig.ConfigMapName),
	)

	ctx, cancel := context.WithTimeout(context.Background(), baseConfig.Timeout)
	defer cancel()

	return l.Run(ctx)
}

func printConfig(checkupConfig config.Config) {
	log.Println("Using the following config:")
	log.Printf("%q: %q", config.NetworkAttachmentDefinitionNameParamName, checkupConfig.NetworkAttachmentDefinitionName)
	log.Printf("%q: %q", config.TrafficGeneratorImageParamName, checkupConfig.TrafficGeneratorImage)
	log.Printf("%q: %q", config.TrafficGeneratorNodeLabelSelectorParamName, checkupConfig.TrafficGeneratorNodeLabelSelector)
	log.Printf("%q: %q", config.TrafficGeneratorPacketsPerSecondParamName, checkupConfig.TrafficGeneratorPacketsPerSecond)
	log.Printf("%q: %q", config.PortBandwidthGBParamName, fmt.Sprintf("%d", checkupConfig.PortBandwidthGB))
	log.Printf("%q: %q", config.DPDKNodeLabelSelectorParamName, checkupConfig.DPDKNodeLabelSelector)
	log.Printf("%q: %q", config.TrafficGeneratorEastMacAddressParamName, checkupConfig.TrafficGeneratorEastMacAddress)
	log.Printf("%q: %q", config.TrafficGeneratorWestMacAddressParamName, checkupConfig.TrafficGeneratorWestMacAddress)
	log.Printf("%q: %q", config.DPDKEastMacAddressParamName, checkupConfig.DPDKEastMacAddress)
	log.Printf("%q: %q", config.DPDKWestMacAddressParamName, checkupConfig.DPDKWestMacAddress)
	log.Printf("%q: %q", config.VMContainerDiskImageParamName, checkupConfig.VMContainerDiskImage)
	log.Printf("%q: %q", config.TestDurationParamName, checkupConfig.TestDuration)
	log.Printf("%q: %t", config.VerboseParamName, checkupConfig.Verbose)
}
