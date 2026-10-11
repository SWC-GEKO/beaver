package controlplane

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const idleTimeout = 10 * time.Minute

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

	log.Printf("registering function with full-topic-name: %s", fullTopicName)
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

	log.Printf("registered function: %v successfully", o.Functions[fullTopicName])
	return true
}

// Start needs to be called by the ControlPlane in a Goroutine
func (o *Observer) Start() error {
	log.Println("started to observe the global-nats...")
	_, err := o.Consumer.Consume(func(msg jetstream.Msg) {
		log.Printf("received message in subject: %s", msg.Subject())

		subject := msg.Subject()
		md, err := msg.Metadata()
		if err != nil {
			return
		}

		o.mtx.Lock()
		f, ok := o.Functions[subject]
		if !ok {
			o.mtx.Unlock()
			return
		}
		f.lastEvent = time.Now()

		switch f.Status {
		case Active:
			o.mtx.Unlock()
			return
		case Stopping:
			if f.pendingSeq == 0 {
				f.pendingSeq = md.Sequence.Stream
			}
			o.mtx.Unlock()
			return
		default:
			log.Println("unknown function status")
		}
		f.Status = Active
		o.mtx.Unlock()

		go o.startFunction(context.Background(), subject, f, md.Sequence.Stream)
	})
	return err
}

func (o *Observer) startFunction(ctx context.Context, subj string, f *Function, seq uint64) {
	start := time.Now()
	if err := f.Start(ctx, seq); err != nil {
		log.Printf("failed to start function %s: %v", f.UniqueName, err)
		o.setIdle(subj)
		return
	}
	log.Println("starting took: ", time.Since(start))

	go o.stopWhenIdle(ctx, subj, f)
}

func (o *Observer) stopWhenIdle(ctx context.Context, subject string, f *Function) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.mtx.Lock()
			idle := time.Since(f.lastEvent) >= idleTimeout

			if idle {
				f.Status = Stopping
			}
			o.mtx.Unlock()

			if !idle {
				continue
			}

			if err := f.Stop(ctx); err != nil {
				log.Printf("stopping %s failed: %v", f.UniqueName, err)
				return
			}

			o.mtx.Lock()
			pending := f.pendingSeq
			if pending != 0 {
				f.Status = Active
			} else {
				f.Status = Idle
			}
			o.mtx.Unlock()

			if pending != 0 {
				go o.startFunction(ctx, subject, f, pending)
			}
			return
		}
	}
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
