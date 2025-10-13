---
date: 2025-10-05
deciders: Core Team
doc_uuid: 5da63362-c053-45d3-a554-b1699120e310
id: adr-010
project_id: prism-data-layer
status: Accepted
tags:
- performance
- architecture
title: Caching Layer Design
---

## Context

Many workloads are read-heavy with repeated access to the same data:
- User profiles fetched on every page load
- Configuration data read frequently
- Popular content accessed by millions

Caching reduces:
- Backend load (fewer database queries)
- Latency (memory faster than disk)
- Costs (fewer backend resources needed)

Netflix's KV DAL includes look-aside caching with EVCache (memcached).

**Problem**: Should Prism include caching, and if so, how?

## Decision

Implement **optional look-aside caching** at the proxy layer, configurable per-namespace.

## Rationale

### Look-Aside Cache Pattern

Read Path:
┌──────┐    ┌───────┐    ┌──────┐    ┌──────────┐
│Client│───▶│ Proxy │───▶│Cache │───▶│ Backend  │
└──────┘    └───────┘    └──────┘    └──────────┘
                │            │              │
                │    Cache   │              │
                │    Hit ────┘              │
                │                           │
                │    Cache Miss ────────────┘
                │                           │
                │    Populate Cache ◀───────┘
                │
                ▼
             Response

Write Path:
┌──────┐    ┌───────┐    ┌──────┐    ┌──────────┐
│Client│───▶│ Proxy │───▶│Backend───▶│ (Write)  │
└──────┘    └───────┘    └──────┘    └──────────┘
                │            │
                │    Invalidate
                └───────────▶│
```text

### Cache Configuration

```
namespace: user-profiles

