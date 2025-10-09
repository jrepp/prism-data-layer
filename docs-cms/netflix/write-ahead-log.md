https://netflixtechblog.com/building-a-resilient-data-platform-with-write-ahead-log-at-netflix-127b6712359a

Netflix's Write-Ahead Log (WAL) is a distributed, generic platform built on the Data Gateway framework, designed to ensure durable, ordered, and reliable delivery of data mutations. It acts as a resilient buffer between a client application and a target datastore, absorbing and replaying data changes to protect against downstream failures and system inconsistencies. Unlike a traditional database WAL, Netflix's version is an abstracted, pluggable service that decouples the application from the underlying queueing and storage technologies. 
Building a Resilient Data Platform with Write-Ahead Log at ...
Building a Resilient Data Platform with Write-Ahead Log at ...
Building a Resilient Data Platform with Write-Ahead Log at ...
Core architecture
Namespace-driven configuration
Abstraction Layer: The WAL provides a simple WriteToLog API endpoint that abstracts all the underlying complexity.
Logical Separation: A "namespace" is used to provide logical separation for different use cases and define where and how data is stored.
Pluggable Backends: Each namespace can be configured to use different queueing technologies, such as Apache Kafka, AWS SQS, or a combination of multiple. This allows the platform team to optimize for performance, durability, and cost without any application code changes.
Centralized Configuration: The namespace serves as a central hub for configuring settings like retry backoff multipliers and the maximum number of retry attempts. 
Separation of concerns
The WAL separates message producers and consumers. Producers receive client requests and place them in a queue, while consumers process messages from the queue and send them to target datastores. This separation enables independent scaling of producer and consumer groups for each shard based on resource usage. 
Under the hood
The WAL handles ordered mutations, particularly for complex requests. This involves tagging individual operations with sequence numbers, using a completion marker, persisting the state to durable storage, and reconstructing/applying the mutations in order via the consumer. 
Key functionality and use cases
1. System entropy management
The WAL helps maintain consistency between different datastores by handling asynchronous mutations and retries from a single write by the application. 
2. Generic data replication
It provides a generic replication solution for datastores without built-in capabilities, forwarding mutations in-region or cross-region. 
3. Data corruption and incident recovery
The WAL acts as a replayable log of mutations for recovering from database corruption. This allows restoring from a backup and replaying WAL mutations, with the option to omit faulty ones. 
4. Asynchronous processing and delayed queues
The WAL can smooth traffic spikes, act as a delayed queue for requests like bulk deletes with added delay and jitter, and abstract away retry logic for real-time pipelines. 
Resiliency built-in
The WAL incorporates several resiliency features:
Automatic scaling of producers and consumers.
Integration with adaptive load shedding to prevent overload.
Dedicated Dead Letter Queues (DLQs) for handling errors. 
Netflix's WAL is a platform tool that improves the reliability and resilience of its data ecosystem by centralizing the management of durability, consistency, and retries.
