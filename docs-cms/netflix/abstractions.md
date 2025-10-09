Besides Key-Value (KV) and TimeSeries, Netflix has built several other abstractions on its Data Gateway platform, including a Distributed Counter and a Write-Ahead Log (WAL). This highlights the platform's versatility, allowing for different data access patterns beyond the two most commonly cited examples. 
Distributed Counter Abstraction
The Distributed Counter Abstraction is a specialized service built on the Data Gateway to handle counting immutable events at a massive scale. 
Purpose: Tracks and measures user interactions and business performance metrics with different trade-offs for speed and accuracy.
Architecture: Built on top of the TimeSeries abstraction, it logs each counting event as an immutable record in a durable storage system like Cassandra.
Counting modes: It offers several modes to meet different use cases, including:
Best-Effort Regional Counter: Built on EVCache (a distributed caching solution), it provides very low latency but approximate counts and lacks global consistency or durability. It is useful for scenarios like A/B testing where exact numbers are not critical.
Eventually Consistent Global Counter: Provides durability and global consistency at the cost of some latency by using a background rollup process that aggregates events stored in the TimeSeries abstraction.
Accurate Global Counter: An experimental approach that computes real-time deltas on top of the rolled-up count to provide stronger accuracy guarantees. 
Write-Ahead Log (WAL) Abstraction
The Write-Ahead Log is a crucial abstraction for ensuring the resilience and reliability of other services. 
Purpose: The WAL is designed to provide a reliable way to durably log events before they are processed by a system. This ensures that even if a service crashes, the event stream is not lost and can be reprocessed later.
Architecture: The WAL abstraction is deployed as shards, with each use case receiving its own isolated shard to prevent the "noisy neighbor" problem. It uses namespaces to apply the correct configuration for each use case.
Resilience: The WAL, like other abstractions, uses resilience techniques like adaptive load shedding to protect the system from traffic overloads. 
Other potential abstractions
While not as well-documented as the KV, TimeSeries, and Counter abstractions, other types of abstractions can be, and have been, built on the Data Gateway. 
Tree Abstraction: An early talk on data abstraction mentioned the concept of a "tree abstraction" used in conjunction with the KV abstraction to solve bigger problems.
UI Personalization Abstraction: This could be another layer built on top of the KV abstraction to solve specific personalization needs. 
In essence, the Data Gateway is a foundational platform that provides a standard set of tools for developing, deploying, and managing a variety of data-specific abstractions. The KV and TimeSeries abstractions are just two examples, with the Counter and WAL being other notable services that leverage the same platform principles
