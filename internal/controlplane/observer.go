package controlplane

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Observer struct {
	Stream    string
	JetStream jetstream.JetStream

	Functions map[string]*Function
	Consumer  jetstream.Consumer

	mtx sync.Mutex
}

func NewObserver(ctx context.Context, natsUrl, stream string) (*Observer, error) {
	nc, err := nats.Connect(natsUrl)
	if err != nil {
		return nil, fmt.Errorf("connecting to global nats failed with err: %v", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("creating jetstream instance based on valid nats connection failed with err: %v", err)
	}

	natsConsumer, err := js.CreateOrUpdateConsumer(ctx, stream, jetstream.ConsumerConfig{
		Name:          "nats-observer",
		Durable:       "nats-observer",
		DeliverPolicy: jetstream.DeliverNewPolicy,
		AckPolicy:     jetstream.AckNonePolicy,
		FilterSubject: fmt.Sprintf("%s.>", stream),
	})
	if err != nil {
		return nil, fmt.Errorf("creating consumer failed with err: %v", err)
	}

	return &Observer{
		Stream:    stream,
		JetStream: js,
		Consumer:  natsConsumer,
		Functions: make(map[string]*Function),
	}, nil
}

func (o *Observer) RegisterFunction(rec FunctionRecord) bool {
	fullTopicName := fmt.Sprintf("%s.%s", o.Stream, rec.UniqueName)

	o.mtx.Lock()
	defer o.mtx.Unlock()

	if _, exists := o.Functions[fullTopicName]; exists {
		return false
	}

	f, err := NewFunction(rec)
	if err != nil {
		log.Println("creating new function failed with err: ", err)
		return false
	}
	o.Functions[fullTopicName] = f

	return true
}

// Start needs to be called by the ControlPlane in a Goroutine
func (o *Observer) Start() error {
	log.Println("started to observe the global-nats...")
	_, err := o.Consumer.Consume(func(msg jetstream.Msg) {
		log.Printf("received message in subject: %s", msg.Subject())

		subject := msg.Subject()

		o.mtx.Lock()

		f, exists := o.Functions[subject]
		if !exists {
			o.mtx.Unlock()
			return
		}

		if f.Status == Active {
			o.mtx.Unlock()
			return
		}

		f.Status = Active
		o.Functions[subject] = f
		o.mtx.Unlock()

		go o.startFunction(subject, f, msg)
	})
	return err
}

func (o *Observer) startFunction(subj string, f *Function, msg jetstream.Msg) {
	// Strip FUNCTIONS.<unique-name> -> <unique-name>
	uniqueName := strings.TrimPrefix(subj, o.Stream+".")

	metadata, err := msg.Metadata()
	if err != nil {
		log.Println("failed to fetch metadata: ", err)
		return
	}

	log.Println("starting compose file")
	start := time.Now()
	err = f.Start(context.Background(), metadata.Sequence.Stream)
	if err != nil {
		log.Printf("failed to start function %s: %v", uniqueName, err)
		o.setIdle(subj)
		return
	}

	// TODO: implement goroutine that in parallel waits to shutdown the function!

	log.Println("starting took: ", time.Since(start))
}

func (o *Observer) stopFunction(subj string, f *Function) {
	// TODO: implement me
	panic("implement me...")
}

func (o *Observer) setIdle(subj string) {
	o.mtx.Lock()
	defer o.mtx.Unlock()

	f, exists := o.Functions[subj]
	if !exists {
		return
	}

	f.Status = Idle
	o.Functions[subj] = f
	log.Printf("function %s set idle", subj)
}
