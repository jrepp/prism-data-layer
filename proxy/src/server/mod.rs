//! gRPC server for Prism proxy

mod keyvalue;

use crate::proto::pattern::key_value_server::KeyValueServer;
use crate::router::Router;
use keyvalue::KeyValueService;
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::sync::oneshot;
use tonic::transport::Server;

/// Proxy server
pub struct ProxyServer {
    router: Arc<Router>,
    listen_address: String,
    shutdown_tx: Option<oneshot::Sender<()>>,
}

impl ProxyServer {
    /// Create a new proxy server
    pub fn new(router: Arc<Router>, listen_address: String) -> Self {
        Self {
            router,
            listen_address,
            shutdown_tx: None,
        }
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
                .add_service(KeyValueServer::new(keyvalue_service))
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