cache:
  enabled: true
  backend: redis  # or memcached
  ttl_seconds: 300  # 5 minutes
  max_item_size_bytes: 1048576  # 1 MB

  # Invalidation strategy
  invalidation: write_through  # or ttl_only

  # Connection
  connection:
    endpoints: [redis://cache-cluster-1:6379]
    pool_size: 50
```text

### Implementation

```
#[async_trait]
pub trait CacheBackend: Send + Sync {
    async fn get(&self, key: &str) -> Result<Option<Vec<u8>>>;
    async fn set(&self, key: &str, value: &[u8], ttl: Duration) -> Result<()>;
    async fn delete(&self, key: &str) -> Result<()>;
}

pub struct RedisCache {
    pool: redis::aio::ConnectionManager,
}

#[async_trait]
impl CacheBackend for RedisCache {
    async fn get(&self, key: &str) -> Result<Option<Vec<u8>>> {
        let mut conn = self.pool.clone();
        let result: Option<Vec<u8>> = conn.get(key).await?;
        Ok(result)
    }

    async fn set(&self, key: &str, value: &[u8], ttl: Duration) -> Result<()> {
        let mut conn = self.pool.clone();
        conn.set_ex(key, value, ttl.as_secs() as usize).await?;
        Ok(())
    }

    async fn delete(&self, key: &str) -> Result<()> {
        let mut conn = self.pool.clone();
        conn.del(key).await?;
        Ok(())
    }
}
```text

### Cache-Aware Backend Wrapper

```
pub struct CachedBackend<B: KeyValueBackend> {
    backend: B,
    cache: Option<Arc<dyn CacheBackend>>,
    config: CacheConfig,
}

#[async_trait]
impl<B: KeyValueBackend> KeyValueBackend for CachedBackend<B> {
    async fn get(&self, namespace: &str, id: &str, keys: Vec<&[u8]>) -> Result<Vec<Item>> {
        let cache = match &self.cache {
            Some(c) => c,
            None => return self.backend.get(namespace, id, keys).await,
        };

        let mut cached_items = Vec::new();
        let mut missing_keys = Vec::new();

        // Check cache for each key
        for key in &keys {
            let cache_key = format!("{}:{}:{}", namespace, id, hex::encode(key));

            match cache.get(&cache_key).await? {
                Some(value) => {
                    metrics::CACHE_HITS.inc();
                    cached_items.push(Item {
                        key: key.to_vec(),
                        value,
                        metadata: None,
                    });
                }
                None => {
                    metrics::CACHE_MISSES.inc();
                    missing_keys.push(*key);
                }
            }
        }

        // Fetch missing keys from backend
        if !missing_keys.is_empty() {
            let backend_items = self.backend.get(namespace, id, missing_keys).await?;

            // Populate cache
            for item in &backend_items {
                let cache_key = format!("{}:{}:{}", namespace, id, hex::encode(&item.key));
                cache.set(&cache_key, &item.value, self.config.ttl).await?;
            }

            cached_items.extend(backend_items);
        }

        Ok(cached_items)
    }

    async fn put(&self, namespace: &str, id: &str, items: Vec<Item>) -> Result<()> {
        // Write to backend first
        self.backend.put(namespace, id, items.clone()).await?;

        // Invalidate cache
        if let Some(cache) = &self.cache {
            for item in &items {
                let cache_key = format!("{}:{}:{}", namespace, id, hex::encode(&item.key));

                match self.config.invalidation {
                    Invalidation::WriteThrough => {
                        // Update cache with new value
                        cache.set(&cache_key, &item.value, self.config.ttl).await?;
                    }
                    Invalidation::TtlOnly => {
                        // Delete from cache, let next read repopulate
                        cache.delete(&cache_key).await?;
                    }
                }
            }
        }

        Ok(())
    }
}
```text

### Cache Key Design

Format: {namespace}:{id}:{key_hex}

Examples:
user-profiles:user123:70726f66696c65  (key="profile")
user-profiles:user123:73657474696e6773 (key="settings")
```

**Why hex encoding**?
- Keys may contain binary data
- Redis keys must be strings
- Hex is safe, readable

### Cache Metrics

```rust
lazy_static! {
    static ref CACHE_HITS: CounterVec = register_counter_vec!(
        "prism_cache_hits_total",
        "Cache hits",
        &["namespace"]
    ).unwrap();

    static ref CACHE_MISSES: CounterVec = register_counter_vec!(
        "prism_cache_misses_total",
        "Cache misses",
        &["namespace"]
    ).unwrap();

    static ref CACHE_HIT_RATE: GaugeVec = register_gauge_vec!(
        "prism_cache_hit_rate",
        "Cache hit rate (0-1)",
        &["namespace"]
    ).unwrap();
}

// Calculate hit rate periodically
fn update_cache_hit_rate(namespace: &str) {
    let hits = CACHE_HITS.with_label_values(&[namespace]).get();
    let misses = CACHE_MISSES.with_label_values(&[namespace]).get();
    let total = hits + misses;

    if total > 0 {
        let hit_rate = hits as f64 / total as f64;
        CACHE_HIT_RATE.with_label_values(&[namespace]).set(hit_rate);
    }
}
```

### Alternatives Considered

1. **No Caching** (backend only)
   - Pros: Simpler
   - Cons: Higher latency, higher backend load
   - Rejected: Caching is essential for read-heavy workloads

2. **Write-Through Cache** (cache is source of truth)
   - Pros: Always consistent
   - Cons: Cache becomes critical dependency, harder to scale
   - Rejected: Increases risk

3. **In-Proxy Memory Cache** (no external cache)
   - Pros: No extra dependency, ultra-fast
   - Cons: Memory pressure on proxy, no sharing between shards
   - Rejected: Doesn't scale

4. **Client-Side Caching**
   - Pros: Zero proxy overhead
   - Cons: Inconsistency, cache invalidation complexity
   - Rejected: Let platform handle it

## Consequences

### Positive

- **Lower Latency**: Cache hits are 10-100x faster than backend
- **Reduced Backend Load**: Fewer queries to database
- **Cost Savings**: Smaller backend instances needed
- **Optional**: Namespaces can opt out if not needed

### Negative

- **Eventual Consistency**: Cache may be stale until TTL expires
  - *Mitigation*: Short TTL for frequently-changing data
- **Extra Dependency**: Redis/memcached must be available
  - *Mitigation*: Degrade gracefully on cache failure
- **Memory Cost**: Cache requires memory
  - *Mitigation*: Right-size cache, use eviction policies

### Neutral

- **Cache Invalidation**: Classic hard problem
  - TTL + write-through handles most cases

## Implementation Notes

### Graceful Degradation

```rust
async fn get_with_cache_fallback(
    &self,
    namespace: &str,
    id: &str,
    keys: Vec<&[u8]>,
) -> Result<Vec<Item>> {
    // Try cache first
    match self.get_from_cache(namespace, id, &keys).await {
        Ok(items) => Ok(items),
        Err(CacheError::Unavailable) => {
            // Cache down, go straight to backend
            metrics::CACHE_UNAVAILABLE.inc();
            self.backend.get(namespace, id, keys).await
        }
        Err(e) => Err(e.into()),
    }
}
```

### Cache Warming

```rust
pub async fn warm_cache(&self, namespace: &str) -> Result<()> {
    // Preload hot data into cache
    let hot_keys = self.get_hot_keys(namespace).await?;

    for key in hot_keys {
        let items = self.backend.get(namespace, &key.id, vec![&key.key]).await?;
        for item in items {
            let cache_key = format!("{}:{}:{}", namespace, key.id, hex::encode(&item.key));
            self.cache.set(&cache_key, &item.value, self.config.ttl).await?;
        }
    }

    Ok(())
}
```

### Cache Backends

Support multiple cache backends:

```rust
pub enum CacheBackendType {
    Redis,
    Memcached,
    InMemory,  // For testing
}

impl CacheBackendType {
    pub fn create(&self, config: &CacheConfig) -> Result<Arc<dyn CacheBackend>> {
        match self {
            Self::Redis => Ok(Arc::new(RedisCache::new(config)?)),
            Self::Memcached => Ok(Arc::new(MemcachedCache::new(config)?)),
            Self::InMemory => Ok(Arc::new(InMemoryCache::new(config)?)),
        }
    }
}
```

## References

- [Netflix KV DAL: Caching](https://netflixtechblog.com/)
- [Redis Best Practices](https://redis.io/docs/manual/patterns/)
- [Memcached Documentation](https://memcached.org/)
- [Cache Aside Pattern](https://learn.microsoft.com/en-us/azure/architecture/patterns/cache-aside)
- ADR-005: Backend Plugin Architecture

## Revision History

- 2025-10-05: Initial draft and acceptance