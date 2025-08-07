package rtry

import (
	"math"
	"testing"

	"github.com/rabbitmq/amqp091-go"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestGetRetryCount(t *testing.T) {
	cases := []struct {
		name   string
		header any
		want   int
	}{
		{"no header", nil, 1},
		{"int32 header", int32(3), 4},
		{"int64 header", int64(2), 3},
		{"string numeric", "5", 6},
		{"string non-numeric", "abc", 1},
		{"unsupported type", 3.14, 1},
		{"negative value", int32(-7), 1},
		{"max int32", int32(math.MaxInt32), math.MaxInt32},
	}

	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := amqp.Table{}
			if tc.header != nil {
				h[retryHeader] = tc.header
			}

			got := getRetryCount(amqp.Delivery{Headers: h})
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestBuildRetryHeaders(t *testing.T) {
	// Given: a message with initial headers
	msg := amqp091.Delivery{
		Headers: amqp091.Table{
			"x-original":    "keep-me",
			"x-retry-count": int32(1), // existing count to be overridden
		},
	}

	retryCount := 3

	headers := buildRetryHeaders(msg, retryCount)

	if val, ok := headers["x-original"]; !ok || val != "keep-me" {
		t.Errorf("expected x-original header to be preserved, got: %v", val)
	}

	// And: should override or add x-retry-count
	if val, ok := headers["x-retry-count"]; !ok {
		t.Error("expected x-retry-count to be present")
	} else if intVal, ok := val.(int32); !ok || intVal != int32(retryCount) {
		t.Errorf("expected x-retry-count to be %d, got %v", retryCount, val)
	}
}

func TestGetPublishing(t *testing.T) {
	rty := Retry{
		mainXchange: "main-ex",
		mainQueue:   "main-queue",
		retryQueue:  "retry-queue",
		mainKey:     "main-key",
		retryKey:    "retry-key",
		maxAttempts: 5,
		backoff: func(i int) int {
			return 3
		},
	}

	msg := amqp091.Delivery{
		Body:        []byte("hello"),
		ContentType: "application/json",
		Headers:     amqp091.Table{"x-original-header": "abc"},
	}

	retryCount := 2

	option := Option{
		"delay_in_second": 3,
	}

	pub := rty.getPublishing(msg, retryCount, option)

	if string(pub.Body) != string(msg.Body) {
		t.Errorf("unexpected body: got %q, want %q", pub.Body, msg.Body)
	}

	if pub.ContentType != msg.ContentType {
		t.Errorf("unexpected content type: got %q, want %q", pub.ContentType, msg.ContentType)
	}

	if pub.Expiration != "3000" {
		t.Errorf("unexpected expiration: got %q, want %q", pub.Expiration, "3000")
	}

	if v, ok := pub.Headers["x-original-header"]; !ok || v != "abc" {
		t.Errorf("missing or incorrect x-original-header: got %v", v)
	}

	if v, ok := pub.Headers["x-retry-count"]; !ok || v != int32(retryCount) {
		t.Errorf("missing or incorrect x-retry-count: got %v", v)
	}
}
