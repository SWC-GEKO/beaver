package main

import (
	"beam/pkg"
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"google.golang.org/api/option"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/io/pubsubio"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/x/beamx"
)

const projectId = ""
const subscriptionId = ""
const duration = 0
const tickRate = 1 // events per second
const topicName = ""
const seed = 42
const keySpace = 4

var ctx = context.Background()

func keyFn(b []byte) (string, pkg.Message) {
	m := pkg.Deserialize(b)
	return m.Key, m
}

func main() {
	flag.Parse()
	beam.Init()

	p := beam.NewPipeline()
	s := p.Root()

	pc := pubsubio.Read(s, projectId, pubsubio.ReadOptions{
		Subscription: subscriptionId,
	})

	keyed := beam.ParDo(s, keyFn, pc)
	beam.ParDo0(s, pkg.NewStatsFn(), keyed)

	go benchmark(duration * time.Second)
	if err := beamx.Run(ctx, p); err != nil {
		panic(err)
	}
}

func benchmark(dur time.Duration) {
	timer := time.NewTimer(dur)
	defer timer.Stop()

	interval := time.Second / time.Duration(tickRate)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	client, err := pubsub.NewClient(context.Background(), projectId, option.WithCredentialsFile("keys.json"))
	if err != nil {
		log.Fatalln(err)
	}
	defer client.Close()
	pub := client.Publisher(topicName)

	rng := rand.New(rand.NewSource(seed))
	t0 := time.Now().UnixNano()
	messageId := atomic.Int64{}

loop:
	for {
		select {
		case <-timer.C:
			log.Println("finishing benchmark...")
			break loop
		case <-ticker.C:
			go func() {
				value := float64(rng.Intn(100) + 1)
				key := fmt.Sprintf("%d", rng.Intn(keySpace))

				m := pkg.NewMessage(key, messageId.Load(), t0, time.Now().UnixNano(), value)
				messageId.Add(1)

				res := pub.Publish(ctx, &pubsub.Message{
					Data: m.Serialize(),
				})
				if msgId, err := res.Get(ctx); err == nil {
					log.Println("publishing success " + msgId)
				} else {
					log.Fatalln("error while publishing message: " + err.Error())
				}
			}()
		}

	}
}
