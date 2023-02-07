package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
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
	portStatsJSONString, err := t.runCommandWithJSONResponse(ctx, fmt.Sprintf("stats --port %d -p", port))
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
	globalStatsJSONString, err := t.runCommandWithJSONResponse(ctx, "stats -g")
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

func (t trexConsole) MonitorDropRates(ctx context.Context, duration time.Duration) (float64, error) {
	const interval = 10 * time.Second
	log.Printf("Monitoring traffic generator side drop rates every %ss during the test duration...", interval)
	maxDropRateBps := float64(0)

	ctxWithNewDeadline, cancel := context.WithDeadline(ctx, time.Now().Add(duration))
	defer cancel()
	conditionFn := func(ctx context.Context) (bool, error) {
		statsGlobal, err := t.GetGlobalStats(ctx)
		if statsGlobal.Result.MRxDropBps > maxDropRateBps {
			maxDropRateBps = statsGlobal.Result.MRxDropBps
		}
		return false, err
	}
	if err := wait.PollImmediateUntilWithContext(ctxWithNewDeadline, interval, conditionFn); err != nil {
		if errors.Is(err, wait.ErrWaitTimeout) {
			log.Printf("finished polling for drop rates")
		} else {
			return 0, fmt.Errorf("failed to poll global stats in trex-console: %w", err)
		}
	}

	return maxDropRateBps, nil
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
	var (
		err            error
		stdout, stderr string
	)

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

func (t trexConsole) runCommandWithJSONResponse(ctx context.Context, command string) (string, error) {
	var (
		err            error
		stdout, stderr string
	)

	const verboseOn = "verbose on;"
	command = verboseOn + command
	if stdout, stderr, err = t.podClient.ExecuteCommandOnPod(ctx, t.namespace, t.name, t.containerName,
		[]string{
			"/bin/sh",
			"-c",
			fmt.Sprintf("echo %q | ./trex-console -q", command),
		}); err != nil {
		return "", fmt.Errorf("failed to get pod stats \"%s/%s\": err %w, stderr: %s", t.namespace, t.name, err, stderr)
	}

	return extractJSONString(stdout)
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
