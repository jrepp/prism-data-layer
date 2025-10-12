---
date: 2025-10-09
deciders: Platform Team
doc_uuid: 4b6223b5-4cb2-4523-8d66-afbdc2ed0b04
id: adr-042
project_id: prism-data-layer
status: Proposed
tags:
- backend
- queue
- sqs
- aws
- plugin
- messaging
title: 'ADR-042: AWS SQS Queue Backend Plugin'
---

## Context

Prism requires a queue backend for asynchronous message processing use cases such as:
- **Job Processing**: Background tasks, batch jobs, workflows
- **Event-Driven Architecture**: Decoupling microservices
- **Load Leveling**: Buffering requests during traffic spikes
- **Retry Logic**: Automatic retries with exponential backoff
- **Dead Letter Queues**: Handling failed messages

**AWS SQS (Simple Queue Service)** is a fully managed message queuing service that provides:
- **Standard Queues**: At-least-once delivery, best-effort ordering, unlimited throughput
- **FIFO Queues**: Exactly-once processing, strict ordering, 3,000 msg/sec with batching
- **Dead Letter Queues**: Automatic handling of failed messages
- **Long Polling**: Reduces empty receives and costs
- **Visibility Timeout**: Prevents duplicate processing
- **Message Attributes**: Metadata for routing and filtering

## Decision

Implement an **AWS SQS Queue Backend Plugin** for Prism that provides:

1. **Queue Abstraction Layer**: Unified API for send/receive/delete operations
2. **Standard + FIFO Support**: Both queue types available
3. **Batch Operations**: Send/receive up to 10 messages per API call
4. **Dead Letter Queues**: Automatic retry and failure handling
5. **Long Polling**: Efficient message retrieval
6. **AWS Integration**: IAM authentication, CloudWatch metrics, VPC endpoints

## Rationale

### Why SQS?

**Pros**:
- ✅ Fully managed (no infrastructure to manage)
- ✅ AWS native (seamless integration with Lambda, ECS, etc.)
- ✅ Unlimited scalability (standard queues)
- ✅ Low cost ($0.40 per million requests)
- ✅ High availability (distributed architecture)
- ✅ Simple API (no complex broker setup)
- ✅ Dead letter queues (built-in failure handling)

**Cons**:
- ❌ At-least-once delivery for standard queues (duplicates possible)
- ❌ No message routing (unlike RabbitMQ exchanges)
- ❌ Limited throughput for FIFO queues (3,000 msg/sec)
- ❌ AWS vendor lock-in

**Alternatives Considered**:

| Queue System | Pros | Cons | Verdict |
|--------------|------|------|---------|
| **RabbitMQ** | Rich routing, mature, self-hostable | Requires operational expertise, no managed AWS service | ❌ Rejected: Higher ops burden |
| **Kafka** | High throughput, event streaming, replay | Over-engineered for simple queueing, higher cost | ❌ Rejected: Too complex for job queues |
| **AWS SNS** | Pub/sub fanout, push-based | Not a queue (no retry logic), no message persistence | ❌ Rejected: Different use case |
| **Redis** | Fast, simple, key-value store | Not durable, requires self-management | ❌ Rejected: Not purpose-built for queues |
| **SQS** | Managed, simple, cost-effective, AWS-native | At-least-once delivery, no routing | ✅ **Accepted**: Best for AWS job queues |

### When to Use SQS Backend

**Use SQS for**:
- Background job processing (email sending, image processing)
- Asynchronous task queues (video transcoding, report generation)
- Decoupling microservices (order service → payment service)
- Load leveling (buffer traffic spikes)
- Retry logic (handle transient failures)

**Don't use SQS for**:
- Event streaming with replay (use Kafka)
- Real-time notifications (use WebSockets or SNS)
- Complex routing logic (use RabbitMQ)
- Transactions across queues (SQS has no distributed transactions)

## Queue Data Abstraction Layer

### Core Operations

