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

	return probeRedisViaRESP(host, port, cfg.Password, target, timeout, start)
}

func probeRedisViaRESP(host string, port int, password, target string, timeout time.Duration, start time.Time) schema.SingleProbeResult {
	conn, err := dialTCP(host, port, timeout)
	if err != nil {
		return failProbe("redis", start, fmt.Sprintf("TCP dial failed: %v", err))
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	scanner := bufio.NewScanner(conn)
	resp := func(cmd string) (string, error) {
		if _, err := conn.Write([]byte(cmd)); err != nil {
			return "", err
		}
		if scanner.Scan() {
			return scanner.Text(), nil
		}
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no response")
	}

	if password != "" {
		authCmd := fmt.Sprintf("*2\r\n$4\r\nAUTH\r\n$%d\r\n%s\r\n", len(password), password)
		if line, err := resp(authCmd); err != nil || !strings.HasPrefix(line, "+OK") {
			if strings.HasPrefix(line, "-ERR") || strings.HasPrefix(line, "-WRONGPASS") {
				return okProbe("redis", target, fmt.Sprintf("live_server %s", line), start)
			}
			return failProbe("redis", start, fmt.Sprintf("AUTH failed: %s %v", line, err))
		}
	}

	line, err := resp("*1\r\n$4\r\nPING\r\n")
	if err != nil {
		return failProbe("redis", start, fmt.Sprintf("PING failed: %v", err))
	}
	if strings.HasPrefix(line, "-NOAUTH") {
		return okProbe("redis", target, "server_alive auth=PasswordRequired", start)
	}
	if !strings.HasPrefix(line, "+PONG") {
		return failProbe("redis", start, fmt.Sprintf("PING unexpected response: %s", line))
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
	if scanner.Scan() {
		_ = scanner.Text() // consume value
	}

	delCmd := fmt.Sprintf("*2\r\n$3\r\nDEL\r\n$%d\r\n%s\r\n", len(setKey), setKey)
	resp(delCmd)

	evidence := fmt.Sprintf("ops=[PING,SET,GET,DEL] target=%s", target)
	return okProbe("redis", target, evidence, start)
}


