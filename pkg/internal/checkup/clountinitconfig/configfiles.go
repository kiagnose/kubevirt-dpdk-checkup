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
 * Copyright 2024 Red Hat, Inc.
 *
 */

package clountinitconfig

import (
	"fmt"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
)

const (
	CfgScriptName = "dpdk-checkup-boot.sh"
	BinDirectory  = "/usr/bin/"
)

type Config struct {
	isolatedCores string
}

func NewConfig() Config {
	const isolatedCores = "2-7"

	return Config{
		isolatedCores: isolatedCores,
	}
}

func (c Config) GenerateCfgFile() string {
	const cloudInitScriptTemplate = `#!/bin/bash
set -ex

driverctl set-override %s vfio-pci
driverctl set-override %s vfio-pci

marker_file=%s
if [ ! -f "$marker_file" ]; then
  # Running tuned-adm (requires manual reboot)
  echo "isolated_cores=%s" > /etc/tuned/cpu-partitioning-variables.conf
  tuned-adm profile cpu-partitioning

  # Signaling that tuned-adm has run and that the image is ready for manual reboot
  touch "$marker_file"
  chcon -t virt_qemu_ga_exec_t "$marker_file"
fi
`
	return fmt.Sprintf(cloudInitScriptTemplate,
		config.VMIEastNICPCIAddress,
		config.VMIWestNICPCIAddress,
		config.TunedAdmSetMarkerFileFullPath,
		c.isolatedCores,
	)
}
