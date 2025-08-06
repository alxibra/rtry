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
