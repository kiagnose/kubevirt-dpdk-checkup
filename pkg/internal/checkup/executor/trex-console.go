package executor

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type trexConsole struct {
	podClient     podExecuteClient
	namespace     string
	name          string
	containerName string
}

func NewTrexConsole(client podExecuteClient, namespace, name, containerName string) trexConsole {
	return trexConsole{
		podClient:     client,
		namespace:     namespace,
		name:          name,
		containerName: containerName,
	}
}

func (t trexConsole) GetPortStats(ctx context.Context, port int) (string, error) {
	return t.runCommand(ctx, fmt.Sprintf("stats --port %d -p", port))
}

func (t trexConsole) GetGlobalStats(ctx context.Context) (string, error) {
	return t.runCommand(ctx, "stats -g")
}

func (t trexConsole) ClearStats(ctx context.Context) (string, error) {
	return t.runCommand(ctx, "clear")
}

func (t trexConsole) StartTraffic(ctx context.Context, packetPerSecondMillion, port int, testDuration time.Duration) (string, error) {
	testDurationMinutes := int(testDuration.Minutes())
	return t.runCommand(ctx, fmt.Sprintf("start -f /opt/tests/testpmd.py -m %dmpps -p %d -d %dm",
		packetPerSecondMillion, port, testDurationMinutes))
}

func (t trexConsole) StopTraffic(ctx context.Context) (string, error) {
	return t.runCommand(ctx, "stop -a")
}

func (t trexConsole) runCommand(ctx context.Context, command string) (string, error) {
	var err error
	var stdout, stderr string

	if stdout, stderr, err = t.podClient.ExecuteCommandOnPod(ctx, t.namespace, t.name, t.containerName,
		[]string{
			"/bin/sh",
			"-c",
			fmt.Sprintf("echo %q | ./trex-console -q", command),
		}); err != nil {
		return "", fmt.Errorf("failed to get pod stats \"%s/%s\": err %w, stderr: %s", t.namespace, t.name, err, stderr)
	}

	return cleanStdout(stdout), nil
}

func cleanStdout(rawStdout string) string {
	stdout := strings.Replace(rawStdout, "Using 'python3' as Python interpeter", "", -1)
	stdout = strings.Replace(stdout, "-=TRex Console v3.0=-", "", -1)
	stdout = strings.Replace(stdout, "Type 'help' or '?' for supported actions", "", -1)
	stdout = strings.Replace(stdout, "trex>Global Statistitcs", "", -1)
	stdout = strings.Replace(stdout, "trex>", "", -1)

	return stdout
}
