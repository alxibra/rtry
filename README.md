# rtry

**rtry** is a lightweight Go library for handling RabbitMQ retry logic using a dedicated retry queue, exponential backoff with jitter, and configurable max-attempt limits. It supports clean separation of retry queue declarations and re-publishing logic when processing fails.

---

##  Features

-  **Exponential backoff with jitter** (customizable)
-  **Retry metadata** via headers (`x-retry-count`)
-  **Max retry attempts** with graceful drop logging
-  **Inject custom backoff strategy**
-  **Clean RabbitMQ queue setup** (main + retry + bindings)

---

##  Installation

```bash
go get github.com/alxibra/rtry@latest
```

---

## Quick Start

### Minimal Example

```go
package main

import (
	"log"

	"github.com/alxibra/rtry"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
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

	log.Println("Ready to consume messages.")
	for msg := range msgs {
		log.Printf("Received message: %s", msg.Body)
		rty.Retry(msg, nil, chn)
		msg.Ack(false)
	}
}

func initialize() (*amqp.Connection, *amqp.Channel) {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	return conn, ch
}

```
