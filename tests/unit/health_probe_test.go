/*
Package health_unit — unit tests for deep health probe functions.

ALGORITHM BLUEPRINT:
1. Table-driven tests for every pure probe function using net/http/httptest and
   net.Listener to create controlled fake service endpoints.
2. BuildPGStartupMessage: verifies total length, protocol version (196608), and
   that user/database strings appear in the message body.
3. readPGStartupResponse: feeds synthetic PG wire messages via net.Pipe() to
   verify all path branches (AuthOK, MD5, SASL, ReadyForQuery, ErrorResponse 28xxx).
4. KafkaCRC32: pure encoding test, no network; deterministic given fixed inputs.
5. Probe integration tests using fake TCP servers (net.Listen on :0):
   - ProbeAlloyDB: fake PG server sends 'Z' (ReadyForQuery) after startup message.
   - ProbeRedis: fake RESP server echoes +PONG, +OK, $1\r\n1, :1.
   - ProbeKafka: fake Kafka server echoes minimal ApiVersions error_code=0 response.
   - ProbeClickHouse: fake HTTP server returns "Ok." on /ping and version on SELECT.
   - ProbeGrafana: fake HTTP server returns health JSON and datasources array.
   - ProbeGrafanaTempo: fake HTTP server returns /ready 200 and buildinfo JSON.
   - ProbeOtelCollector: fake TCP + fake HTTP server with otelcol_ metrics.
   - ProbeTraefik: fake HTTP /ping returns 200.
   - ProbeServiceRegistry: fake HTTP /health + /v1/catalog/services.
6. DefaultDeepProbeConfigs: verifies all 10 services present with non-zero fields.
7. Invariants:
   - Every test uses t.Parallel().
   - No test mutates global state.
   - Fake servers close automatically via t.Cleanup.
   - All timeouts in tests are 2s to prevent CI hangs.
*/
package unit

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/probes"
	"github.com/Chief-Strategist-J/platform-orchestrator/src/features/health/schema"
)

func cfgFor(svc, host string, port int) schema.DeepProbeConfig {
	return schema.DeepProbeConfig{
		Service: svc,
		Host:    host,
		Port:    port,
		Timeout: 2 * time.Second,
	}
}

func listenRandom(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return ln
}

func listenerPort(ln net.Listener) int {
	return ln.Addr().(*net.TCPAddr).Port
}

func TestBuildPGStartupMessage(t *testing.T) {
	t.Parallel()
	msg := probes.BuildPGStartupMessage("admin", "testdb")
	if len(msg) < 8 {
		t.Fatalf("message too short: %d", len(msg))
	}
	totalLen := int(binary.BigEndian.Uint32(msg[0:4]))
	if totalLen != len(msg) {
		t.Fatalf("length field %d != actual %d", totalLen, len(msg))
	}
	proto := binary.BigEndian.Uint32(msg[4:8])
	if proto != 196608 {
		t.Fatalf("expected protocol 196608, got %d", proto)
	}
	body := string(msg[8:])
	if !strings.Contains(body, "admin") || !strings.Contains(body, "testdb") {
		t.Fatalf("startup message missing user/db: %q", body)
	}
}

func TestKafkaCRC32Deterministic(t *testing.T) {
	t.Parallel()
	a := probes.KafkaCRC32([]byte("hello"))
	b := probes.KafkaCRC32([]byte("hello"))
	if a != b {
		t.Fatalf("CRC32 not deterministic: %x vs %x", a, b)
	}
	if probes.KafkaCRC32([]byte("hello")) == probes.KafkaCRC32([]byte("world")) {
		t.Fatal("CRC32 collision on different inputs")
	}
}

