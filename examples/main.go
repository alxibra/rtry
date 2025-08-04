package main

import (
	"log"
	"math"

	"github.com/alxibra/rtry"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	// withCompleteOption()
	withoutCompleteOption()
}

func initialize() (*amqp.Connection, *amqp.Channel) {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	failOnErr(err, "Failed to connect to RabbitMQ")
	ch, err := conn.Channel()
	failOnErr(err, "Failed to open a channel")
	return conn, ch
}

func failOnErr(err error, msg string) {
	if err != nil {
		log.Fatalf("💥 %s: %s", msg, err)
	}
}

func withoutCompleteOption() {
	conn, chn := initialize()
	defer conn.Close()
	defer chn.Close()

	rty, err := rtry.Init(
		"main_exchange",
		"main_queue",
		"retry_queue",
		"main_key",
		"retry_key",
		nil,
		chn,
	)

	if err != nil {
		log.Fatalln(err)
	}

	msgs, _ := rty.Consume(chn)

	log.Println("Ready to consume messages. Press Ctrl+C to exit.")
	for msg := range msgs {
		log.Printf("Received message: %s", msg.Body)
		rty.Retry(
			msg,
			nil,
			chn,
		)
		msg.Ack(false)
	}
}

func withCompleteOption() {
	conn, chn := initialize()
	defer conn.Close()
	defer chn.Close()

	rty, err := rtry.Init(
		"main_exchange",
		"main_queue",
		"retry_queue",
		"main_key",
		"retry_key",
		rtry.Option{
			"max-attempts": 3,
			"backoff": func(retryCount int) int {
				return int(math.Pow(float64(retryCount), 4) + 5)

			},
		},
		chn,
	)

	if err != nil {
		log.Fatalln(err)
	}

	msgs, _ := rty.Consume(chn)

	log.Println("Ready to consume messages. Press Ctrl+C to exit.")
	for msg := range msgs {
		log.Printf("Received message: %s", msg.Body)
		rty.Retry(
			msg,
			rtry.Option{
				"delay_in_second": 5,
			},
			chn,
		)
		msg.Ack(false)
	}

}
