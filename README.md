# rtry

![Go Version](https://img.shields.io/badge/go-1.18%2B-blue)

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

### With Custom Options

```go
rty, err := rtry.Init(
	"main_exchange",
	"main_queue",
	"retry_queue",
	"main_key",
	"retry_key",
	rtry.Option{
		"max-attempts": 3,
		"backoff": func(retryCount int) int {
			return int(math.Pow(float64(retryCount), 4) + 5) // custom exponential backoff function
		},
	},
	chn,
)

msgs, _ := rty.Consume(chn)

for msg := range msgs {
	rty.Retry(
		msg,
		rtry.Option{
			"delay_in_second": 5, // if need fixed delayed 
		},
		chn,
	)
	msg.Ack(false)
}
```

**Retry Options**


| Option Key        | Type             | Description                                      |
|-------------------|------------------|--------------------------------------------------|
| `max-attempts`    | `int`            | Maximum retry attempts before dropping           |
| `backoff`         | `func(int) int`  | Custom backoff strategy per retry count          |
| `delay_in_second` | `int`            | Fixed delay override (bypasses backoff function) |

##  Requirements

- Go **1.18+**
- The following dependencies:

```go
require (
    github.com/fatih/color v1.18.0
    github.com/rabbitmq/amqp091-go v1.10.0
    golang.org/x/exp v0.0.0-20250718183923-645b1fa84792
)
