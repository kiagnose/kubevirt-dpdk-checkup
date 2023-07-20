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

package trex

import (
	"time"

	expect "github.com/google/goexpect"

	"kubevirt.io/client-go/kubecli"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/executor/console"
)

type vmiSerialConsoleClient interface {
	VMISerialConsole(namespace, name string, timeout time.Duration) (kubecli.StreamInterface, error)
}

const (
	shellPrompt  = "# "
	batchTimeout = 30 * time.Second
)

func StartTrexService(vmiSerialClient vmiSerialConsoleClient, namespace, vmiName string) error {
	command := "systemctl start " + SystemdUnitFileName
	_, err := console.SafeExpectBatchWithResponse(vmiSerialClient, namespace, vmiName,
		[]expect.Batcher{
			&expect.BSnd{S: command + "\n"},
			&expect.BExp{R: shellPrompt},
		},
		batchTimeout,
	)
	return err
}
