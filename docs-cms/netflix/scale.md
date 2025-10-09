---
title: "Netflix Data Gateway Scale Metrics"
sidebar_label: "Scale"
sidebar_position: 2
tags: [netflix, scale, performance, metrics]
---

Netflix's Data Gateway approach has enabled the company to achieve massive scale across a number of dimensions, supporting a global user base and billions of requests daily. It is a critical component of Netflix's microservice infrastructure, providing abstractions that manage the complexity and performance of multiple data stores. 
Here are some key indicators of the scale achieved:
Traffic and throughput
API Gateway traffic: The front-facing API gateway, which receives requests from user devices, has been reported to handle over 700,000 requests per second. This translates to over a billion API requests per day, highlighting the massive volume of traffic the entire system, including the DAL, must support.
Online datastore operations: As of 2024, the key-value abstraction layer—one of several built on the Data Gateway—supported over 8 million queries per second (QPS).
Specific datastore benchmarks: Performance metrics have been released for particular use cases and data stores. For example, a benchmark for one Cassandra cluster showed the ability to handle over a million writes per second.
High-throughput use cases: For time-series data, the platform manages up to 10 million writes per second while ensuring low-latency retrieval for petabytes of data. 
Use cases and deployment
Broad adoption: The data gateway approach is not limited to a single use case but is widely adopted across Netflix's ecosystem. As of 2024, the data access abstractions were used by over 3,500 use cases.
Diverse data needs: The platform supports a variety of data access needs, including key-value, time-series, and counters, showing its versatility.
Global reach: The Data Gateway manages global read and write operations, allowing for tunable consistency and supporting Netflix's worldwide streaming service.
Continuous evolution: The Data Gateway approach is foundational to how Netflix builds and scales new services. Recent posts, like the "100x Faster" redesign of the Maestro workflow engine, credit the simplified database access for improved performance and throughput. 
Efficiency and resilience
Optimized performance: The platform is engineered for high performance. For example, client-side compression is used to reduce payload sizes, with one case showing a 75% reduction in search-related traffic.
Handling extreme loads: The architecture is designed to manage bursty traffic from content launches and regional failovers. Features like automated load shedding and circuit breaking are built into the data layers to protect the underlying infrastructure during these spikes.
Operational savings: The move to a simplified, standardized approach has yielded significant operational benefits. In one instance, a specific migration led to a 90% reduction in internal database query traffic, which was a "significant source of system alerts" before. 
In summary, Netflix has achieved massive scale with its Data Gateway by building a platform that standardizes data access, abstracts complex data stores, and embeds resilience at the infrastructure level. This has allowed application teams to innovate at a high pace, while the data platform ensures high throughput, low latency, and reliability at a global level.
