//! Pattern management for Prism proxy
//!
//! This module handles the lifecycle of backend patterns, including:
//! - Pattern discovery and loading
//! - Lifecycle management (Initialize, Start, HealthCheck, Shutdown)
//! - gRPC communication with patterns
//! - Health monitoring

mod client;

use client::PatternClient;
use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;
use tokio::process::Child;
use tokio::sync::RwLock;
use tokio::time::sleep;

/// Pattern status enumeration
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PatternStatus {
    /// Pattern is not yet started
    Uninitialized,
    /// Pattern is starting up
    Starting,
    /// Pattern is running and healthy
    Running,
    /// Pattern is unhealthy but still running
    Degraded,
    /// Pattern is shutting down
    Stopping,
    /// Pattern has stopped
    Stopped,
    /// Pattern has failed
    Failed(String),
}

/// Pattern metadata and handle
pub struct Pattern {
    /// Pattern name
    pub name: String,
    /// Pattern version
    pub version: String,
    /// Pattern binary path
    pub binary_path: PathBuf,
    /// Current status
    pub status: PatternStatus,
    /// Pattern process handle (if running)
    process: Option<Child>,
    /// gRPC endpoint (if running)
    pub grpc_endpoint: Option<String>,
    /// gRPC client (if connected)
    client: Option<PatternClient>,
    /// Pattern configuration
    config: serde_json::Value,
}

impl Pattern {
    /// Create a new pattern instance
    pub fn new(name: String, binary_path: PathBuf) -> Self {
        Self {
            name,
            version: String::new(),
            binary_path,
            status: PatternStatus::Uninitialized,
            process: None,
            grpc_endpoint: None,
            client: None,
            config: serde_json::json!({}),
        }
    }

    /// Set pattern configuration
    pub fn with_config(mut self, config: serde_json::Value) -> Self {
        self.config = config;
        self
    }

    /// Get pattern status
    pub fn status(&self) -> &PatternStatus {
        &self.status
    }

    /// Check if pattern is running
    pub fn is_running(&self) -> bool {
        matches!(
            self.status,
            PatternStatus::Running | PatternStatus::Degraded
        )
    }

    /// Spawn the pattern process
    async fn spawn(&mut self, port: u16) -> crate::Result<()> {
        use tokio::process::Command;

        tracing::info!(
            pattern = %self.name,
            binary = %self.binary_path.display(),
            port = port,
            "spawning pattern process"
        );

        // Build command with gRPC port argument
        let mut cmd = Command::new(&self.binary_path);
        cmd.arg("--grpc-port").arg(port.to_string());

        // Spawn the process
        let child = cmd.spawn().map_err(|e| {
            tracing::error!(
                pattern = %self.name,
                binary = %self.binary_path.display(),
                error = %e,
                "failed to spawn pattern process"
            );
            e
        })?;

        let pid = child.id();
        self.process = Some(child);

        // Set gRPC endpoint
        let endpoint = format!("http://localhost:{}", port);
        self.grpc_endpoint = Some(endpoint.clone());

        tracing::info!(
            pattern = %self.name,
            pid = ?pid,
            endpoint = %endpoint,
            "pattern process spawned successfully"
        );

        // Wait for process to start
        tracing::info!(
            pattern = %self.name,
            "waiting for pattern process to initialize"
        );
        sleep(Duration::from_millis(500)).await;

        Ok(())
    }

    /// Connect gRPC client to pattern with retry and exponential backoff
    async fn connect_client(&mut self) -> crate::Result<()> {
        if let Some(ref endpoint) = self.grpc_endpoint {
            tracing::info!(
                pattern = %self.name,
                endpoint = %endpoint,
                "connecting gRPC client to pattern with retry"
            );

            // Retry configuration: 5 attempts with exponential backoff
            let max_attempts = 5;
            let initial_delay = Duration::from_millis(100);
            let max_delay = Duration::from_secs(2);

            let mut attempt = 1;
            let mut delay = initial_delay;

            loop {
                tracing::debug!(
                    pattern = %self.name,
                    attempt = attempt,
                    max_attempts = max_attempts,
                    "attempting gRPC connection"
                );

                match PatternClient::connect(endpoint.clone()).await {
                    Ok(client) => {
                        self.client = Some(client);

                        tracing::info!(
                            pattern = %self.name,
                            endpoint = %endpoint,
                            attempts = attempt,
                            "gRPC client connected successfully"
                        );

                        return Ok(());
                    }
                    Err(e) => {
                        if attempt >= max_attempts {
                            tracing::error!(
                                pattern = %self.name,
                                endpoint = %endpoint,
                                attempts = attempt,
                                error = %e,
                                "failed to connect gRPC client after max retries"
                            );
                            return Err(e);
                        }

                        tracing::warn!(
                            pattern = %self.name,
                            attempt = attempt,
                            next_delay_ms = delay.as_millis(),
                            error = %e,
                            "gRPC connection attempt failed, retrying"
                        );

                        sleep(delay).await;

                        // Exponential backoff: double the delay, cap at max_delay
                        delay = (delay * 2).min(max_delay);
                        attempt += 1;
                    }
                }
            }
        } else {
            tracing::error!(
                pattern = %self.name,
                "no gRPC endpoint available for connection"
            );
            anyhow::bail!("No gRPC endpoint available");
        }
    }

