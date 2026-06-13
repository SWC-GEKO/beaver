package main

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

func main() {
	nc, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		panic(err)
	}

	log.Println(nc.Status())

	ticker := time.NewTicker(1 * time.Second)
	timer := time.NewTimer(20 * time.Second)

	counter := 0
	for {
		select {
		case <-timer.C:
			log.Println("finished execution")
			return
		case <-ticker.C:
			h := make(nats.Header)
			h["Key"] = []string{"user-1"}
			msg := &nats.Msg{
				Subject: "FUNCTIONS.test",
				Header:  h,
				Data:    []byte(fmt.Sprintf("hello-%d", counter)),
			}
			if err = nc.PublishMsg(msg); err != nil {
				panic(err)
			}
			counter++
		}
	}
}
