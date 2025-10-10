//! Prism Proxy - Main entry point

use prism_proxy::{PatternManager, ProxyConfig, ProxyServer, Router};
use std::sync::Arc;
use tracing::{error, info};

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter("prism_proxy=info")
        .init();

    info!("Starting Prism Proxy v{}", env!("CARGO_PKG_VERSION"));

    // Load configuration
    let config = ProxyConfig::default();
    info!("Loaded configuration: {:?}", config);

    // Create pattern manager
    let pattern_manager = Arc::new(PatternManager::new());

    // Register patterns from config
    for pattern_config in &config.patterns {
        info!(
            "Registering pattern: {} at {:?}",
            pattern_config.name, pattern_config.binary_path
        );
        pattern_manager
            .register_pattern(pattern_config.name.clone(), pattern_config.binary_path.clone())
            .await?;
    }

    // Create router
    let router = Arc::new(Router::new(pattern_manager.clone()));

    // Create and start server
    let mut server = ProxyServer::new(router, config.listen_address.clone());

    // Start patterns
    for pattern_config in &config.patterns {
        info!("Starting pattern: {}", pattern_config.name);
        if let Err(e) = pattern_manager.start_pattern(&pattern_config.name).await {
            error!("Failed to start pattern {}: {}", pattern_config.name, e);
        }
    }

    // Start server
    info!("Starting gRPC server on {}", config.listen_address);
    server.start().await?;

    // Wait for shutdown signal
    tokio::signal::ctrl_c().await?;

    // Graceful shutdown
    info!("Received shutdown signal, stopping patterns...");
    for pattern_config in &config.patterns {
        if let Err(e) = pattern_manager.stop_pattern(&pattern_config.name).await {
            error!("Failed to stop pattern {}: {}", pattern_config.name, e);
        }
    }

    server.shutdown().await?;
    info!("Proxy shutdown complete");

    Ok(())
}
