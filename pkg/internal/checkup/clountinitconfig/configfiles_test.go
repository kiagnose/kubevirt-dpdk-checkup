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

package clountinitconfig_test

import (
	"testing"

	assert "github.com/stretchr/testify/require"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/clountinitconfig"
)

func TestGetTrexCfgFile(t *testing.T) {
	cfgs := clountinitconfig.NewConfig()
	cfgFile := cfgs.GenerateCfgFile()

	const expectedCfgFile = `#!/bin/bash
set -ex

driverctl set-override 0000:06:00.0 vfio-pci
driverctl set-override 0000:07:00.0 vfio-pci

marker_file=/var/dpdk-checkup-tuned-adm-set-marker
if [ ! -f "$marker_file" ]; then
  # Running tuned-adm (requires manual reboot)
  echo "isolated_cores=2-7" > /etc/tuned/cpu-partitioning-variables.conf
  tuned-adm profile cpu-partitioning

  # Signaling that tuned-adm has run and that the image is ready for manual reboot
  touch "$marker_file"
  chcon -t virt_qemu_ga_exec_t "$marker_file"
fi
`
	assert.Equal(t, expectedCfgFile, cfgFile)
}
