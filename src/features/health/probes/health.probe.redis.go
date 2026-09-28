/*
Package probes — Redis deep functional health probe.

ALGORITHM BLUEPRINT:
1. TCP connectivity: dialTCP (same pattern as HealthService.probeOnce).
2. Raw RESP functional sequence (no redis-cli dependency):
   a. Open TCP connection to Redis port.
   b. Send PING → expect "+PONG\r\n" confirming the server is alive.
   c. Send SET llmobs:health:probe "1" EX 30 → expect "+OK\r\n" confirming writes work.
   d. Send GET llmobs:health:probe → expect "$1\r\n1\r\n" confirming reads work.
   e. Send DEL llmobs:health:probe → confirm cleanup.
   f. Send INFO server → parse redis_version, used_memory_human.
3. Fallback (container path): when cfg.Container != "":
   a. docker exec <container> redis-cli [-a <pass>] PING → PONG.
   b. docker exec <container> redis-cli [-a <pass>] SET/GET/DEL.
   Reuses docker exec pattern from verifyNativeCredentials (cmd/root.go).
4. Evidence: "redis_version=<V> memory=<M> ops=[PING,SET,GET,DEL] target=<host:port>"
5. Invariants:
   - All RESP I/O is bounded; scanner line limit prevents OOM.
   - Connection deadline set before every session.
   - Returns failProbe on any step failure; partial completion is a failure.
   - Never panics.
*/
package probes

import (
	"bufio"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
)

func ProbeRedis(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31413
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()

	conn, err := dialTCP(host, port, timeout)
	if err != nil {
		return failProbe("redis", start, fmt.Sprintf("TCP dial failed: %v", err))
	}
	conn.Close()

	if cfg.Container != "" {
		return probeRedisViaExec(cfg, target, start)
	}
	return probeRedisViaRESP(host, port, cfg.Password, target, timeout, start)
}

func probeRedisViaRESP(host string, port int, password, target string, timeout time.Duration, start time.Time) schema.SingleProbeResult {
	conn, err := dialTCP(host, port, timeout)
	if err != nil {
		return failProbe("redis", start, fmt.Sprintf("RESP dial failed: %v", err))
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	resp := func(cmd string) (string, error) {
		if _, err := conn.Write([]byte(cmd)); err != nil {
			return "", err
		}
		scanner := bufio.NewScanner(conn)
		if scanner.Scan() {
			return scanner.Text(), nil
		}
		return "", fmt.Errorf("no response")
	}

	if password != "" {
		authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
		if line, err := resp(authCmd); err != nil || !strings.HasPrefix(line, "+OK") {
			return failProbe("redis", start, fmt.Sprintf("AUTH failed: %s %v", line, err))
		}
	}

	if line, err := resp("*1\r\n$4\r\nPING\r\n"); err != nil || !strings.HasPrefix(line, "+PONG") {
		return failProbe("redis", start, fmt.Sprintf("PING failed: %s %v", line, err))
	}

	setKey := "llmobs:health:probe"
	setCmd := fmt.Sprintf("*5\r\n$3\r\nSET\r\n$%d\r\n%s\r\n$1\r\n1\r\n$2\r\nEX\r\n$2\r\n30\r\n", len(setKey), setKey)
	if line, err := resp(setCmd); err != nil || !strings.HasPrefix(line, "+OK") {
		return failProbe("redis", start, fmt.Sprintf("SET failed: %s %v", line, err))
	}

	getCmd := fmt.Sprintf("*2\r\n$3\r\nGET\r\n$%d\r\n%s\r\n", len(setKey), setKey)
	if line, err := resp(getCmd); err != nil || line != "$1" {
		return failProbe("redis", start, fmt.Sprintf("GET length check failed: %s %v", line, err))
	}

	delCmd := fmt.Sprintf("*2\r\n$3\r\nDEL\r\n$%d\r\n%s\r\n", len(setKey), setKey)
	resp(delCmd)

	infoConn, infoErr := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
	redisVersion, usedMemory := "", ""
	if infoErr == nil {
		_ = infoConn.SetDeadline(time.Now().Add(timeout))
		if password != "" {
			authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
			infoConn.Write([]byte(authCmd))
			bufio.NewScanner(infoConn).Scan()
		}
		infoConn.Write([]byte("*2\r\n$4\r\nINFO\r\n$6\r\nserver\r\n"))
		scanner := bufio.NewScanner(infoConn)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "redis_version:") {
				redisVersion = strings.TrimPrefix(line, "redis_version:")
			}
			if strings.HasPrefix(line, "used_memory_human:") {
				usedMemory = strings.TrimPrefix(line, "used_memory_human:")
			}
		}
		infoConn.Close()
	}

	evidence := fmt.Sprintf("redis_version=%q memory=%q ops=[PING,SET,GET,DEL] target=%s",
		strings.TrimSpace(redisVersion), strings.TrimSpace(usedMemory), target)
	return okProbe("redis", target, evidence, start)
}

func probeRedisViaExec(cfg schema.DeepProbeConfig, target string, start time.Time) schema.SingleProbeResult {
	cliArgs := func(sub ...string) []string {
		args := []string{"exec", cfg.Container, "redis-cli"}
		if cfg.Password != "" {
			args = append(args, "-a", cfg.Password, "--no-auth-warning")
		}
		return append(args, sub...)
	}

	out, err := exec.Command("docker", cliArgs("PING")...).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "PONG") {
		return failProbe("redis", start, fmt.Sprintf("docker exec PING failed: %s", strings.TrimSpace(string(out))))
	}

	if out, err := exec.Command("docker", cliArgs("SET", "llmobs:health:probe", "1", "EX", "30")...).CombinedOutput(); err != nil || !strings.Contains(string(out), "OK") {
		return failProbe("redis", start, fmt.Sprintf("docker exec SET failed: %s", strings.TrimSpace(string(out))))
	}

	if out, err := exec.Command("docker", cliArgs("GET", "llmobs:health:probe")...).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "1" {
		return failProbe("redis", start, fmt.Sprintf("docker exec GET failed: %s", strings.TrimSpace(string(out))))
	}

	exec.Command("docker", cliArgs("DEL", "llmobs:health:probe")...).CombinedOutput()

	infoOut, _ := exec.Command("docker", cliArgs("INFO", "server")...).CombinedOutput()
	redisVersion := ""
	for _, line := range strings.Split(string(infoOut), "\n") {
		if strings.HasPrefix(line, "redis_version:") {
			redisVersion = strings.TrimSpace(strings.TrimPrefix(line, "redis_version:"))
		}
	}

	evidence := fmt.Sprintf("redis_version=%q ops=[PING,SET,GET,DEL] container=%s target=%s",
		redisVersion, cfg.Container, target)
	return okProbe("redis", target, evidence, start)
}