```protobuf
syntax = "proto3";

package prism.queue.v1;

service QueueService {
  // Basic operations
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
  rpc ReceiveMessage(ReceiveMessageRequest) returns (ReceiveMessageResponse);
  rpc DeleteMessage(DeleteMessageRequest) returns (DeleteMessageResponse);

  // Batch operations
  rpc SendMessageBatch(SendMessageBatchRequest) returns (SendMessageBatchResponse);
  rpc DeleteMessageBatch(DeleteMessageBatchRequest) returns (DeleteMessageBatchResponse);

  // Queue management
  rpc CreateQueue(CreateQueueRequest) returns (CreateQueueResponse);
  rpc DeleteQueue(DeleteQueueRequest) returns (DeleteQueueResponse);
  rpc GetQueueAttributes(GetQueueAttributesRequest) returns (GetQueueAttributesResponse);

  // Dead letter queue
  rpc RedriveMessages(RedriveMessagesRequest) returns (RedriveMessagesResponse);
}

message SendMessageRequest {
  string queue_name = 1;
  string message_body = 2;  // Payload (max 256 KB)

  // Optional attributes
  int32 delay_seconds = 3;  // 0-900 seconds
  map<string, MessageAttribute> attributes = 4;

  // FIFO-specific
  string message_group_id = 10;  // Required for FIFO queues
  string message_deduplication_id = 11;  // Optional (SQS can auto-generate)
}

message MessageAttribute {
  oneof value {
    string string_value = 1;
    int64 number_value = 2;
    bytes binary_value = 3;
  }
  string data_type = 4;  // "String", "Number", "Binary"
}

message SendMessageResponse {
  string message_id = 1;
  string md5_of_body = 2;  // Checksum for verification
  string sequence_number = 3;  // FIFO-specific
}

message ReceiveMessageRequest {
  string queue_name = 1;
  int32 max_messages = 2;  // 1-10 messages
  int32 visibility_timeout = 3;  // 0-43200 seconds (12 hours)
  int32 wait_time_seconds = 4;  // 0-20 seconds (long polling)

  repeated string attribute_names = 10;  // Return specific attributes
}

message ReceiveMessageResponse {
  repeated Message messages = 1;
}

message Message {
  string message_id = 1;
  string receipt_handle = 2;  // Required for delete
  string body = 3;
  map<string, MessageAttribute> attributes = 4;

  int32 receive_count = 10;  // How many times message was received
  google.protobuf.Timestamp first_receive_timestamp = 11;
}

message DeleteMessageRequest {
  string queue_name = 1;
  string receipt_handle = 2;  // From ReceiveMessageResponse
}

message DeleteMessageResponse {
  bool success = 1;
}
```

### Example: Job Processing

**Producer** (send job to queue):
```go
import pb "prism/queue/v1"

func submitJob(client pb.QueueServiceClient, jobData string) error {
    req := &pb.SendMessageRequest{
        QueueName:   "image-processing-queue",
        MessageBody: jobData,  // JSON: {"image_url": "s3://...", "filters": ["resize", "watermark"]}
        Attributes: map[string]*pb.MessageAttribute{
            "JobType": {Value: &pb.MessageAttribute_StringValue{StringValue: "image_processing"}},
            "Priority": {Value: &pb.MessageAttribute_NumberValue{NumberValue: 5}},
        },
    }

    resp, err := client.SendMessage(context.Background(), req)
    if err != nil {
        return fmt.Errorf("failed to send message: %w", err)
    }

    log.Printf("Job submitted: %s", resp.MessageId)
    return nil
}
```

**Consumer** (process jobs from queue):
```go
func processJobs(client pb.QueueServiceClient) {
    for {
        // Receive messages (long polling with 20s wait)
        req := &pb.ReceiveMessageRequest{
            QueueName:         "image-processing-queue",
            MaxMessages:       10,  // Batch of 10
            VisibilityTimeout: 300, // 5 minutes to process
            WaitTimeSeconds:   20,  // Long polling
        }

        resp, err := client.ReceiveMessage(context.Background(), req)
        if err != nil {
            log.Printf("Error receiving messages: %v", err)
            continue
        }

        // Process each message
        for _, msg := range resp.Messages {
            if err := processMessage(msg); err != nil {
                log.Printf("Failed to process message %s: %v", msg.MessageId, err)
                continue  // Message will be redelivered after visibility timeout
            }

            // Delete message after successful processing
            deleteReq := &pb.DeleteMessageRequest{
                QueueName:     "image-processing-queue",
                ReceiptHandle: msg.ReceiptHandle,
            }
            client.DeleteMessage(context.Background(), deleteReq)
        }
    }
}
```