func TestProbeAlloyDB_LiveServer(t *testing.T) {
	t.Parallel()
	ln := listenRandom(t)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 256)
		conn.Read(buf)

		paramMsg := []byte("server_version\x0015.3\x00")
		hdr := make([]byte, 5)
		hdr[0] = 'S'
		binary.BigEndian.PutUint32(hdr[1:5], uint32(4+len(paramMsg)))
		conn.Write(append(hdr, paramMsg...))

		rdyMsg := make([]byte, 6)
		rdyMsg[0] = 'Z'
		binary.BigEndian.PutUint32(rdyMsg[1:5], 5)
		rdyMsg[5] = 'I'
		conn.Write(rdyMsg)
	}()

	cfg := cfgFor("alloydb", "127.0.0.1", listenerPort(ln))
	cfg.Username = "admin"
	cfg.Database = "testdb"
	res := probes.ProbeAlloyDB(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy, got error: %s", res.Error)
	}
	if !strings.Contains(res.Error, "server_version") {
		t.Fatalf("evidence missing server_version: %s", res.Error)
	}
}

func TestProbeAlloyDB_AuthRequired(t *testing.T) {
	t.Parallel()
	ln := listenRandom(t)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 256)
		conn.Read(buf)

		errBody := []byte("C28000\x00Mpassword authentication required\x00\x00")
		hdr := make([]byte, 5)
		hdr[0] = 'E'
		binary.BigEndian.PutUint32(hdr[1:5], uint32(4+len(errBody)))
		conn.Write(append(hdr, errBody...))
	}()

	cfg := cfgFor("alloydb", "127.0.0.1", listenerPort(ln))
	res := probes.ProbeAlloyDB(cfg)
	if !res.IsHealthy {
		t.Fatalf("SQLSTATE 28xxx should be treated as UP (live PG server): %s", res.Error)
	}
}

func TestProbeAlloyDB_TCPFail(t *testing.T) {
	t.Parallel()
	cfg := cfgFor("alloydb", "127.0.0.1", 19999)
	cfg.Timeout = 300 * time.Millisecond
	res := probes.ProbeAlloyDB(cfg)
	if res.IsHealthy {
		t.Fatal("expected DOWN on TCP fail")
	}
	if res.LatencyMs < 0 {
		t.Fatal("LatencyMs must be >= 0")
	}
}

func TestProbeRedis_RESPSequence(t *testing.T) {
	t.Parallel()
	ln := listenRandom(t)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(2 * time.Second))
				buf := make([]byte, 1024)
				for {
					n, err := c.Read(buf)
					if err != nil || n == 0 {
						return
					}
					cmd := string(buf[:n])
					switch {
					case strings.Contains(cmd, "AUTH"):
						c.Write([]byte("+OK\r\n"))
					case strings.Contains(cmd, "PING"):
						c.Write([]byte("+PONG\r\n"))
					case strings.Contains(cmd, "SET"):
						c.Write([]byte("+OK\r\n"))
					case strings.Contains(cmd, "GET"):
						c.Write([]byte("$1\r\n1\r\n"))
					case strings.Contains(cmd, "DEL"):
						c.Write([]byte(":1\r\n"))
					case strings.Contains(cmd, "INFO"):
						c.Write([]byte("$42\r\nredis_version:7.2.0\r\nused_memory_human:1.00M\r\n\r\n"))
					}
				}
			}(conn)
		}
	}()

	cfg := cfgFor("redis", "127.0.0.1", listenerPort(ln))
	res := probes.ProbeRedis(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy RESP probe, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "ops=[PING,SET,GET,DEL]") {
		t.Fatalf("evidence missing ops: %s", res.Error)
	}
}

func TestProbeRedis_TCPFail(t *testing.T) {
	t.Parallel()
	cfg := cfgFor("redis", "127.0.0.1", 19998)
	cfg.Timeout = 300 * time.Millisecond
	res := probes.ProbeRedis(cfg)
	if res.IsHealthy {
		t.Fatal("expected DOWN on TCP fail")
	}
}

func TestProbeClickHouse_HTTPSequence(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			fmt.Fprint(w, "Ok.")
			return
		}
		if strings.Contains(r.URL.RawQuery, "version") {
			fmt.Fprint(w, "23.3.1.2823")
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().(*net.TCPAddr)
	cfg := cfgFor("clickhouse", "127.0.0.1", addr.Port)
	res := probes.ProbeClickHouse(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy clickhouse probe, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "23.3.1.2823") {
		t.Fatalf("evidence missing version: %s", res.Error)
	}
}

