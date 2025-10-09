An excellent example of the dual-write pattern is Netflix's migration of its invoicing data from a legacy MySQL database to a new Apache Cassandra instance. The Data Gateway played a central role in abstracting this process from the application services, allowing for a seamless, low-downtime transition. 
The migration problem
Netflix needed to migrate a large and high-change-rate invoicing dataset from its legacy MySQL database to a Cassandra instance. The main challenge was to perform this migration without disrupting customer access to their billing history. 
How the Data Gateway implemented dual-writes
Phase 1: Dual-writes enabled through the "billing gateway"
Abstraction: The Data Gateway, in this case acting as a "billing gateway," was placed in front of both the legacy service (connected to MySQL) and the new service (connected to Cassandra).
Dual-write logic: The gateway was configured to intercept all write requests related to invoicing data. Instead of just sending the request to the old database, the gateway would forward the write to both the old MySQL and the new Cassandra instances simultaneously.
Data mapping: The gateway, or a related "data integrator" service, would handle any necessary data transformation to ensure that the data written to Cassandra conformed to its new schema, while the data going to MySQL maintained its original format.
Error handling and logging: The gateway and integrator were designed to handle failures. If a write to one database succeeded but the other failed, the failure would be logged, and a background reconciliation process would later correct the inconsistency. Metrics tracking mismatches were also critical during this phase. 
Phase 2: Read path transition
Initial read strategy: During the dual-write phase, the gateway continued to serve all read requests from the old MySQL database. The new Cassandra database was populated with both existing and new data, but it wasn't yet used for production reads.
Backfilling historical data: An offline or background process (forklifting) was run to copy all historical data from MySQL to Cassandra. This was performed during off-peak hours to minimize load.
Canary testing: As the Cassandra database was populated and validated, the gateway began to "canary release" the new read path. For instance, the gateway could direct read requests for a small, non-critical subset of users (e.g., in a specific country) to the new Cassandra service.
Validation and fallback: If a read request went to Cassandra but the data was missing or inconsistent, the gateway would fall back to reading from MySQL, ensuring service continuity. This provided a safety net while validating the new data store. 
Phase 3: Cutover and cleanup
Read switch: Once the confidence level was high and the data in Cassandra was verified for completeness and correctness, the gateway was configured to serve all read traffic from the new Cassandra database.
Write switch: All write requests were then directed exclusively to the new Cassandra instance, and the dual-write process was disabled.
Cleanup: The legacy MySQL database and its associated service could then be safely decommissioned. 
Key takeaways from this migration
Separation of concerns: The Data Gateway successfully separated the application logic from the database implementation, making the migration transparent to upstream services.
Safety via gradual release: The combination of dual-writes, canary releases, and fallback mechanisms allowed the Netflix team to manage risk and validate each stage of the migration without a high-stakes "big-bang" cutover.
Observability is paramount: Extensive metrics and logging for unexpected data issues were crucial to identifying and fixing problems throughout the process.
Automation: The process was heavily automated, from data backfilling to the phased release strategy, which is necessary for managing migration at Netflix's scale
