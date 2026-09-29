/*
Package observability implements OpenTelemetry tracing, span tree lifecycle management, and W3C context propagation conforming to OTel semantic conventions.

ALGORITHM BLUEPRINT:
1. StartSpan / StartSpanWithAttributes: Derives a child span context linked to parent trace/span IDs, tracking start time, attributes, and lifecycle events.
2. In-Depth Context Propagation: Injects and extracts W3C Trace Context '00-{trace_id}-{span_id}-01' headers into outbound HTTP requests and goroutine contexts.
3. Event & Error Recording: Captures timestamped domain events and error statuses on active spans for distributed diagnostic analysis.
4. Invariants:
   - Zero inline comments inside function bodies.
   - All span lifecycles must be closed via End() or deferral.
   - Trace IDs must be 128-bit hex strings; Span IDs must be 64-bit hex strings.
   - Span operations must be thread-safe.
*/
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Chief-Strategist-J/platform-orchestrator/src/shared/ports"
)

type contextKey string

const (
	traceParentKey contextKey = "traceparent"
	activeSpanKey  contextKey = "active_span"
)

type SpanEvent struct {
	Name       string                 `json:"name"`
	Timestamp  time.Time              `json:"timestamp"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

type OTelSpan struct {
	traceID       string
	spanID        string
	parentSpanID  string
	operationName string
	startTime     time.Time
	endTime       time.Time
	attributes    map[string]interface{}
	events        []SpanEvent
	err           error
	status        string
	mu            sync.RWMutex
}

func (s *OTelSpan) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.endTime.IsZero() {
		s.endTime = time.Now()
	}
}

func (s *OTelSpan) SetAttribute(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]interface{})
	}
	s.attributes[key] = value
}

func (s *OTelSpan) SetAttributes(attrs map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attributes == nil {
		s.attributes = make(map[string]interface{})
	}
	for k, v := range attrs {
		s.attributes[k] = v
	}
}

func (s *OTelSpan) AddEvent(name string, attrs map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event := SpanEvent{
		Name:       name,
		Timestamp:  time.Now(),
		Attributes: attrs,
	}
	s.events = append(s.events, event)
}

func (s *OTelSpan) RecordError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
	s.status = "Error"
	if s.attributes == nil {
		s.attributes = make(map[string]interface{})
	}
	s.attributes["error"] = true
	s.attributes["error.message"] = err.Error()
}

func (s *OTelSpan) TraceID() string {
	return s.traceID
}

func (s *OTelSpan) SpanID() string {
	return s.spanID
}

func (s *OTelSpan) OperationName() string {
	return s.operationName
}

func (s *OTelSpan) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.endTime.IsZero() {
		return time.Since(s.startTime)
	}
	return s.endTime.Sub(s.startTime)
}

type OTelTracerAdapter struct {
	serviceName string
}

func NewOTelTracerAdapter(serviceName string) ports.TracerPort {
	return &OTelTracerAdapter{
		serviceName: serviceName,
	}
}

func (t *OTelTracerAdapter) StartSpan(ctx context.Context, operationName string) (context.Context, func()) {
	newCtx, span := t.StartSpanWithAttributes(ctx, operationName, nil)
	return newCtx, span.End
}

func (t *OTelTracerAdapter) StartSpanWithAttributes(ctx context.Context, operationName string, attrs map[string]interface{}) (context.Context, ports.Span) {
	parentTraceparent := t.InjectTraceContext(ctx)
	traceID := ""
	parentSpanID := ""

	if parentTraceparent != "" && len(parentTraceparent) >= 53 {
		traceID = parentTraceparent[3:35]
		parentSpanID = parentTraceparent[36:52]
	} else {
		b := make([]byte, 16)
		rand.Read(b)
		traceID = hex.EncodeToString(b)
	}

	spanBytes := make([]byte, 8)
	rand.Read(spanBytes)
	spanID := hex.EncodeToString(spanBytes)

	mergedAttrs := make(map[string]interface{})
	mergedAttrs["service.name"] = t.serviceName
	for k, v := range attrs {
		mergedAttrs[k] = v
	}

	span := &OTelSpan{
		traceID:       traceID,
		spanID:        spanID,
		parentSpanID:  parentSpanID,
		operationName: operationName,
		startTime:     time.Now(),
		attributes:    mergedAttrs,
		events:        make([]SpanEvent, 0),
		status:        "OK",
	}

	newTraceParent := fmt.Sprintf("00-%s-%s-01", traceID, spanID)
	newCtx := context.WithValue(ctx, traceParentKey, newTraceParent)
	newCtx = context.WithValue(newCtx, activeSpanKey, span)

	return newCtx, span
}

func (t *OTelTracerAdapter) InjectTraceContext(ctx context.Context) string {
	if val, ok := ctx.Value(traceParentKey).(string); ok && val != "" {
		return val
	}
	b := make([]byte, 16)
	rand.Read(b)
	span := make([]byte, 8)
	rand.Read(span)
	return fmt.Sprintf("00-%s-%s-01", hex.EncodeToString(b), hex.EncodeToString(span))
}

func (t *OTelTracerAdapter) ExtractTraceContext(ctx context.Context, traceparent string) context.Context {
	if traceparent == "" {
		return ctx
	}
	return context.WithValue(ctx, traceParentKey, traceparent)
}

func SpanFromContext(ctx context.Context) ports.Span {
	if span, ok := ctx.Value(activeSpanKey).(ports.Span); ok {
		return span
	}
	return &noopSpan{}
}

func SetAttribute(ctx context.Context, key string, value interface{}) {
	SpanFromContext(ctx).SetAttribute(key, value)
}

func SetAttributes(ctx context.Context, attrs map[string]interface{}) {
	SpanFromContext(ctx).SetAttributes(attrs)
}

func AddEvent(ctx context.Context, name string, attrs map[string]interface{}) {
	SpanFromContext(ctx).AddEvent(name, attrs)
}

func RecordError(ctx context.Context, err error) {
	if err != nil {
		SpanFromContext(ctx).RecordError(err)
	}
}

func InjectHTTPHeaders(ctx context.Context, header http.Header) {
	if header == nil {
		return
	}
	if val, ok := ctx.Value(traceParentKey).(string); ok && val != "" {
		header.Set("traceparent", val)
	}
}

type noopSpan struct{}

func (n *noopSpan) End()                                       {}
func (n *noopSpan) SetAttribute(key string, value interface{}) {}
func (n *noopSpan) SetAttributes(attrs map[string]interface{})  {}
func (n *noopSpan) AddEvent(name string, attrs map[string]interface{}) {
}
func (n *noopSpan) RecordError(err error) {}
func (n *noopSpan) TraceID() string       { return "" }
func (n *noopSpan) SpanID() string        { return "" }
