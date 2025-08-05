package rtry

import (
	"math"
	"testing"

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
