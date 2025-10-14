//! Configuration management for Prism proxy

use serde::{Deserialize, Serialize};
use std::path::PathBuf;

/// Proxy configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProxyConfig {
    /// gRPC server listen address
    pub listen_address: String,
    /// Pattern configurations
    pub patterns: Vec<PatternConfig>,
}

/// Pattern configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PatternConfig {
    /// Pattern name
    pub name: String,
    /// Path to pattern binary
    pub binary_path: PathBuf,
    /// Pattern-specific configuration
    #[serde(default)]
    pub config: serde_json::Value,
}

impl Default for ProxyConfig {
    fn default() -> Self {
        Self {
            listen_address: "0.0.0.0:8980".to_string(),
            patterns: Vec::new(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = ProxyConfig::default();
        assert_eq!(config.listen_address, "0.0.0.0:8980");
        assert!(config.patterns.is_empty());
    }
}
