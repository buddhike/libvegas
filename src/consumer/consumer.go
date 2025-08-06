package consumer

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kinesis/types"
	"github.com/buddhike/pebble/aws"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Consumer struct {
	cfg                  *ConsumerConfig
	ready                chan struct{}
	done                 chan struct{}
	processFn            func(types.Record)
	kds                  aws.Kinesis
	componentsDoneNotify map[string]runnable
	logger               *zap.Logger
	pop                  *PopService
}

func MustNewConsumer(name, streamName, efoConsumerArn, popUrls string, processFn func(types.Record), opts ...func(*ConsumerConfig)) *Consumer {
	cfg := &ConsumerConfig{
		ID:                                   uuid.NewString(),
		Name:                                 name,
		StreamName:                           streamName,
		EfoConsumerArn:                       efoConsumerArn,
		ProcessFn:                            processFn,
		PopUrls:                              popUrls,
		HealthcheckTimeoutMilliseconds:       1000,
		WorkerInactivityTimeoutMilliseconds:  5000,
		CheckpointIntervalMilliseconds:       60000,
		CheckpointRetryIntervalMilliseconds:  1000,
		ApplyRequestTimeoutMilliseconds:      500,
		CheckpointRequestTimeoutMilliseconds: 500,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	logger := cfg.logger
	if logger == nil {
		l, err := zap.NewProduction()
		if err != nil {
			panic(err)
		}
		logger = l
	}

	if cfg.KinesisClient == nil {
		awsCfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			panic(err)
		}
		cfg.KinesisClient = kinesis.NewFromConfig(awsCfg)
	}

	return &Consumer{
		cfg:                  cfg,
		ready:                make(chan struct{}),
		done:                 make(chan struct{}),
		componentsDoneNotify: make(map[string]runnable),
		kds:                  cfg.KinesisClient,
		processFn:            processFn,
		logger:               logger.Named("consumer").With(zap.String("name", cfg.Name), zap.String("workerid", cfg.ID)),
	}
}

func MustNewStandaloneConsumer(name, streamName, efoConsumerArn string, processFn func(types.Record), opts ...func(*ConsumerConfig)) *Consumer {
	opts = append(opts, AsStandalone())
	return MustNewConsumer(name, streamName, efoConsumerArn, "", processFn, opts...)
}

func (c *Consumer) Start(ctx context.Context) error {
	go func() error {
		defer close(c.ready)
		if c.cfg.StandaloneConsumer {
			c.pop = NewPopService(c.cfg.Name, c.cfg.StreamName)
			err := c.pop.Start(ctx)
			if err != nil {
				return err
			}
			c.cfg.PopUrls = c.pop.cfg.PopUrls
		}
		worker := NewWorker(c.cfg, c.kds, c.logger)
		c.componentsDoneNotify["Worker"] = worker
		worker.Start(ctx)
		return nil
	}()

	return nil
}

func (c *Consumer) WaitForShutdownCompletion() {
	<-c.ready
	for k, v := range c.componentsDoneNotify {
		c.logger.Info("waiting for shutdown completion", zap.String("component", k))
		v.WaitForShutdownCompletion()
		c.logger.Info("shutdown completed", zap.String("component", k))
	}
	if c.pop != nil {
		c.pop.WaitForShutdownCompletion()
	}
}
