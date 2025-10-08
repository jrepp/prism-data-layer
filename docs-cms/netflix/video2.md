The video explains how Netflix built its Real-Time Distributed Graph (RDG) to handle the increasing complexity of its business, which now includes gaming, ad-supported plans, and live events, beyond its traditional streaming service.

The RDG aims to:

Intuitively represent highly interconnected data across the company (5:35).
Power in-the-moment experiences with near real-time data and fast insights (5:50).
Handle ever-growing data volumes at global scale (6:01).
The architecture of the RDG has three main layers:

Ingestion and Processing: Uses Apache Kafka for streaming events and Apache Flink for real-time processing to generate graph nodes and edges (6:18).
Storage: Leverages Netflix's internal KV Data Abstraction Layer (KV Doll) to store nodes and edges, chosen for its scalability and low latency. Nodes are modeled using unique identifiers, and edges use an adjacency list format (10:02).
Serving: Provides a gRPC API for online, real-time graph traversals (using breadth-first search) and saves daily copies of data to Iceberg tables for offline use cases like ETL and analysis (14:14).
The video also touches on challenges faced, such as choosing the right data sources, tuning Flink jobs, and backfilling data using Apache Spark for better performance (16:41). The current RDG setup manages over 8 billion nodes and 150 billion edges, supporting millions of reads and writes per second (15:35).

To access the full transcript of the video, please visit the video's description.


This talk is a part of the Data Engineering Open Forum 2025 at Netflix.

Speakers:
Luis Medina, Senior Data Engineer at Netflix
Adrian Taruc, Senior Data Engineer at Netflix

Abstract:
As a company grows, so does the quantity and complexity of its data. There can sometimes exist hidden relationships in this data that can be used to build better products and create more meaningful user experiences. However, it often becomes difficult to leverage its interconnectedness due to the limitations of the systems where it resides. Data warehouses, for example, excel at handling large volumes of data but they lack in things like speed. Other systems like non-relational databases can handle large volumes of data for faster access (at least compared to relational databases). However, this access usually favors only one side of the read/write path, especially when under heavy load, and their APIs are usually not flexible enough to effectively connect data together.

At Netflix, we built a real-time, distributed graph (RTDG) system to overcome these limitations. In order to achieve the scale and resiliency needed, we leveraged several technologies including Kafka and Flink for near-real-time data ingestion together with Netflix’s Key-Value Abstraction Layer (backed by Cassandra) for storage.

In this talk, you will learn more about our journey building this RTDG including how we designed the architecture that powers it, the various challenges we faced, the trade-offs we made, and how this type of system can be applied to real-world problems such as fraud detection and recommendation engines.

—

If you are interested in attending a future Data Engineering Open Forum, we highly recommend you join our Google Group (https://groups.google.com/g/data-engi...) to stay tuned to event announcements.