## Implementation

### Plugin Architecture

```go
// plugins/backends/sqs/plugin.go
package sqs

import (
    "context"
    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/sqs"
)

type SQSPlugin struct {
    config    *SQSConfig
    client    *sqs.Client
    namespace string
}

type SQSConfig struct {
    Region         string
    QueuePrefix    string  // Prefix for queue names (e.g., "prism-prod-")
    EnableDLQ      bool    // Enable dead letter queues
    MaxRetries     int     // Max receive count before DLQ
    FifoEnabled    bool    // Use FIFO queues by default
}

func (p *SQSPlugin) SendMessage(ctx context.Context, req *SendMessageRequest) (*SendMessageResponse, error) {
    queueURL, err := p.getQueueURL(ctx, req.QueueName)
    if err != nil {
        return nil, fmt.Errorf("failed to get queue URL: %w", err)
    }

    input := &sqs.SendMessageInput{
        QueueUrl:    aws.String(queueURL),
        MessageBody: aws.String(req.MessageBody),
    }

    // Optional delay
    if req.DelaySeconds > 0 {
        input.DelaySeconds = aws.Int32(req.DelaySeconds)
    }

    // Message attributes
    if len(req.Attributes) > 0 {
        input.MessageAttributes = p.convertAttributes(req.Attributes)
    }

    // FIFO-specific
    if req.MessageGroupId != "" {
        input.MessageGroupId = aws.String(req.MessageGroupId)
    }
    if req.MessageDeduplicationId != "" {
        input.MessageDeduplicationId = aws.String(req.MessageDeduplicationId)
    }

    result, err := p.client.SendMessage(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to send message: %w", err)
    }

    return &SendMessageResponse{
        MessageId:      aws.ToString(result.MessageId),
        Md5OfBody:      aws.ToString(result.MD5OfMessageBody),
        SequenceNumber: aws.ToString(result.SequenceNumber),
    }, nil
}

func (p *SQSPlugin) ReceiveMessage(ctx context.Context, req *ReceiveMessageRequest) (*ReceiveMessageResponse, error) {
    queueURL, err := p.getQueueURL(ctx, req.QueueName)
    if err != nil {
        return nil, fmt.Errorf("failed to get queue URL: %w", err)
    }

    input := &sqs.ReceiveMessageInput{
        QueueUrl:            aws.String(queueURL),
        MaxNumberOfMessages: aws.Int32(req.MaxMessages),
        VisibilityTimeout:   aws.Int32(req.VisibilityTimeout),
        WaitTimeSeconds:     aws.Int32(req.WaitTimeSeconds),  // Long polling
        AttributeNames:      []types.QueueAttributeName{types.QueueAttributeNameAll},
        MessageAttributeNames: []string{"All"},
    }

    result, err := p.client.ReceiveMessage(ctx, input)
    if err != nil {
        return nil, fmt.Errorf("failed to receive messages: %w", err)
    }

    messages := make([]*Message, len(result.Messages))
    for i, msg := range result.Messages {
        messages[i] = &Message{
            MessageId:     aws.ToString(msg.MessageId),
            ReceiptHandle: aws.ToString(msg.ReceiptHandle),
            Body:          aws.ToString(msg.Body),
            Attributes:    p.convertMessageAttributes(msg.MessageAttributes),
        }
    }

    return &ReceiveMessageResponse{Messages: messages}, nil
}

func (p *SQSPlugin) DeleteMessage(ctx context.Context, req *DeleteMessageRequest) (*DeleteMessageResponse, error) {
    queueURL, err := p.getQueueURL(ctx, req.QueueName)
    if err != nil {
        return nil, fmt.Errorf("failed to get queue URL: %w", err)
    }

    _, err = p.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
        QueueUrl:      aws.String(queueURL),
        ReceiptHandle: aws.String(req.ReceiptHandle),
    })

    return &DeleteMessageResponse{Success: err == nil}, err
}

func (p *SQSPlugin) SendMessageBatch(ctx context.Context, req *SendMessageBatchRequest) (*SendMessageBatchResponse, error) {
    queueURL, err := p.getQueueURL(ctx, req.QueueName)
    if err != nil {
        return nil, fmt.Errorf("failed to get queue URL: %w", err)
    }

    // Build batch entries (max 10 per request)
    entries := make([]types.SendMessageBatchRequestEntry, len(req.Messages))
    for i, msg := range req.Messages {
        entries[i] = types.SendMessageBatchRequestEntry{
            Id:          aws.String(fmt.Sprintf("msg-%d", i)),
            MessageBody: aws.String(msg.MessageBody),
        }

        if msg.DelaySeconds > 0 {
            entries[i].DelaySeconds = aws.Int32(msg.DelaySeconds)
        }

        if msg.MessageGroupId != "" {
            entries[i].MessageGroupId = aws.String(msg.MessageGroupId)
        }
    }

    result, err := p.client.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
        QueueUrl: aws.String(queueURL),
        Entries:  entries,
    })

    if err != nil {
        return nil, fmt.Errorf("failed to send batch: %w", err)
    }

    // Map results
    responses := make([]*SendMessageResponse, len(result.Successful))
    for i, success := range result.Successful {
        responses[i] = &SendMessageResponse{
            MessageId:      aws.ToString(success.MessageId),
            Md5OfBody:      aws.ToString(success.MD5OfMessageBody),
            SequenceNumber: aws.ToString(success.SequenceNumber),
        }
    }

    return &SendMessageBatchResponse{
        Successful: responses,
        Failed:     len(result.Failed),
    }, nil
}
```

