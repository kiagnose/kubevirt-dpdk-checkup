package executor

import (
	"context"
	"encoding/json"
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

func (t trexConsole) GetPortStats(ctx context.Context, port int) (portStats, error) {
	portStatsJSONString, err := t.runCommand(ctx, fmt.Sprintf("stats --port %d -p", port), true)
	if err != nil {
		return portStats{}, fmt.Errorf("failed to get global stats json: %w", err)
	}

	var ps portStats
	err = json.Unmarshal([]byte(portStatsJSONString), &ps)
	if err != nil {
		return portStats{}, fmt.Errorf("failed to unmarshal port %d stats json: %w", port, err)
	}
	return ps, nil
}

func (t trexConsole) GetGlobalStats(ctx context.Context) (globalStats, error) {
	globalStatsJSONString, err := t.runCommand(ctx, "stats -g", true)
	if err != nil {
		return globalStats{}, fmt.Errorf("failed to get global stats json: %w", err)
	}

	var gs globalStats
	err = json.Unmarshal([]byte(globalStatsJSONString), &gs)
	if err != nil {
		return globalStats{}, fmt.Errorf("failed to unmarshal global stats json: %w", err)
	}
	return gs, nil
}

func (t trexConsole) ClearStats(ctx context.Context) (string, error) {
	return t.runCommand(ctx, "clear", false)
}

func (t trexConsole) StartTraffic(ctx context.Context, packetPerSecondMillion, port int, testDuration time.Duration) (string, error) {
	testDurationMinutes := int(testDuration.Minutes())
	return t.runCommand(ctx, fmt.Sprintf("start -f /opt/tests/testpmd.py -m %dmpps -p %d -d %dm",
		packetPerSecondMillion, port, testDurationMinutes), false)
}

func (t trexConsole) StopTraffic(ctx context.Context) (string, error) {
	return t.runCommand(ctx, "stop -a", false)
}

func (t trexConsole) runCommand(ctx context.Context, command string, returnJSONString bool) (string, error) {
	var err error
	var stdout, stderr string

	if returnJSONString {
		const verboseString = "verbose on;"
		command = verboseString + command
	}
	if stdout, stderr, err = t.podClient.ExecuteCommandOnPod(ctx, t.namespace, t.name, t.containerName,
		[]string{
			"/bin/sh",
			"-c",
			fmt.Sprintf("echo %q | ./trex-console -q", command),
		}); err != nil {
		return "", fmt.Errorf("failed to get pod stats \"%s/%s\": err %w, stderr: %s", t.namespace, t.name, err, stderr)
	}

	if returnJSONString {
		return extractJSONString(stdout)
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

func extractJSONString(input string) (string, error) {
	const responseStart = "Server Response:\n\n"
	const responseEnd = "\n\n"

	startIndex := strings.Index(input, responseStart) + len(responseStart)
	if startIndex == -1 {
		return "", fmt.Errorf("could not find start of JSON string")
	}

	endIndex := strings.Index(input[startIndex:], responseEnd) + startIndex
	if endIndex == -1 {
		return "", fmt.Errorf("could not find end of JSON string")
	}

	return input[startIndex:endIndex], nil
}
