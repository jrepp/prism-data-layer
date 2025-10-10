---
title: Prism Data Access Layer - Flow Diagrams
tags: [diagrams, architecture, workflows]
---

This document contains sequence and architecture diagrams for Prism's key workflows.

## Client Configuration and Session Flow

### Sequence: Client Connection with Named Configuration

```text
┌──────┐                  ┌──────────┐                  ┌──────────┐
│Client│                  │  Prism   │                  │ Backend  │
│      │                  │  Proxy   │                  │ (Postgres│
└──┬───┘                  └────┬─────┘                  └────┬─────┘
   │                           │                             │
   │  1. GetConfig("user-profiles")                         │
   ├──────────────────────────>│                             │
   │                           │                             │
   │  2. Return ClientConfig   │                             │
   │<──────────────────────────┤                             │
   │  {                        │                             │
   │    pattern: KEY_VALUE     │                             │
   │    backend: POSTGRES      │                             │
   │    consistency: STRONG    │                             │
   │  }                        │                             │
   │                           │                             │
   │  3. CreateSession(config, auth)                        │
   ├──────────────────────────>│                             │
   │                           │                             │
   │                           │  4. Validate auth           │
   │                           ├─────────────┐               │
   │                           │             │               │
   │                           │<────────────┘               │
   │                           │                             │
   │                           │  5. Initialize backend conn │
   │                           ├────────────────────────────>│
   │                           │                             │
   │                           │  6. Connection established  │
   │                           │<────────────────────────────┤
   │                           │                             │
   │                           │  7. Create session state    │
   │                           ├─────────────┐               │
   │                           │             │               │
   │                           │<────────────┘               │
   │                           │                             │
   │  8. Return session token  │                             │
   │<──────────────────────────┤                             │
   │  {                        │                             │
   │    session_token: "abc..."│                             │
   │    session_id: "123"      │                             │
   │    expires_at: ...        │                             │
   │  }                        │                             │
   │                           │                             │
   │  9. Put(session_token, data)                           │
   ├──────────────────────────>│                             │
   │                           │                             │
   │                           │  10. Validate session       │
   │                           ├─────────────┐               │
   │                           │             │               │
   │                           │<────────────┘               │
   │                           │                             │
   │                           │  11. INSERT INTO table      │
   │                           ├────────────────────────────>│
   │                           │                             │
   │                           │  12. Rows affected          │
   │                           │<────────────────────────────┤
   │                           │                             │
   │                           │  13. Audit log              │
   │                           ├─────────────┐               │
   │                           │             │               │
   │                           │<────────────┘               │
   │                           │                             │
   │  14. PutResponse          │                             │
   │<──────────────────────────┤                             │
   │                           │                             │
```

### Sequence: Client with Inline Configuration

```text
┌──────┐                  ┌──────────┐                  ┌──────────┐
│Client│                  │  Prism   │                  │ Backend  │
│      │                  │  Proxy   │                  │ (Kafka)  │
└──┬───┘                  └────┬─────┘                  └────┬─────┘
   │                           │                             │
   │  1. CreateSession(        │                             │
   │     auth,                 │                             │
   │     inline_config: {      │                             │
   │       pattern: QUEUE      │                             │
   │       backend: KAFKA      │                             │
   │     })                    │                             │
   ├──────────────────────────>│                             │
   │                           │                             │
   │                           │  2. Validate config         │
   │                           ├─────────────┐               │
   │                           │             │               │
   │                           │<────────────┘               │
   │                           │                             │
   │                           │  3. Connect to Kafka        │
   │                           ├────────────────────────────>│
   │                           │                             │
   │                           │  4. Producer ready          │
   │                           │<────────────────────────────┤
   │                           │                             │
   │  5. Session token         │                             │
   │<──────────────────────────┤                             │
   │                           │                             │
   │  6. Publish(session_token, msg)                        │
   ├──────────────────────────>│                             │
   │                           │                             │
   │                           │  7. Send to topic           │
   │                           ├────────────────────────────>│
   │                           │                             │
   │                           │  8. Offset + partition      │
   │                           │<────────────────────────────┤
   │                           │                             │
   │  9. PublishResponse       │                             │
   │<──────────────────────────┤                             │
   │                           │                             │
```

## Queue Operations Flow

### Kafka Publisher Flow

```text
┌───────────┐         ┌──────────┐         ┌────────────┐         ┌────────┐
│  Client   │         │  Prism   │         │   Kafka    │         │ Kafka  │
│Application│         │  Proxy   │         │ Publisher  │         │Cluster │
│           │         │          │         │(Container) │         │        │
└─────┬─────┘         └────┬─────┘         └─────┬──────┘         └───┬────┘
      │                    │                     │                    │
      │  Publish(msg)      │                     │                    │
      ├───────────────────>│                     │                    │
      │                    │                     │                    │
      │                    │  Enqueue internal   │                    │
      │                    ├────────────────────>│                    │
      │                    │                     │                    │
      │  Response          │                     │  Produce to topic  │
      │<───────────────────┤                     ├───────────────────>│
      │                    │                     │                    │
      │                    │                     │  Offset + part     │
      │                    │                     │<───────────────────┤
      │                    │                     │                    │
      │                    │  Ack message        │                    │
      │                    │<────────────────────┤                    │
      │                    │                     │                    │
```