### Queue Creation with Dead Letter Queue

```go
func (p *SQSPlugin) CreateQueue(ctx context.Context, req *CreateQueueRequest) (*CreateQueueResponse, error) {
    queueName := p.config.QueuePrefix + req.QueueName

    // Determine queue type (standard or FIFO)
    if req.FifoQueue || p.config.FifoEnabled {
        queueName += ".fifo"
    }

    attributes := map[string]string{
        "VisibilityTimeout":            "300",  // 5 minutes
        "MessageRetentionPeriod":       "345600",  // 4 days
        "ReceiveMessageWaitTimeSeconds": "20",  // Long polling
    }

    // FIFO-specific attributes
    if req.FifoQueue {
        attributes["FifoQueue"] = "true"
        attributes["ContentBasedDeduplication"] = "true"  // Auto-generate dedup IDs
    }

    // Create main queue
    createResult, err := p.client.CreateQueue(ctx, &sqs.CreateQueueInput{
        QueueName:  aws.String(queueName),
        Attributes: attributes,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create queue: %w", err)
    }

    queueURL := aws.ToString(createResult.QueueUrl)

    // Create dead letter queue if enabled
    if p.config.EnableDLQ {
        dlqName := queueName + "-dlq"
        dlqResult, err := p.client.CreateQueue(ctx, &sqs.CreateQueueInput{
            QueueName:  aws.String(dlqName),
            Attributes: attributes,  // Same config as main queue
        })
        if err != nil {
            return nil, fmt.Errorf("failed to create DLQ: %w", err)
        }

        // Get DLQ ARN
        dlqAttrs, err := p.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
            QueueUrl:       dlqResult.QueueUrl,
            AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn},
        })
        if err != nil {
            return nil, fmt.Errorf("failed to get DLQ ARN: %w", err)
        }

        dlqArn := dlqAttrs.Attributes[string(types.QueueAttributeNameQueueArn)]

        // Configure redrive policy on main queue
        redrivePolicy := fmt.Sprintf(`{"maxReceiveCount":"%d","deadLetterTargetArn":"%s"}`,
            p.config.MaxRetries, dlqArn)

        _, err = p.client.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
            QueueUrl: aws.String(queueURL),
            Attributes: map[string]string{
                "RedrivePolicy": redrivePolicy,
            },
        })
        if err != nil {
            return nil, fmt.Errorf("failed to set redrive policy: %w", err)
        }
    }

    return &CreateQueueResponse{
        QueueUrl:  queueURL,
        QueueName: queueName,
    }, nil
}
```