    /// Initialize the pattern via gRPC
    async fn initialize_pattern(&mut self) -> crate::Result<()> {
        if let Some(ref mut client) = self.client {
            tracing::info!(
                pattern = %self.name,
                version = %self.version,
                "initializing pattern via gRPC"
            );

            client
                .initialize(
                    self.name.clone(),
                    self.version.clone(),
                    self.config.clone(),
                )
                .await
                .map_err(|e| {
                    tracing::error!(
                        pattern = %self.name,
                        error = %e,
                        "failed to initialize pattern"
                    );
                    e
                })?;

            tracing::info!(
                pattern = %self.name,
                "pattern initialized successfully"
            );

            Ok(())
        } else {
            tracing::error!(
                pattern = %self.name,
                "no gRPC client available for initialization"
            );
            anyhow::bail!("No gRPC client available");
        }
    }

    /// Start the pattern via gRPC
    async fn start_pattern(&mut self) -> crate::Result<()> {
        if let Some(ref mut client) = self.client {
            tracing::info!(
                pattern = %self.name,
                "starting pattern via gRPC"
            );

            client.start().await.map_err(|e| {
                tracing::error!(
                    pattern = %self.name,
                    error = %e,
                    "failed to start pattern"
                );
                e
            })?;

            tracing::info!(
                pattern = %self.name,
                "pattern started successfully"
            );

            Ok(())
        } else {
            tracing::error!(
                pattern = %self.name,
                "no gRPC client available for start"
            );
            anyhow::bail!("No gRPC client available");
        }
    }

    /// Stop the pattern via gRPC
    async fn stop_pattern(&mut self) -> crate::Result<()> {
        tracing::info!(
            pattern = %self.name,
            "stopping pattern via gRPC"
        );

        if let Some(ref mut client) = self.client {
            if let Err(e) = client.stop(30).await {
                tracing::warn!(
                    pattern = %self.name,
                    error = %e,
                    "gRPC stop call failed, will kill process"
                );
            } else {
                tracing::info!(
                    pattern = %self.name,
                    "pattern stopped gracefully via gRPC"
                );
            }
        }

        // Kill the process if still running
        if let Some(mut process) = self.process.take() {
            tracing::info!(
                pattern = %self.name,
                pid = ?process.id(),
                "killing pattern process"
            );

            let _ = process.kill().await;
            let _ = process.wait().await;

            tracing::info!(
                pattern = %self.name,
                "pattern process terminated"
            );
        }

        Ok(())
    }

    /// Health check via gRPC
    async fn health_check_pattern(&mut self) -> crate::Result<PatternStatus> {
        if let Some(ref mut client) = self.client {
            client.health_check().await
        } else {
            Ok(PatternStatus::Uninitialized)
        }
    }
}

/// Pattern manager - coordinates pattern lifecycle
pub struct PatternManager {
    /// Registered patterns
    patterns: Arc<RwLock<HashMap<String, Pattern>>>,
}