### Kafka Consumer Flow

```text
┌────────┐         ┌────────────┐         ┌──────────┐         ┌───────────┐
│ Kafka  │         │   Kafka    │         │  Prism   │         │  Client   │
│Cluster │         │ Consumer   │         │  Proxy   │         │Application│
│        │         │(Container) │         │          │         │           │
└───┬────┘         └─────┬──────┘         └────┬─────┘         └─────┬─────┘
    │                    │                     │                     │
    │  Poll messages     │                     │                     │
    │<───────────────────┤                     │                     │
    │                    │                     │                     │
    │  Message batch     │                     │                     │
    ├───────────────────>│                     │                     │
    │                    │                     │                     │
    │                    │  Forward to proxy   │                     │
    │                    ├────────────────────>│                     │
    │                    │                     │                     │
    │                    │                     │  Subscribe(topic)   │
    │                    │                     │<────────────────────┤
    │                    │                     │                     │
    │                    │                     │  Stream messages    │
    │                    │                     ├────────────────────>│
    │                    │                     │                     │
    │                    │                     │  Ack message        │
    │                    │                     │<────────────────────┤
    │                    │                     │                     │
    │                    │  Commit offset      │                     │
    │                    ├────────────────────>│                     │
    │                    │                     │                     │
    │  Commit            │                     │                     │
    │<───────────────────┤                     │                     │
    │                    │                     │                     │
```

## Transactional Write Flow

### Two-Table Transaction (Inbox/Outbox Pattern)

```text
┌──────┐         ┌──────────┐         ┌────────────┐         ┌──────────┐
│Client│         │  Prism   │         │Transaction │         │ Postgres │
│      │         │  Proxy   │         │ Processor  │         │ Database │
└──┬───┘         └────┬─────┘         └─────┬──────┘         └────┬─────┘
   │                  │                     │                     │
   │  Write({         │                     │                     │
   │    data: {...}   │                     │                     │
   │    mailbox: {...}│                     │                     │
   │  })              │                     │                     │
   ├─────────────────>│                     │                     │
   │                  │                     │                     │
   │                  │  Process transaction│                     │
   │                  ├────────────────────>│                     │
   │                  │                     │                     │
   │                  │                     │  BEGIN              │
   │                  │                     ├────────────────────>│
   │                  │                     │                     │
   │                  │                     │  INSERT data_table  │
   │                  │                     ├────────────────────>│
   │                  │                     │                     │
   │                  │                     │  Rows affected      │
   │                  │                     │<────────────────────┤
   │                  │                     │                     │
   │                  │                     │  INSERT mailbox     │
   │                  │                     ├────────────────────>│
   │                  │                     │                     │
   │                  │                     │  Message ID + seq   │
   │                  │                     │<────────────────────┤
   │                  │                     │                     │
   │                  │                     │  COMMIT             │
   │                  │                     ├────────────────────>│
   │                  │                     │                     │
   │                  │                     │  Success            │
   │                  │                     │<────────────────────┤
   │                  │                     │                     │
   │                  │  WriteResponse      │                     │
   │                  │<────────────────────┤                     │
   │                  │                     │                     │
   │  Response        │                     │                     │
   │<─────────────────┤                     │                     │
   │                  │                     │                     │
   │                  │                     │                     │
   │                  │         ┌────────────────────┐            │
   │                  │         │   Mailbox          │            │
   │                  │         │   Listener         │            │
   │                  │         │  (Container)       │            │
   │                  │         └─────┬──────────────┘            │
   │                  │               │                           │
   │                  │               │  SELECT new messages      │
   │                  │               ├──────────────────────────>│
   │                  │               │                           │
   │                  │               │  Message rows             │
   │                  │               │<──────────────────────────┤
   │                  │               │                           │
   │                  │               │  Process messages         │
   │                  │               ├─────────────┐             │
   │                  │               │             │             │
   │                  │               │<────────────┘             │
   │                  │               │                           │
   │                  │               │  UPDATE processed=true    │
   │                  │               ├──────────────────────────>│
   │                  │               │                           │
```

## Paged Reader Flow

### Streaming Pagination