## Performance Considerations

### 1. Batch Operations

**Always batch when possible** to reduce API calls and costs:

```go
// Bad: Individual sends (10 API calls)
for i := 0; i < 10; i++ {
    client.SendMessage(ctx, &SendMessageRequest{...})
}

// Good: Batch send (1 API call)
client.SendMessageBatch(ctx, &SendMessageBatchRequest{
    Messages: [...10 messages...],
})
```

**Cost savings**:
- Individual sends: 10 × $0.0000004 = $0.000004
- Batch send: 1 × $0.0000004 = $0.0000004 (10x cheaper)

### 2. Long Polling

**Use long polling** to reduce empty receives:

```go
// Bad: Short polling (immediate return if empty)
req := &ReceiveMessageRequest{
    QueueName:       "jobs",
    WaitTimeSeconds: 0,  // Short polling
}

// Good: Long polling (wait up to 20s for messages)
req := &ReceiveMessageRequest{
    QueueName:       "jobs",
    WaitTimeSeconds: 20,  // Long polling
}
```

**Benefits**:
- Reduces empty receives by up to 99%
- Lowers API costs
- Lowers latency (immediate notification of new messages)

### 3. Visibility Timeout Tuning

**Set visibility timeout based on processing time**:

```go
// Processing takes ~2 minutes on average
req := &ReceiveMessageRequest{
    QueueName:         "jobs",
    VisibilityTimeout: 300,  // 5 minutes (2x processing time)
}
```

**Too short**: Message redelivered before processing completes
**Too long**: Failed messages delayed unnecessarily

### 4. Message Prefetching

**Prefetch multiple messages** to keep workers busy:

```go
// Worker pool with 10 workers
const numWorkers = 10

for {
    req := &ReceiveMessageRequest{
        QueueName:   "jobs",
        MaxMessages: 10,  // Fetch 10 messages (one per worker)
    }

    resp, err := client.ReceiveMessage(ctx, req)
    for _, msg := range resp.Messages {
        workerPool.Submit(func() {
            processMessage(msg)
        })
    }
}
```

## Cost Optimization

**SQS Pricing** (us-east-1, as of 2025):
- **Standard Queue**: $0.40 per million requests (first 1M free/month)
- **FIFO Queue**: $0.50 per million requests (no free tier)
- **Data Transfer**: $0.09/GB out to internet (free within AWS)

**Optimization Strategies**:
1. **Use batch operations** (10x cheaper per message)
2. **Enable long polling** (reduces empty receives)
3. **Delete messages promptly** (avoid unnecessary receives)
4. **Use standard queues** unless ordering is critical
5. **Leverage free tier** (1M requests/month)

**Example Cost** (standard queue):
- **Sends**: 10M messages/month = 10 requests = $0.004
- **Receives**: 10M long polls = 10 requests = $0.004
- **Deletes**: 10M deletes = 10 requests = $0.004
- **Total**: $0.012/month for 10M messages (with batching)

Compare to:
- **Without batching**: 30M requests = $12/month (1000x more!)

## Monitoring

### CloudWatch Metrics

```yaml
metrics:
  - sqs_approximate_number_of_messages_visible      # Messages in queue
  - sqs_approximate_age_of_oldest_message          # Age of oldest message
  - sqs_number_of_messages_sent                    # Send rate
  - sqs_number_of_messages_received                # Receive rate
  - sqs_number_of_messages_deleted                 # Delete rate
  - sqs_approximate_number_of_messages_not_visible # In-flight messages

alerts:
  - metric: sqs_approximate_number_of_messages_visible
    threshold: 1000
    action: scale_up_workers

  - metric: sqs_approximate_age_of_oldest_message
    threshold: 3600  # 1 hour
    action: alert_ops_team
```

### Dead Letter Queue Monitoring

```yaml
dlq_alerts:
  - queue: image-processing-dlq
    metric: sqs_approximate_number_of_messages_visible
    threshold: 10
    action: alert_devops_team
    message: "10+ messages in DLQ, investigate failures"
```

