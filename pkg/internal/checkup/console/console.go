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

package console

import (
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"time"

	expect "github.com/google/goexpect"

	"kubevirt.io/client-go/kubecli"
)

type vmiSerialConsoleClient interface {
	VMISerialConsole(namespace, name string, timeout time.Duration) (kubecli.StreamInterface, error)
}

const (
	PromptExpression = `(\$ |\# )`
	CRLF             = "\r\n"
)

// NewExpecter will connect to an already logged in VMI console and return the generated expecter it will wait `timeout` for the connection.
func NewExpecter(serialConsoleClient vmiSerialConsoleClient,
	vmiNamespace,
	vmiName string,
	timeout time.Duration,
	opts ...expect.Option) (expect.Expecter, <-chan error, error) {
	vmiReader, vmiWriter := io.Pipe()
	expecterReader, expecterWriter := io.Pipe()
	resCh := make(chan error)

	startTime := time.Now()
	con, err := serialConsoleClient.VMISerialConsole(vmiNamespace, vmiName, timeout)
	if err != nil {
		return nil, nil, err
	}
	timeout -= time.Since(startTime)

	go func() {
		resCh <- con.Stream(kubecli.StreamOptions{
			In:  vmiReader,
			Out: expecterWriter,
		})
	}()

	opts = append(opts, expect.SendTimeout(timeout), expect.Verbose(true))
	return expect.SpawnGeneric(&expect.GenOptions{
		In:  vmiWriter,
		Out: expecterReader,
		Wait: func() error {
			return <-resCh
		},
		Close: func() error {
			expecterWriter.Close()
			vmiReader.Close()
			return nil
		},
		Check: func() bool { return true },
	}, timeout, opts...)
}

func RetValue(retcode string) string {
	return "\n" + retcode + CRLF + ".*" + PromptExpression
}

// SafeExpectBatchWithResponse runs the batch from `expected`, connecting to a VMI's console and
// waiting for the batch to return with a response until timeout.
// It validates that the commands arrive to the console.
// NOTE: This functions inherits limitations from `ExpectBatchWithValidatedSend`, refer to it for more information.
func SafeExpectBatchWithResponse(serialConsoleClient vmiSerialConsoleClient,
	vmiNamespace,
	vmiName string,
	expected []expect.Batcher,
	timeout time.Duration) ([]expect.BatchRes, error) {
	expecter, _, err := NewExpecter(serialConsoleClient, vmiNamespace, vmiName, timeout)
	if err != nil {
		return nil, err
	}
	defer expecter.Close()

	resp, err := ExpectBatchWithValidatedSend(expecter, expected, timeout)
	if err != nil {
		log.Printf("%v", resp)
	}
	return resp, err
}

// ExpectBatchWithValidatedSend adds the expect.BSnd command to the exect.BExp expression.
// It is done to make sure the match was found in the result of the expect.BSnd
// command and not in a leftover that wasn't removed from the buffer.
// NOTE: the method contains the following limitations:
//   - Use of `BatchSwitchCase`
//   - Multiline commands
//   - No more than one sequential send or receive
func ExpectBatchWithValidatedSend(expecter expect.Expecter, batch []expect.Batcher, timeout time.Duration) ([]expect.BatchRes, error) {
	sendFlag := false
	expectFlag := false
	previousSend := ""

	const minimumRequiredBatches = 2
	if len(batch) < minimumRequiredBatches {
		return nil, fmt.Errorf("ExpectBatchWithValidatedSend requires at least 2 batchers, supplied %v", batch)
	}

	for i, batcher := range batch {
		switch batcher.Cmd() {
		case expect.BatchExpect:
			if expectFlag {
				return nil, fmt.Errorf("two sequential expect.BExp are not allowed")
			}
			expectFlag = true
			sendFlag = false
			if _, ok := batch[i].(*expect.BExp); !ok {
				return nil, fmt.Errorf("ExpectBatchWithValidatedSend support only expect of type BExp")
			}
			bExp, _ := batch[i].(*expect.BExp)
			previousSend = regexp.QuoteMeta(previousSend)

			// Remove the \n since it is translated by the console to \r\n.
			previousSend = strings.TrimSuffix(previousSend, "\n")
			bExp.R = fmt.Sprintf("%s%s%s", previousSend, "((?s).*)", bExp.R)
		case expect.BatchSend:
			if sendFlag {
				return nil, fmt.Errorf("two sequential expect.BSend are not allowed")
			}
			sendFlag = true
			expectFlag = false
			previousSend = batcher.Arg()
		case expect.BatchSwitchCase:
			return nil, fmt.Errorf("ExpectBatchWithValidatedSend doesn't support BatchSwitchCase")
		default:
			return nil, fmt.Errorf("unknown command: ExpectBatchWithValidatedSend supports only BatchExpect and BatchSend")
		}
	}

	res, err := expecter.ExpectBatch(batch, timeout)
	return res, err
}

func configureConsole(expecter expect.Expecter, shouldSudo bool) error {
	sudoString := ""
	if shouldSudo {
		sudoString = "sudo "
	}
	batch := []expect.Batcher{
		&expect.BSnd{S: "stty cols 500 rows 500\n"},
		&expect.BExp{R: PromptExpression},
		&expect.BSnd{S: "echo $?\n"},
		&expect.BExp{R: RetValue("0")},
		&expect.BSnd{S: fmt.Sprintf("%sdmesg -n 1\n", sudoString)},
		&expect.BExp{R: PromptExpression},
		&expect.BSnd{S: "echo $?\n"},
		&expect.BExp{R: RetValue("0")},
	}
	const configureConsoleTimeout = 30 * time.Second
	resp, err := expecter.ExpectBatch(batch, configureConsoleTimeout)
	if err != nil {
		log.Printf("%v", resp)
	}
	return err
}
