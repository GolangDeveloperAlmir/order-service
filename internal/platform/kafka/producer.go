package kafka

import (
	"strings"
	"time"

	"context"

	"github.com/GolangDeveloperAlmir/order-service/internal/platform/log"
	k "github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *k.Writer
	log    *log.Logger
}

func NewProducer(brokersCSV, topic string) *Producer {
	brokers := strings.Split(brokersCSV, ",")

	return &Producer{
		writer: &k.Writer{
			Addr:         k.TCP(brokers...),
			Topic:        topic,
			Balancer:     &k.Hash{},
			BatchTimeout: 50 * time.Millisecond,
			RequiredAcks: k.RequireOne,
		},
	}
}

func (p *Producer) Close() error {
	p.log.Info("Close Producer")
	return p.writer.Close()
}

func (p *Producer) Publish(ctx context.Context, key string, value []byte) error {
	p.log.Info("Producer write message")

	return p.writer.WriteMessages(
		ctx,
		k.Message{
			Key:   []byte(key),
			Value: value,
		},
	)
}
