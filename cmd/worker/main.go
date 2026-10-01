package main

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"tasklattice/internal/config"
	"tasklattice/internal/database"
	"tasklattice/internal/repository"
	"tasklattice/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}
	if cfg.SQSQueueURL == "" || cfg.SQSDLQURL == "" {
		log.Fatal("SQS_QUEUE_URL and SQS_DLQ_URL are required for the worker")
	}

	pool, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sqsClient, err := worker.NewSQSClient(ctx, cfg.AWSRegion, cfg.AWSEndpointURL)
	if err != nil {
		log.Fatalf("SQS client failed: %v", err)
	}
	if err := worker.EnsureQueues(ctx, sqsClient, cfg.SQSQueueURL, cfg.SQSDLQURL, cfg.SQSMaxReceiveCount); err != nil {
		log.Fatalf("SQS setup failed: %v", err)
	}

	outboxRepo := repository.NewOutboxRepository(pool)
	idemRepo := repository.NewIdempotencyRepository(pool)

	publisher := worker.NewPublisher(outboxRepo, worker.NewQueue(sqsClient, cfg.SQSQueueURL), cfg.WorkerOutboxBatch, cfg.WorkerPollInterval)
	consumer := worker.NewConsumer(worker.NewQueue(sqsClient, cfg.SQSQueueURL), outboxRepo, "worker", cfg.SQSVisibilityTimeout, 5)
	worker.RegisterHandlers(consumer)
	cleanup := worker.NewCleanup(pool, idemRepo, time.Hour)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); publisher.Run(ctx) }()
	go func() { defer wg.Done(); consumer.Run(ctx) }()
	go func() { defer wg.Done(); cleanup.Run(ctx) }()
	log.Println("Worker started")

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()
	log.Println("Shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	go func() {
		<-shutdownCtx.Done()
		if shutdownCtx.Err() == context.DeadlineExceeded {
			log.Println("Graceful shutdown timed out")
		}
	}()

	cancel()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		log.Println("Worker stopped")
	case <-shutdownCtx.Done():
		log.Println("Worker stop forced by timeout")
	}
}
