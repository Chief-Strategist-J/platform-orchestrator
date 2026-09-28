/*
Package probes — AlloyDB deep functional health probe.

ALGORITHM BLUEPRINT:
1. TCP connectivity: reuses dialTCP (wraps net.DialTimeout, same pattern as
   HealthService.probeOnce in services/health.service.go).
2. PostgreSQL v3.0 StartupMessage handshake over the raw socket:
   a. Build StartupMessage: int32 total-length | int32 protocol-version(196608)
      | NUL-delimited key=value pairs (user, database, application_name) | NUL.
   b. Write message; set deadline before every I/O call.
   c. Read backend messages in a loop until ReadyForQuery ('Z') or ErrorResponse:
      - 'R' AuthenticationRequest  → extract auth code label
      - 'S' ParameterStatus        → extract server_version string
      - 'Z' ReadyForQuery          → server is fully functional; break loop
      - 'E' ErrorResponse          → parse SQLSTATE; SQLSTATE 28xxx means a live
        PG server requiring authentication — treat as UP with evidence
      - 'K' BackendKeyData         → skip, continue reading
3. Evidence string: "server_version=<V> auth=<type> target=<host:port>"
4. Invariants:
   - Message body size is bounded to 65536 bytes to prevent OOM.
   - All I/O carries the config Timeout deadline.
   - Connection is closed via defer before return.
   - Returns failProbe on any unrecoverable error; never panics.
*/
package probes

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
)

func ProbeAlloyDB(cfg schema.DeepProbeConfig) schema.SingleProbeResult {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 31420
	}
	user := cfg.Username
	if user == "" {
		user = "admin"
	}
	dbName := cfg.Database
	if dbName == "" {
		dbName = "llm_observability"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	target := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()

	conn, err := dialTCP(host, port, timeout)
	if err != nil {
		return failProbe("alloydb", start, fmt.Sprintf("TCP dial failed: %v", err))
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	msg := BuildPGStartupMessage(user, dbName)
	if _, err := conn.Write(msg); err != nil {
		return failProbe("alloydb", start, fmt.Sprintf("startup write failed: %v", err))
	}

	serverVersion, authType, pgErr := readPGStartupResponse(conn)
	if pgErr != "" {
		return failProbe("alloydb", start, pgErr)
	}

	evidence := fmt.Sprintf("server_version=%q auth=%s target=%s user=%s db=%s",
		serverVersion, authType, target, user, dbName)
	return okProbe("alloydb", target, evidence, start)
}

func BuildPGStartupMessage(user, database string) []byte {
	params := []byte{}
	for _, pair := range [][2]string{{"user", user}, {"database", database}, {"application_name", "llmobs-health-probe"}} {
		params = append(params, []byte(pair[0]+"\x00"+pair[1]+"\x00")...)
	}
	params = append(params, 0x00)
	totalLen := 4 + 4 + len(params)
	buf := make([]byte, totalLen)
	binary.BigEndian.PutUint32(buf[0:4], uint32(totalLen))
	binary.BigEndian.PutUint32(buf[4:8], 196608)
	copy(buf[8:], params)
	return buf
}

func readPGStartupResponse(conn interface{ Read([]byte) (int, error) }) (serverVersion, authType string, pgErr string) {
	body := make([]byte, 65536)
	for i := 0; i < 20; i++ {
		typeBuf := make([]byte, 1)
		if _, err := conn.Read(typeBuf); err != nil {
			pgErr = fmt.Sprintf("read type: %v", err)
			return
		}
		lenBuf := make([]byte, 4)
		if _, err := conn.Read(lenBuf); err != nil {
			pgErr = fmt.Sprintf("read length: %v", err)
			return
		}
		msgLen := int(binary.BigEndian.Uint32(lenBuf)) - 4
		if msgLen < 0 || msgLen > 65536 {
			pgErr = fmt.Sprintf("invalid message length %d", msgLen+4)
			return
		}
		if err := readFull(conn, body[:msgLen]); err != nil {
			pgErr = fmt.Sprintf("read body: %v", err)
			return
		}
		chunk := body[:msgLen]
		switch typeBuf[0] {
		case 'R':
			if len(chunk) >= 4 {
				authCode := binary.BigEndian.Uint32(chunk[:4])
				switch authCode {
				case 0:
					authType = "OK(no-auth)"
				case 3:
					authType = "CleartextPassword"
					return
				case 5:
					authType = "MD5"
					return
				case 10:
					authType = "SASL"
					return
				default:
					authType = fmt.Sprintf("code=%d", authCode)
					return
				}
			}
		case 'S':
			nul := strings.IndexByte(string(chunk), 0)
			if nul >= 0 && string(chunk[:nul]) == "server_version" {
				serverVersion = strings.TrimRight(string(chunk[nul+1:]), "\x00")
			}
		case 'Z':
			return
		case 'E':
			sqlstate, msg := parsePGError(chunk)
			if strings.HasPrefix(sqlstate, "28") {
				if authType == "" {
					authType = "PasswordRequired"
				}
				return
			}
			pgErr = fmt.Sprintf("PG error %s: %s", sqlstate, msg)
			return
		}
	}
	return
}

func parsePGError(body []byte) (sqlstate, message string) {
	i := 0
	for i < len(body) {
		code := body[i]
		i++
		if code == 0 {
			break
		}
		end := strings.IndexByte(string(body[i:]), 0)
		if end < 0 {
			break
		}
		val := string(body[i : i+end])
		i += end + 1
		switch code {
		case 'C':
			sqlstate = val
		case 'M':
			message = val
		}
	}
	return
}
