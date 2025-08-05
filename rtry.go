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
	now := time.Now().Format("2006/01/02 15:04:05")
	if fn, ok := o["backoff"].(func(int) int); ok {
		color.Cyan("%s Using configured backoff function", now)
		return fn
	}
	return defaultBackoff
}

func parseMaxAttempts(o Option) int {
	now := time.Now().Format("2006/01/02 15:04:05")
	if val, ok := o["max-attempts"]; ok {
		if attempts, ok := val.(int); ok {
			color.Cyan("%s Using configured max-attempts: %d", now, attempts)
			return attempts
		}
	}
	color.Yellow("%s 'max-attempts' not set, using default: 5", now)
	return 5
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
		color.Red(
			"%s Max retry attempts reached (%d).",
			time.Now().Format("2006/01/02 15:04:05"),
			rty.maxAttempts,
		)
		return fmt.Errorf("Max retry attempts reached (%d).", rty.maxAttempts)
	}
	newHeaders := amqp.Table{}
	maps.Copy(newHeaders, msg.Headers)
	newHeaders["x-retry-count"] = int32(retryCount)
	delaySeconds := rty.parseDelayInSecond(option, retryCount)
	delayMs := strconv.Itoa(delaySeconds * 1000)
	nextRetryAt := time.Now().Add(time.Duration(delaySeconds) * time.Second)
	color.Yellow(
		"%s Attempt %d/%d with delay %d seconds. Retry at: %s",
		time.Now().Format("2006/01/02 15:04:05"),
		retryCount,
		rty.maxAttempts,
		delaySeconds,
		nextRetryAt.Format("2006-01-02 15:04:05"),
	)

	return ch.Publish(
		rty.mainXchange,
		rty.retryKey,
		false,
		false,
		amqp091.Publishing{
			Body:        msg.Body,
			ContentType: msg.ContentType,
			Headers:     newHeaders,
			Expiration:  delayMs,
		},
	)
}

func (r Retry) parseDelayInSecond(o Option, retryCount int) int {
	now := time.Now().Format("2006/01/02 15:04:05")
	if val, ok := o["delay_in_second"]; ok {
		if delay, ok := val.(int); ok {
			color.Cyan("%s Using configured delay_in_second: %d", now, delay)
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