impl PatternManager {
    /// Create a new pattern manager
    pub fn new() -> Self {
        Self {
            patterns: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Register a pattern
    pub async fn register_pattern(&self, name: String, binary_path: PathBuf) -> crate::Result<()> {
        tracing::info!(
            pattern = %name,
            binary = %binary_path.display(),
            "registering pattern"
        );

        let pattern = Pattern::new(name.clone(), binary_path.clone());
        let mut patterns = self.patterns.write().await;
        patterns.insert(name.clone(), pattern);

        tracing::info!(
            pattern = %name,
            "pattern registered successfully"
        );

        Ok(())
    }

    /// Get pattern by name (returns metadata only, not handles)
    pub async fn get_pattern(&self, name: &str) -> Option<(String, PatternStatus, Option<String>)> {
        let patterns = self.patterns.read().await;
        patterns
            .get(name)
            .map(|p| (p.name.clone(), p.status.clone(), p.grpc_endpoint.clone()))
    }

    /// List all registered patterns
    pub async fn list_patterns(&self) -> Vec<String> {
        let patterns = self.patterns.read().await;
        patterns.keys().cloned().collect()
    }

    /// Start a pattern
    pub async fn start_pattern(&self, name: &str) -> crate::Result<()> {
        tracing::info!(pattern = %name, "starting pattern lifecycle");

        let mut patterns = self.patterns.write().await;
        if let Some(pattern) = patterns.get_mut(name) {
            pattern.status = PatternStatus::Starting;
            tracing::info!(pattern = %name, "pattern status: Starting");

            // Allocate a port (for now, use a simple scheme: 9000 + hash)
            let port = 9000 + (name.chars().map(|c| c as u16).sum::<u16>() % 1000);
            tracing::info!(pattern = %name, port = port, "allocated gRPC port");

            // Spawn the process
            tracing::info!(pattern = %name, "step 1/4: spawning pattern process");
            if let Err(e) = pattern.spawn(port).await {
                pattern.status = PatternStatus::Failed(format!("Spawn failed: {}", e));
                tracing::error!(pattern = %name, error = %e, "failed to spawn pattern");
                anyhow::bail!("Failed to spawn pattern: {}", e);
            }

            // Connect gRPC client
            tracing::info!(pattern = %name, "step 2/4: connecting gRPC client");
            if let Err(e) = pattern.connect_client().await {
                pattern.status = PatternStatus::Failed(format!("gRPC connect failed: {}", e));
                tracing::error!(pattern = %name, error = %e, "failed to connect gRPC client");
                anyhow::bail!("Failed to connect gRPC client: {}", e);
            }

            // Initialize pattern
            tracing::info!(pattern = %name, "step 3/4: initializing pattern");
            if let Err(e) = pattern.initialize_pattern().await {
                pattern.status = PatternStatus::Failed(format!("Initialize failed: {}", e));
                tracing::error!(pattern = %name, error = %e, "failed to initialize pattern");
                anyhow::bail!("Failed to initialize pattern: {}", e);
            }

            // Start pattern
            tracing::info!(pattern = %name, "step 4/4: starting pattern");
            if let Err(e) = pattern.start_pattern().await {
                pattern.status = PatternStatus::Failed(format!("Start failed: {}", e));
                tracing::error!(pattern = %name, error = %e, "failed to start pattern");
                anyhow::bail!("Failed to start pattern: {}", e);
            }

            pattern.status = PatternStatus::Running;
            tracing::info!(
                pattern = %name,
                endpoint = ?pattern.grpc_endpoint,
                "pattern lifecycle complete - Running"
            );

            Ok(())
        } else {
            tracing::error!(pattern = %name, "pattern not found in registry");
            anyhow::bail!("Pattern not found: {}", name)
        }
    }

    /// Stop a pattern
    pub async fn stop_pattern(&self, name: &str) -> crate::Result<()> {
        tracing::info!(pattern = %name, "stopping pattern");

        let mut patterns = self.patterns.write().await;
        if let Some(pattern) = patterns.get_mut(name) {
            pattern.status = PatternStatus::Stopping;
            tracing::info!(pattern = %name, "pattern status: Stopping");

            // Send shutdown via gRPC and kill process
            if let Err(e) = pattern.stop_pattern().await {
                tracing::warn!(pattern = %name, error = %e, "error stopping pattern");
            }

            pattern.status = PatternStatus::Stopped;
            tracing::info!(pattern = %name, "pattern stopped successfully");

            Ok(())
        } else {
            tracing::error!(pattern = %name, "pattern not found in registry");
            anyhow::bail!("Pattern not found: {}", name)
        }
    }

    /// Health check a pattern
    pub async fn health_check(&self, name: &str) -> crate::Result<PatternStatus> {
        tracing::debug!(pattern = %name, "performing health check");

        let mut patterns = self.patterns.write().await;
        if let Some(pattern) = patterns.get_mut(name) {
            // If pattern is running, do a gRPC health check
            if pattern.is_running() {
                match pattern.health_check_pattern().await {
                    Ok(status) => {
                        tracing::info!(
                            pattern = %name,
                            status = ?status,
                            "health check successful"
                        );
                        pattern.status = status.clone();
                        Ok(status)
                    }
                    Err(e) => {
                        tracing::warn!(
                            pattern = %name,
                            error = %e,
                            current_status = ?pattern.status,
                            "health check failed, returning current status"
                        );
                        Ok(pattern.status.clone())
                    }
                }
            } else {
                tracing::debug!(
                    pattern = %name,
                    status = ?pattern.status,
                    "pattern not running, returning current status"
                );
                Ok(pattern.status.clone())
            }
        } else {
            tracing::error!(pattern = %name, "pattern not found in registry");
            anyhow::bail!("Pattern not found: {}", name)
        }
    }
}

impl Default for PatternManager {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    #[tokio::test]
    async fn test_pattern_manager_creation() {
        let manager = PatternManager::new();
        let patterns = manager.list_patterns().await;
        assert!(patterns.is_empty(), "New manager should have no patterns");
    }

    #[tokio::test]
    async fn test_register_pattern() {
        let manager = PatternManager::new();
        let path = PathBuf::from("/path/to/memstore");

        manager
            .register_pattern("memstore".to_string(), path.clone())
            .await
            .expect("Failed to register pattern");

        let patterns = manager.list_patterns().await;
        assert_eq!(patterns.len(), 1, "Should have one pattern registered");
        assert_eq!(patterns[0], "memstore");
    }

    #[tokio::test]
    async fn test_get_pattern() {
        let manager = PatternManager::new();
        let path = PathBuf::from("/path/to/memstore");

        manager
            .register_pattern("memstore".to_string(), path.clone())
            .await
            .unwrap();

        let result = manager.get_pattern("memstore").await;
        assert!(result.is_some(), "Should find registered pattern");

        let (name, status, endpoint) = result.unwrap();
        assert_eq!(name, "memstore");
        assert_eq!(status, PatternStatus::Uninitialized);
        assert_eq!(endpoint, None);
    }

    #[tokio::test]
    async fn test_pattern_lifecycle_without_real_binary() {
        // This test checks the lifecycle without actually spawning a process
        // Full integration tests with real binaries are in tests/integration/
        let manager = PatternManager::new();
        let path = PathBuf::from("/nonexistent/path");

        // Register pattern
        manager
            .register_pattern("test-pattern".to_string(), path)
            .await
            .unwrap();

        // Check initial status
        let status = manager.health_check("test-pattern").await.unwrap();
        assert_eq!(status, PatternStatus::Uninitialized);

        // Try to start pattern (will fail since binary doesn't exist)
        let result = manager.start_pattern("test-pattern").await;
        assert!(
            result.is_err(),
            "Should fail to start non-existent binary"
        );

        // Check that status changed to Failed
        let status = manager.health_check("test-pattern").await.unwrap();
        assert!(
            matches!(status, PatternStatus::Failed(_)),
            "Status should be Failed after spawn failure"
        );
    }

    #[tokio::test]
    async fn test_pattern_not_found() {
        let manager = PatternManager::new();

        let result = manager.start_pattern("nonexistent").await;
        assert!(result.is_err(), "Should fail to start nonexistent pattern");

        let result = manager.health_check("nonexistent").await;
        assert!(result.is_err(), "Should fail health check for nonexistent pattern");
    }

    #[tokio::test]
    async fn test_pattern_status_transitions() {
        let pattern = Pattern::new("test".to_string(), PathBuf::from("/test"));

        assert_eq!(pattern.status(), &PatternStatus::Uninitialized);
        assert!(!pattern.is_running());
    }

    #[tokio::test]
    async fn test_pattern_spawn_with_invalid_binary() {
        let mut pattern = Pattern::new("test".to_string(), PathBuf::from("/nonexistent"));

        // Try to spawn non-existent binary
        let result = pattern.spawn(9999).await;
        assert!(result.is_err(), "Should fail to spawn non-existent binary");
    }

    #[tokio::test]
    async fn test_pattern_with_config() {
        let config = serde_json::json!({
            "max_keys": 1000,
            "cleanup_period": "60s"
        });

        let pattern = Pattern::new("test".to_string(), PathBuf::from("/test"))
            .with_config(config.clone());

        assert_eq!(pattern.config, config);
    }
}
