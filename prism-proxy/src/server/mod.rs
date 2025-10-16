//! gRPC server for Prism proxy

mod keyvalue;

use crate::proto::interfaces::keyvalue::key_value_basic_interface_server::KeyValueBasicInterfaceServer;
use crate::router::Router;
use keyvalue::KeyValueService;
use std::net::SocketAddr;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::oneshot;
use tokio::sync::RwLock;
use tokio::time::sleep;
use tonic::transport::Server;

/// Server drain state
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DrainState {
    /// Server is running normally
    Running,
    /// Server is draining connections (rejecting new connections, completing existing work)
    Draining { started_at: Instant },
    /// Server is stopping (all patterns stopped)
    Stopping,
}

/// Proxy server
pub struct ProxyServer {
    router: Arc<Router>,
    listen_address: String,
    shutdown_tx: Option<oneshot::Sender<()>>,
    /// Drain state tracking
    drain_state: Arc<RwLock<DrainState>>,
    /// Active frontend connection count
    active_connections: Arc<AtomicUsize>,
}

impl ProxyServer {
    /// Create a new proxy server
    pub fn new(router: Arc<Router>, listen_address: String) -> Self {
        Self {
            router,
            listen_address,
            shutdown_tx: None,
            drain_state: Arc::new(RwLock::new(DrainState::Running)),
            active_connections: Arc::new(AtomicUsize::new(0)),
        }
    }

    /// Get current drain state
    pub async fn get_drain_state(&self) -> DrainState {
        self.drain_state.read().await.clone()
    }

    /// Get active connection count
    pub fn get_active_connections(&self) -> usize {
        self.active_connections.load(Ordering::Relaxed)
    }

    /// Start the server
    pub async fn start(&mut self) -> crate::Result<()> {
        let addr: SocketAddr = self.listen_address.parse()?;
        tracing::info!("Starting proxy server on {}", addr);

        // Create KeyValue service
        let keyvalue_service = KeyValueService::new(self.router.clone());

        // Create shutdown channel
        let (shutdown_tx, shutdown_rx) = oneshot::channel::<()>();
        self.shutdown_tx = Some(shutdown_tx);

        // Start gRPC server
        tokio::spawn(async move {
            Server::builder()
                .add_service(KeyValueBasicInterfaceServer::new(keyvalue_service))
                .serve_with_shutdown(addr, async {
                    shutdown_rx.await.ok();
                })
                .await
                .expect("gRPC server failed");
        });

        // Give server time to start
        tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;

        Ok(())
    }

    /// Shutdown the server
    pub async fn shutdown(&mut self) -> crate::Result<()> {
        tracing::info!("Shutting down proxy server");

        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }

        // Give server time to shutdown
        tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;

        Ok(())
    }

    /// Drain and shutdown the server gracefully
    ///
    /// This implements the drain-on-shutdown behavior:
    /// 1. Enter drain mode - reject new connections, complete existing work
    /// 2. Signal pattern runners to drain
    /// 3. Wait for frontend connections to complete (with timeout)
    /// 4. Stop pattern runners
    /// 5. Shutdown gRPC server
    pub async fn drain_and_shutdown(
        &mut self,
        timeout: Duration,
        reason: String,
    ) -> crate::Result<()> {
        tracing::info!(
            timeout_secs = timeout.as_secs(),
            reason = %reason,
            "🔸 Starting drain-on-shutdown sequence"
        );

        // Phase 1: Enter drain mode
        {
            let mut state = self.drain_state.write().await;
            *state = DrainState::Draining {
                started_at: Instant::now(),
            };
        }
        tracing::info!("🔸 DRAIN MODE: Rejecting new connections, completing existing work");

        // Phase 2: Signal pattern runners to drain
        tracing::info!("🔸 Signaling pattern runners to drain");
        if let Err(e) = self
            .router
            .pattern_manager
            .drain_all_patterns(timeout.as_secs() as i32, reason.clone())
            .await
        {
            tracing::warn!(error = %e, "Failed to drain pattern runners, continuing shutdown");
        }

        // Phase 3: Wait for frontend connections to complete
        tracing::info!(
            active = self.active_connections.load(Ordering::Relaxed),
            "⏳ Waiting for frontend connections to drain"
        );

        let poll_interval = Duration::from_millis(100);
        let deadline = Instant::now() + timeout;

        while self.active_connections.load(Ordering::Relaxed) > 0 {
            if Instant::now() > deadline {
                let remaining = self.active_connections.load(Ordering::Relaxed);
                tracing::warn!(
                    remaining_connections = remaining,
                    "⏱️  Drain timeout exceeded, forcing shutdown"
                );
                break;
            }
            sleep(poll_interval).await;
        }

        tracing::info!("✅ Frontend connections drained");

        // Phase 4: Stop pattern runners
        {
            let mut state = self.drain_state.write().await;
            *state = DrainState::Stopping;
        }
        tracing::info!("🔹 STOPPING MODE: Stopping pattern runners");

        if let Err(e) = self
            .router
            .pattern_manager
            .stop_all_patterns()
            .await
        {
            tracing::warn!(error = %e, "Failed to stop pattern runners, continuing shutdown");
        }

        // Phase 5: Shutdown gRPC server
        if let Some(shutdown_tx) = self.shutdown_tx.take() {
            let _ = shutdown_tx.send(());
        }

        // Give server time to shutdown
        sleep(Duration::from_millis(100)).await;

        tracing::info!("✅ Proxy shutdown complete");
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pattern::PatternManager;

    #[tokio::test]
    async fn test_server_creation() {
        let pattern_manager = Arc::new(PatternManager::new());
        let router = Arc::new(Router::new(pattern_manager));
        let _server = ProxyServer::new(router, "0.0.0.0:8980".to_string());
        // Server created successfully
    }

    #[tokio::test]
    async fn test_server_start_and_shutdown() {
        let pattern_manager = Arc::new(PatternManager::new());
        let router = Arc::new(Router::new(pattern_manager));
        let mut server = ProxyServer::new(router, "127.0.0.1:19980".to_string());

        // Start server
        server.start().await.expect("Failed to start server");

        // Shutdown server
        server.shutdown().await.expect("Failed to shutdown server");
    }
}
