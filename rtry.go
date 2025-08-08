package rtry

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/rabbitmq/amqp091-go"
	amqp "github.com/rabbitmq/amqp091-go"
	"golang.org/x/exp/maps"
)

const retryHeader = "x-retry-count"
const warning = 1
const info = 2
const err = 3
const defaultAttempts = 5

type Option map[string]any

type Retry struct {
	mainXchange string
	mainQueue   string
	retryQueue  string
	retryKey    string
	mainKey     string
	maxAttempts int
	backoff     func(int) int
}

func Init(
	mainXchange string,
	mainQueue string,
	retryQueue string,
	mainKey string,
	retryKey string,
	options Option,
	ch *amqp091.Channel,
) (Retry, error) {
	var rty Retry
	err := ch.ExchangeDeclare(
		mainXchange,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return rty, err
	}

	q, err := ch.QueueDeclare(
		mainQueue,                            // name
		true,                                 // durable
		false,                                // delete when unused
		false,                                // exclusive
		false,                                // no-wait
		amqp.Table{"x-queue-type": "quorum"}, // arguments
	)
	if err != nil {
		return rty, err
	}
	err = ch.QueueBind(
		q.Name,      // queue name
		mainKey,     // routing key
		mainXchange, // exchange name
		false,       // noWait
		nil,         // args
	)
	if err != nil {
		return rty, fmt.Errorf("mainQueue QueueBind:")
	}

	// Declare a retry queue with dead-lettering to the main exchange and routing key.
	rq, err := ch.QueueDeclare(
		retryQueue, // Queue name
		true,       // Durable
		false,      // Delete when unused
		false,      // Exclusive
		false,      // No-wait
		amqp.Table{ // Arguments
			"x-dead-letter-exchange":    mainXchange, // Dead-letter exchange
			"x-dead-letter-routing-key": mainKey,     // Dead-letter routing key
			"x-queue-type":              "quorum",    // Quorum queue type
		},
	)
	if err != nil {
		return rty, fmt.Errorf("retryQueue QueueDeclare: %w", err)
	}

	err = ch.QueueBind(
		rq.Name,
		retryKey,
		mainXchange,
		false,
		nil,
	)
	if err != nil {
		return rty, fmt.Errorf("retryQueue QueueBind:")
	}

	rty.mainXchange = mainXchange
	rty.mainQueue = mainQueue
	rty.retryQueue = retryQueue
	rty.mainKey = mainKey
	rty.retryKey = retryKey
	rty.maxAttempts = parseMaxAttempts(options)
	rty.backoff = parseBackoff(options)
	return rty, nil
}

func parseBackoff(o Option) func(int) int {
	if fn, ok := o["backoff"].(func(int) int); ok {
		bLog(info, "Using configured backoff function")
		return fn
	}
	return defaultBackoff
}

func parseMaxAttempts(o Option) int {
	if val, ok := o["max-attempts"]; ok {
		if attempts, ok := val.(int); ok {
			bLog(info, "Using configured max-attempts: %d", attempts)
			return attempts
		}
	}
	bLog(warning, "'max-attempts' not set, using default: %d", defaultAttempts)
	return defaultAttempts
}

func (rty Retry) Consume(ch *amqp091.Channel) (<-chan amqp091.Delivery, error) {
	msgs, err := ch.Consume(rty.mainQueue, "", false, false, false, false, nil)
	return msgs, fmt.Errorf("Retry Consumer: %w", err)
}

func (rty Retry) Retry(
	msg amqp091.Delivery,
	option Option,
	ch *amqp091.Channel,
) error {
	retryCount := getRetryCount(msg)

	if retryCount > rty.maxAttempts {
		bLog(err, "Max retry attempts reached (%d).", rty.maxAttempts)
		return fmt.Errorf("Max retry attempts reached (%d).", rty.maxAttempts)
	}

	return ch.Publish(
		rty.mainXchange,
		rty.retryKey,
		false,
		false,
		rty.getPublishing(msg, retryCount, option),
	)
}

func (rty Retry) getPublishing(msg amqp091.Delivery, retryCount int, option Option) amqp091.Publishing {
	return amqp091.Publishing{
		Body:        msg.Body,
		ContentType: msg.ContentType,
		Headers:     buildRetryHeaders(msg, retryCount),
		Expiration:  rty.getDelay(option, retryCount),
	}
}

func (r Retry) parseDelayInSecond(o Option, retryCount int) int {
	if val, ok := o["delay_in_second"]; ok {
		if delay, ok := val.(int); ok {
			bLog(info, "Using configured delay_in_second: %d", delay)
			return delay
		}
	}
	return r.backoff(retryCount)
}

func defaultBackoff(retryCount int) int {
	var delay int
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	delay = int(math.Pow(float64(retryCount), 4) + 5)
	jitter := r.Intn(delay) - 2
	delay += int(jitter)
	return delay
}

func getRetryCount(msg amqp091.Delivery) int {
	const defaultRetry = 1

	val, ok := msg.Headers[retryHeader]
	if !ok {
		return defaultRetry
	}

	var retryCount = 0
	switch v := val.(type) {
	case int32:
		retryCount = int(v)
	case int64:
		retryCount = int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			retryCount = n
		} else {
			return defaultRetry
		}
	default:
		return defaultRetry
	}
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount == math.MaxInt32 {
		return retryCount
	}
	return retryCount + 1
}

func buildRetryHeaders(msg amqp091.Delivery, retryCount int) amqp.Table {
	headers := amqp.Table{}
	maps.Copy(headers, msg.Headers)
	headers["x-retry-count"] = int32(retryCount)
	return headers
}

func (rty Retry) getDelay(o Option, retryCount int) string {
	delaySeconds := rty.parseDelayInSecond(o, retryCount)
	now := time.Now()
	nextRetryAt := now.Add(time.Duration(delaySeconds) * time.Second)

	bLog(
		warning,
		"Attempt %d/%d with delay %d seconds. Retry at: %s",
		retryCount,
		rty.maxAttempts,
		delaySeconds,
		nextRetryAt.Format("2006-01-02 15:04:05"),
	)

	return strconv.Itoa(delaySeconds * 1000)
}

func bLog(level int, format string, a ...any) {
	now := time.Now()
	a = append([]any{now.Format("2006/01/02 15:04:05")}, a...)
	format = "%s " + format
	switch level {
	case warning:
		color.Yellow(format, a...)
	case info:
		color.Cyan(format, a...)
	case err:
		color.Red(format, a...)
	default:
		color.Yellow(format, a...)
	}
}
