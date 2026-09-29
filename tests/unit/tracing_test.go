/*
Package unit provides unit test coverage for centralized OpenTelemetry tracing and context propagation.
*/
package unit

import (
	"context"
	"net/http"
	"testing"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/infra/observability"
)

func TestTracingSpanLifecycleAndAttributes(t *testing.T) {
	tracer := observability.NewOTelTracerAdapter("platform-orchestrator")

	ctx, rootSpan := tracer.StartSpanWithAttributes(context.Background(), "test.root_operation", map[string]interface{}{
		"component": "unit-test",
		"env":       "test",
	})
	defer rootSpan.End()

	if rootSpan.TraceID() == "" {
		t.Fatalf("expected non-empty trace ID")
	}
	if rootSpan.SpanID() == "" {
		t.Fatalf("expected non-empty span ID")
	}

	observability.SetAttribute(ctx, "custom.key", "custom.value")
	observability.AddEvent(ctx, "checkpoint_reached", map[string]interface{}{
		"step": 1,
	})

	childCtx, childSpan := tracer.StartSpanWithAttributes(ctx, "test.child_operation", map[string]interface{}{
		"child.field": "child.val",
	})
	defer childSpan.End()

	if childSpan.TraceID() != rootSpan.TraceID() {
		t.Fatalf("expected child trace ID %q to match root %q", childSpan.TraceID(), rootSpan.TraceID())
	}
	if childSpan.SpanID() == rootSpan.SpanID() {
		t.Fatalf("expected child span ID to differ from root span ID")
	}

	req, err := http.NewRequestWithContext(childCtx, http.MethodGet, "http://localhost:3000", nil)
	if err != nil {
		t.Fatalf("failed creating request: %v", err)
	}

	observability.InjectHTTPHeaders(childCtx, req.Header)
	traceparent := req.Header.Get("traceparent")
	if traceparent == "" {
		t.Fatalf("expected traceparent header to be injected")
	}
	if len(traceparent) < 53 {
		t.Fatalf("invalid traceparent format: %s", traceparent)
	}
}