## Security Considerations

### 1. IAM Authentication

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sqs:SendMessage",
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes"
      ],
      "Resource": "arn:aws:sqs:us-east-1:123456789012:prism-*"
    }
  ]
}
```

### 2. Encryption at Rest

```go
attributes := map[string]string{
    "KmsMasterKeyId": "arn:aws:kms:us-east-1:123456789012:key/abc-123",  // Use KMS
    "KmsDataKeyReusePeriodSeconds": "300",  // 5 minutes
}
```

### 3. Encryption in Transit

All SQS communication uses HTTPS (TLS 1.2+).

### 4. VPC Endpoints

```yaml
# Deploy SQS VPC endpoint for private access
vpc_endpoint:
  service_name: com.amazonaws.us-east-1.sqs
  vpc_id: vpc-abc123
  subnet_ids: [subnet-1, subnet-2]
  security_groups: [sg-prism]
```

## Testing Strategy

### Unit Tests

```go
func TestSendMessage(t *testing.T) {
    mockSQS := &MockSQSClient{}
    plugin := &SQSPlugin{client: mockSQS}

    req := &SendMessageRequest{
        QueueName:   "test-queue",
        MessageBody: "test message",
    }

    resp, err := plugin.SendMessage(context.Background(), req)
    require.NoError(t, err)
    assert.NotEmpty(t, resp.MessageId)
}
```

### Integration Tests

```go
func TestQueueRoundTrip(t *testing.T) {
    plugin := setupRealSQS(t)  // Connect to test SQS queue

    // Send message
    sendReq := &SendMessageRequest{
        QueueName:   "test-queue",
        MessageBody: "integration test",
    }
    sendResp, err := plugin.SendMessage(context.Background(), sendReq)
    require.NoError(t, err)

    // Receive message
    recvReq := &ReceiveMessageRequest{
        QueueName:   "test-queue",
        MaxMessages: 1,
    }
    recvResp, err := plugin.ReceiveMessage(context.Background(), recvReq)
    require.NoError(t, err)
    assert.Len(t, recvResp.Messages, 1)
    assert.Equal(t, "integration test", recvResp.Messages[0].Body)

    // Delete message
    deleteReq := &DeleteMessageRequest{
        QueueName:     "test-queue",
        ReceiptHandle: recvResp.Messages[0].ReceiptHandle,
    }
    _, err = plugin.DeleteMessage(context.Background(), deleteReq)
    require.NoError(t, err)
}
```

## Migration Path

### Phase 1: Basic Operations (Week 1)
- Implement SendMessage, ReceiveMessage, DeleteMessage
- IAM authentication
- Standard queues only

### Phase 2: Advanced Features (Week 2)
- Batch operations
- FIFO queue support
- Long polling optimization

### Phase 3: Management (Week 3)
- CreateQueue with DLQ
- Queue attribute management
- Redrive messages from DLQ

### Phase 4: Production (Week 4)
- CloudWatch integration
- Cost optimization
- Performance tuning

## Consequences

### Positive
- ✅ Fully managed (no queue server to operate)
- ✅ Highly available (distributed architecture)
- ✅ Cost-effective ($0.40 per million requests)
- ✅ Simple API (easy to use)
- ✅ Dead letter queues (automatic failure handling)

### Negative
- ❌ At-least-once delivery (duplicates possible)
- ❌ No message routing (basic queue only)
- ❌ FIFO throughput limit (3,000 msg/sec)
- ❌ AWS vendor lock-in

### Neutral
- 🔄 Two queue types (standard vs FIFO) adds complexity but flexibility
- 🔄 Visibility timeout requires tuning per use case

## References

- [AWS SQS Documentation](https://docs.aws.amazon.com/sqs/)
- [SQS Best Practices](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-best-practices.html)
- [SQS FIFO Queues](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/FIFO-queues.html)
- [SQS Dead Letter Queues](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-dead-letter-queues.html)
- ADR-005: Backend Plugin Architecture
- ADR-025: Container Plugin Model

## Revision History

- 2025-10-09: Initial proposal for AWS SQS queue backend plugin