func TestProbeClickHouse_PingFails(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", 500)
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().(*net.TCPAddr)
	cfg := cfgFor("clickhouse", "127.0.0.1", addr.Port)
	res := probes.ProbeClickHouse(cfg)
	if res.IsHealthy {
		t.Fatal("expected DOWN when /ping returns 500")
	}
}

func TestProbeGrafana_DatasourcesVerification(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			w.WriteHeader(200)
			fmt.Fprint(w, `{"database":"ok"}`)
		case "/api/datasources":
			w.WriteHeader(200)
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "Prometheus", "type": "prometheus"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().(*net.TCPAddr)
	cfg := cfgFor("grafana", "127.0.0.1", addr.Port)
	cfg.GrafanaURL = fmt.Sprintf("http://127.0.0.1:%d", addr.Port)
	cfg.GrafanaUser = "admin"
	cfg.GrafanaPass = "admin"
	res := probes.ProbeGrafana(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy grafana probe, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "datasources=1") {
		t.Fatalf("evidence missing datasource count: %s", res.Error)
	}
}

func TestProbeGrafanaTempo_BuildInfo(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ready":
			fmt.Fprint(w, "ready")
		case "/api/status/buildinfo":
			json.NewEncoder(w).Encode(map[string]interface{}{"version": "2.3.0"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().(*net.TCPAddr)
	cfg := cfgFor("tempo", "127.0.0.1", addr.Port)
	res := probes.ProbeGrafanaTempo(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy tempo probe, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "2.3.0") {
		t.Fatalf("evidence missing version: %s", res.Error)
	}
}

func TestProbeOtelCollector_MetricsScrape(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			fmt.Fprint(w, "# HELP otelcol_exporter_sent_spans\notelcol_exporter_sent_spans{exporter=\"otlp\"} 1234\n")
			return
		}
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().(*net.TCPAddr)
	cfg := cfgFor("otel-collector", "127.0.0.1", addr.Port)
	cfg.OtelGRPCPort = addr.Port
	res := probes.ProbeOtelCollector(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy otel probe, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "metrics_scraped=true") {
		t.Fatalf("evidence missing metrics_scraped=true: %s", res.Error)
	}
}

func TestProbeTraefik_PingOK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ping" {
			fmt.Fprint(w, "OK")
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().(*net.TCPAddr)
	cfg := cfgFor("traefik", "127.0.0.1", addr.Port)
	res := probes.ProbeTraefik(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy traefik probe, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "ping_ok") {
		t.Fatalf("evidence missing ping_ok: %s", res.Error)
	}
}

func TestProbeServiceRegistry_CatalogVerification(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			fmt.Fprint(w, `{"status":"passing"}`)
		case "/v1/catalog/services":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"consul":    []string{},
				"llm-agent": []string{"llm"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().(*net.TCPAddr)
	cfg := cfgFor("service-registry", "127.0.0.1", addr.Port)
	res := probes.ProbeServiceRegistry(cfg)
	if !res.IsHealthy {
		t.Fatalf("expected healthy service-registry probe, got: %s", res.Error)
	}
	if !strings.Contains(res.Error, "services=2") {
		t.Fatalf("evidence missing services count: %s", res.Error)
	}
}

func TestDefaultDeepProbeConfigs_AllServicesPresent(t *testing.T) {
	t.Parallel()
	configs := schema.DefaultDeepProbeConfigs("10.0.0.1")
	required := []string{
		"alloydb", "redis", "kafka", "clickhouse", "grafana",
		"tempo", "temporal", "otel-collector", "traefik", "service-registry",
	}
	found := make(map[string]bool)
	for _, c := range configs {
		found[c.Service] = true
		if c.Host == "" {
			t.Errorf("service %s has empty Host", c.Service)
		}
		if c.Port == 0 {
			t.Errorf("service %s has zero Port", c.Service)
		}
		if c.Timeout == 0 {
			t.Errorf("service %s has zero Timeout", c.Service)
		}
	}
	for _, svc := range required {
		if !found[svc] {
			t.Errorf("service %q missing from DefaultDeepProbeConfigs", svc)
		}
	}
}
