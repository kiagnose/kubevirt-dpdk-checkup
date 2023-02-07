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

package client

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	kvcorev1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

type Client struct {
	kubecli.KubevirtClient
	config *rest.Config
}

type resultWrapper struct {
	vmi *kvcorev1.VirtualMachineInstance
	err error
}

type executeWrapper struct {
	stdout string
	stderr string
	err    error
}

func New() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	client, err := kubecli.GetKubevirtClientFromRESTConfig(config)
	if err != nil {
		return nil, err
	}

	return &Client{client, config}, nil
}

func (c *Client) CreatePod(ctx context.Context, namespace string, pod *corev1.Pod) (*corev1.Pod, error) {
	return c.KubevirtClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
}

func (c *Client) GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error) {
	return c.KubevirtClient.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
}

func (c *Client) DeletePod(ctx context.Context, namespace, name string) error {
	return c.KubevirtClient.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (c *Client) GetPodLogsByLabel(ctx context.Context, namespace, labelSelector string) (string, error) {
	podList, err := c.KubevirtClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return "", err
	}

	logs := strings.Builder{}
	for podIdx := range podList.Items {
		rawLogs, err := c.KubevirtClient.CoreV1().Pods(namespace).GetLogs(podList.Items[podIdx].Name,
			&corev1.PodLogOptions{}).DoRaw(context.Background())
		if err != nil {
			return "", err
		}
		logs.WriteString(fmt.Sprintf("Pod %s/%s Logs:\n%s\n", podList.Items[podIdx].Namespace, podList.Items[podIdx].Name, string(rawLogs)))
	}
	return logs.String(), nil
}

func (c *Client) CreateVirtualMachineInstance(ctx context.Context,
	namespace string,
	vmi *kvcorev1.VirtualMachineInstance) (*kvcorev1.VirtualMachineInstance, error) {
	resultCh := make(chan resultWrapper, 1)

	go func() {
		createdVMI, err := c.KubevirtClient.VirtualMachineInstance(namespace).Create(vmi)
		resultCh <- resultWrapper{createdVMI, err}
	}()

	select {
	case result := <-resultCh:
		return result.vmi, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) GetVirtualMachineInstance(ctx context.Context, namespace, name string) (*kvcorev1.VirtualMachineInstance, error) {
	resultCh := make(chan resultWrapper, 1)

	go func() {
		vmi, err := c.KubevirtClient.VirtualMachineInstance(namespace).Get(name, &metav1.GetOptions{})
		resultCh <- resultWrapper{vmi, err}
	}()

	select {
	case result := <-resultCh:
		return result.vmi, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) DeleteVirtualMachineInstance(ctx context.Context, namespace, name string) error {
	resultCh := make(chan error, 1)

	go func() {
		err := c.KubevirtClient.VirtualMachineInstance(namespace).Delete(name, &metav1.DeleteOptions{})
		resultCh <- err
	}()

	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) VMISerialConsole(namespace, name string, timeout time.Duration) (kubecli.StreamInterface, error) {
	return c.KubevirtClient.VirtualMachineInstance(namespace).SerialConsole(
		name,
		&kubecli.SerialConsoleOptions{ConnectionTimeout: timeout},
	)
}

func (c *Client) ExecuteCommandOnPod(ctx context.Context,
	namespace, name, containerName string,
	command []string) (stdout, stderr string, err error) {
	resultCh := make(chan executeWrapper, 1)

	go func() {
		var (
			stdoutBuf bytes.Buffer
			stderrBuf bytes.Buffer
		)
		options := remotecommand.StreamOptions{
			Stdout: &stdoutBuf,
			Stderr: &stderrBuf,
			Tty:    false,
		}

		err = executeCommandOnPodWithOptions(c.KubevirtClient, c.config, namespace, name, containerName, command, options)
		stdout = stdoutBuf.String()
		stderr = stderrBuf.String()
		resultCh <- executeWrapper{stdout, stderr, err}
	}()

	select {
	case result := <-resultCh:
		return result.stdout, result.stderr, result.err
	case <-ctx.Done():
		return stdout, stderr, ctx.Err()
	}
}

func executeCommandOnPodWithOptions(virtCli kubecli.KubevirtClient, clientConfig *rest.Config,
	namespace, name, containerName string,
	command []string,
	options remotecommand.StreamOptions) error {
	req := virtCli.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(name).
		Namespace(namespace).
		SubResource("exec").
		Param("container", containerName)

	req.VersionedParams(&corev1.PodExecOptions{
		Container: containerName,
		Command:   command,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(clientConfig, "POST", req.URL())
	if err != nil {
		return err
	}

	return executor.Stream(options)
}
