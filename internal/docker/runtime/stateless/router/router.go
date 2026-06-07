package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	UniqueName    string
	GlobalNats    string
	GlobalStream  string
	GlobalSubject string

	LocalBaseTopic string
	VirtualShards  int
)

type Router struct {
	GlobalNats      *nats.Conn
	GlobalJetStream string
	GlobalSubject   string

	LocalNats   *nats.Conn
	LocalTopics []string
}

func New(globalBusAddr, globalTopic, globalJetStream, baseTopic string, vShards int, localNats *nats.Conn) (*Router, error) {
	nc, err := nats.Connect(globalBusAddr)
	if err != nil {
		return nil, err
	}

	log.Println(nc.Status())

	shards := make([]string, vShards)
	for i := 0; i < vShards; i++ {
		shards[i] = fmt.Sprintf("%s.%d", baseTopic, i)
	}

	return &Router{
		GlobalNats:      nc,
		GlobalJetStream: globalJetStream,
		GlobalSubject:   globalTopic,
		LocalNats:       localNats,
		LocalTopics:     shards,
	}, nil
}

func (r *Router) RouteEvents(ctx context.Context) error {
	js, err := jetstream.New(r.GlobalNats)
	if err != nil {
		return err
	}

	log.Println("started to consume from jetstream")

	cons, err := js.CreateOrUpdateConsumer(ctx, r.GlobalJetStream, jetstream.ConsumerConfig{
		AckPolicy:     jetstream.AckExplicitPolicy, // requires to acknowledge every message
		FilterSubject: r.GlobalSubject,
	})
	if err != nil {
		return err
	}

	conCtx, err := cons.Consume(func(msg jetstream.Msg) {
		log.Printf("consumed message: %+v", msg)
		k, err := GetKeyFromMsg(msg)
		if err != nil {
			log.Println("fetching key from message failed with: ", err)
		}

		s := GetShard(k, len(r.LocalTopics))

		pubMsg := nats.Msg{
			Subject: r.LocalTopics[s],
			Header:  msg.Headers(),
			Data:    msg.Data(),
		}

		log.Printf("%+v", pubMsg)

		if err = r.LocalNats.PublishMsg(&pubMsg); err != nil {
			log.Println("publishing message to local nats failed with: ", err)
		}

		log.Println("published msg to local NATS")

		if err = msg.Ack(); err != nil {
			log.Println("acknowledging message failed with: ", err)
		}
	})
	if err != nil {
		return err
	}
	defer conCtx.Stop()

	<-ctx.Done()
	return nil
}

// TODO: implement JetStream
func main() {
	LoadEnvVars()
	s, err := server.NewServer(&server.Options{
		JetStream: true,
	})
	if err != nil {
		panic(err)
	}
	go s.Start()

	if !s.ReadyForConnections(5 * time.Second) {
		panic("local message bus (nats) didn't start in 5s, aborting")
	}

	l, err := nats.Connect("nats://localhost:4222")
	if err != nil {
		panic(err)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	router, err := New(GlobalNats, GlobalSubject, GlobalStream, LocalBaseTopic, VirtualShards, l)
	if err != nil {
		panic(err)
	}

	if err = router.RouteEvents(ctx); err != nil {
		panic(err)
	}

	select {
	case <-ctx.Done():
		log.Println("shutdown requested")
	}
}

func LoadEnvVars() {
	UniqueName = os.Getenv("NAME")
	if UniqueName == "" {
		panic("function name must be set")
	}

	GlobalNats = os.Getenv("GLOBAL_NATS")
	if GlobalNats == "" {
		panic("global nats addr must be set")
	}

	GlobalStream = os.Getenv("GLOBAL_STREAM")
	if GlobalStream == "" {
		panic("global stream must be set")
	}

	GlobalSubject = os.Getenv("GLOBAL_TOPIC")
	if GlobalSubject == "" {
		panic("global subject must be set")
	}

	LocalBaseTopic = os.Getenv("LOCAL_TOPIC")
	if LocalBaseTopic == "" {
		panic("local base topic must be set")
	}

	var err error
	VirtualShards, err = strconv.Atoi(os.Getenv("SHARDS"))
	if err != nil {
		panic(err)
	}
}
