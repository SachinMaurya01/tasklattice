// Package worker runs the background processes: the outbox
// publisher, the SQS consumer with notification/reminder handlers, and
// periodic cleanup. Handlers stay idempotent; delivery is at-least-once.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// Queue is a thin wrapper around one SQS queue URL.
type Queue struct {
	client *sqs.Client
	url    string
}

// NewQueue binds a client to a queue URL.
func NewQueue(client *sqs.Client, url string) *Queue {
	return &Queue{client: client, url: url}
}

// NewSQSClient builds an SQS client for a region, optionally aimed at a
// custom endpoint (ElasticMQ/LocalStack) for local development.
func NewSQSClient(ctx context.Context, region, endpoint string) (*sqs.Client, error) {
	var opts []func(*awsconfig.LoadOptions) error
	opts = append(opts, awsconfig.WithRegion(region))
	if strings.TrimSpace(endpoint) != "" {
		opts = append(opts, awsconfig.WithBaseEndpoint(strings.TrimSpace(endpoint)))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return sqs.NewFromConfig(cfg), nil
}

// QueueName derives the queue name from its URL (last path segment).
func QueueName(url string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(url), "/")
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}

// EnsureQueues creates the main and DLQ queues when missing and wires the
// redrive policy (maxReceiveCount) so poison messages land in the DLQ.
func EnsureQueues(ctx context.Context, client *sqs.Client, queueURL, dlqURL string, maxReceiveCount int) error {
	mainName := QueueName(queueURL)
	mainOut, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(mainName)})
	if err != nil {
		return fmt.Errorf("ensure main queue: %w", err)
	}
	mainURL := aws.ToString(mainOut.QueueUrl)
	if mainURL == "" {
		mainURL = queueURL
	}

	dlqName := QueueName(dlqURL)
	dlqOut, err := client.CreateQueue(ctx, &sqs.CreateQueueInput{QueueName: aws.String(dlqName)})
	if err != nil {
		return fmt.Errorf("ensure DLQ: %w", err)
	}
	dlqURLResolved := aws.ToString(dlqOut.QueueUrl)
	if dlqURLResolved == "" {
		dlqURLResolved = dlqURL
	}

	arnOut, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(dlqURLResolved),
		AttributeNames: []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameQueueArn},
	})
	if err != nil {
		return fmt.Errorf("describe DLQ: %w", err)
	}
	arn := arnOut.Attributes[string(sqstypes.QueueAttributeNameQueueArn)]
	if arn == "" {
		return fmt.Errorf("DLQ ARN missing for %s", dlqURLResolved)
	}
	policy, err := json.Marshal(map[string]string{
		"deadLetterTargetArn": arn,
		"maxReceiveCount":     fmt.Sprintf("%d", maxReceiveCount),
	})
	if err != nil {
		return fmt.Errorf("encode redrive policy: %w", err)
	}
	if _, err = client.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
		QueueUrl: aws.String(mainURL),
		Attributes: map[string]string{
			string(sqstypes.QueueAttributeNameRedrivePolicy): string(policy),
		},
	}); err != nil {
		return fmt.Errorf("set redrive policy: %w", err)
	}
	return nil
}

// Send publishes one message with the event type as an attribute. The
// event ID becomes the deduplication ID on FIFO queues.
func (q *Queue) Send(ctx context.Context, body, eventType, eventID string) error {
	in := &sqs.SendMessageInput{
		QueueUrl:    aws.String(q.url),
		MessageBody: aws.String(body),
		MessageAttributes: map[string]sqstypes.MessageAttributeValue{
			"event_type": {DataType: aws.String("String"), StringValue: aws.String(eventType)},
		},
	}
	if strings.HasSuffix(q.url, ".fifo") {
		in.MessageDeduplicationId = aws.String(eventID)
		in.MessageGroupId = aws.String(eventType)
	}
	_, err := q.client.SendMessage(ctx, in)
	return err
}

// Receive long-polls up to 10 messages with full attributes.
func (q *Queue) Receive(ctx context.Context, visibilityTimeout, waitSeconds int32) ([]sqstypes.Message, error) {
	out, err := q.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(q.url),
		MaxNumberOfMessages:   10,
		VisibilityTimeout:     visibilityTimeout,
		WaitTimeSeconds:       waitSeconds,
		AttributeNames:        []sqstypes.QueueAttributeName{sqstypes.QueueAttributeNameAll},
		MessageAttributeNames: []string{"All"},
	})
	if err != nil {
		return nil, err
	}
	return out.Messages, nil
}

// Delete removes a processed message.
func (q *Queue) Delete(ctx context.Context, receipt string) error {
	_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.url),
		ReceiptHandle: aws.String(receipt),
	})
	return err
}

// Defer makes a failed message visible again after delaySeconds.
func (q *Queue) Defer(ctx context.Context, receipt string, delaySeconds int32) error {
	_, err := q.client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(q.url),
		ReceiptHandle:     aws.String(receipt),
		VisibilityTimeout: delaySeconds,
	})
	return err
}