```text
┌──────┐         ┌──────────┐         ┌────────────┐         ┌──────────┐
│Client│         │  Prism   │         │  Indexed   │         │ Database │
│      │         │  Proxy   │         │  Reader    │         │          │
└──┬───┘         └────┬─────┘         └─────┬──────┘         └────┬─────┘
   │                  │                     │                     │
   │  Read(page_size=100)                  │                     │
   ├─────────────────>│                     │                     │
   │                  │                     │                     │
   │                  │  Request page 1     │                     │
   │                  ├────────────────────>│                     │
   │                  │                     │                     │
   │                  │                     │  SELECT LIMIT 100   │
   │                  │                     ├────────────────────>│
   │                  │                     │                     │
   │                  │                     │  100 rows           │
   │                  │                     │<────────────────────┤
   │                  │                     │                     │
   │  Stream Page 1   │                     │                     │
   │<─────────────────┤<────────────────────┤                     │
   │                  │                     │                     │
   │                  │  Request page 2     │                     │
   │                  ├────────────────────>│                     │
   │                  │                     │                     │
   │                  │                     │  SELECT OFFSET 100  │
   │                  │                     ├────────────────────>│
   │                  │                     │                     │
   │                  │                     │  100 rows           │
   │                  │                     │<────────────────────┤
   │                  │                     │                     │
   │  Stream Page 2   │                     │                     │
   │<─────────────────┤<────────────────────┤                     │
   │                  │                     │                     │
   │  ...             │  ...                │  ...                │
   │                  │                     │                     │
   │                  │  Request page N     │                     │
   │                  ├────────────────────>│                     │
   │                  │                     │                     │
   │                  │                     │  SELECT OFFSET N    │
   │                  │                     ├────────────────────>│
   │                  │                     │                     │
   │                  │                     │  0 rows (done)      │
   │                  │                     │<────────────────────┤
   │                  │                     │                     │
   │  Stream complete │                     │                     │
   │<─────────────────┤<────────────────────┤                     │
   │                  │                     │                     │
```

## Complete System Architecture

```text
┌─────────────────────────────────────────────────────────────────────┐
│                         Client Applications                         │
│                                                                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐           │
│  │ Go Client│  │Rust Client│ │Py Client │  │JS Client │           │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────┬─────┘           │
└───────┼─────────────┼─────────────┼─────────────┼──────────────────┘
        │             │             │             │
        │             │ gRPC/HTTP2  │             │
        └─────────────┴─────────────┴─────────────┘
                      │
        ┌─────────────▼──────────────┐
        │    Prism Proxy (Rust)      │
        │    gRPC Server :8980       │
        │                            │
        │  ┌──────────────────────┐  │
        │  │ Configuration Svc    │  │
        │  └──────────────────────┘  │
        │  ┌──────────────────────┐  │
        │  │ Session Service      │  │
        │  └──────────────────────┘  │
        │  ┌──────────────────────┐  │
        │  │ Queue Service        │  │
        │  └──────────────────────┘  │
        │  ┌──────────────────────┐  │
        │  │ PubSub Service       │  │
        │  └──────────────────────┘  │
        │  ┌──────────────────────┐  │
        │  │ Reader Service       │  │
        │  └──────────────────────┘  │
        │  ┌──────────────────────┐  │
        │  │ Transact Service     │  │
        │  └──────────────────────┘  │
        └────────────┬───────────────┘
                     │
        ┌────────────┴────────────┐
        │                         │
        ▼                         ▼
┌───────────────┐       ┌───────────────┐
│ Plugin Layer  │       │ Plugin Layer  │
│ (Containers)  │       │ (Containers)  │
│               │       │               │
│ ┌───────────┐ │       │ ┌───────────┐ │
│ │  Kafka    │ │       │ │   NATS    │ │
│ │ Publisher │ │       │ │ Publisher │ │
│ └───────────┘ │       │ └───────────┘ │
│               │       │               │
│ ┌───────────┐ │       │ ┌───────────┐ │
│ │  Kafka    │ │       │ │   NATS    │ │
│ │ Consumer  │ │       │ │ Consumer  │ │
│ └───────────┘ │       │ └───────────┘ │
└───────┬───────┘       └───────┬───────┘
        │                       │
        ▼                       ▼
┌──────────────┐       ┌──────────────┐
│    Kafka     │       │     NATS     │
│   Cluster    │       │   Cluster    │
└──────────────┘       └──────────────┘

┌───────────────┐       ┌───────────────┐
│ Plugin Layer  │       │ Plugin Layer  │
│ (Containers)  │       │ (Containers)  │
│               │       │               │
│ ┌───────────┐ │       │ ┌───────────┐ │
│ │  Indexed  │ │       │ │Transaction│ │
│ │  Reader   │ │       │ │ Processor │ │
│ └───────────┘ │       │ └───────────┘ │
│               │       │               │
│               │       │ ┌───────────┐ │
│               │       │ │  Mailbox  │ │
│               │       │ │ Listener  │ │
│               │       │ └───────────┘ │
└───────┬───────┘       └───────┬───────┘
        │                       │
        ▼                       ▼
┌──────────────┐       ┌──────────────┐
│  PostgreSQL  │       │  PostgreSQL  │
│   Database   │       │   Database   │
└──────────────┘       └──────────────┘
```

## Notes

- All diagrams use ASCII art for maximum portability
- Sequence diagrams follow left-to-right temporal flow
- Architecture diagrams show deployment topology
- Numbers in sequence diagrams indicate chronological order
- Container plugins are horizontally scalable (deploy multiple replicas)
