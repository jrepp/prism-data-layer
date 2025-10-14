//! Prism Proxy - High-performance data access gateway
//!
//! This library provides the core functionality for the Prism proxy:
//! - Pattern lifecycle management
//! - gRPC server for client requests
//! - Request routing to backend patterns
//! - Configuration management

pub mod config;
pub mod pattern;
pub mod proto;
pub mod router;
pub mod server;

// Re-export commonly used types
pub use config::ProxyConfig;
pub use pattern::{Pattern, PatternManager, PatternStatus};
pub use router::Router;
pub use server::ProxyServer;

/// Result type used throughout the proxy
pub type Result<T> = anyhow::Result<T>;

#[cfg(test)]
mod tests {
    // Library compilation is verified by running any test
}
