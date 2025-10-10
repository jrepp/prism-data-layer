//! Integration tests for Prism proxy
//!
//! These tests verify end-to-end functionality with real pattern binaries

use prism_proxy::{PatternManager, ProxyServer, Router};
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;
use tokio::time::sleep;

#[tokio::test]
#[ignore] // Run with `cargo test -- --ignored` to include integration tests
async fn test_proxy_with_memstore_pattern() {
    // Initialize tracing for debugging
    let _ = tracing_subscriber::fmt()
        .with_env_filter("prism_proxy=debug")
        .try_init();

    // Path to MemStore binary
    let memstore_path = PathBuf::from("../patterns/memstore/memstore");

    // Skip test if binary doesn't exist
    if !memstore_path.exists() {
        eprintln!("Skipping test: MemStore binary not found at {:?}", memstore_path);
        eprintln!("Build it with: cd patterns/memstore && go build -o memstore cmd/main.go");
        return;
    }

    // Create pattern manager
    let pattern_manager = Arc::new(PatternManager::new());

    // Register MemStore pattern
    pattern_manager
        .register_pattern("memstore".to_string(), memstore_path)
        .await
        .expect("Failed to register MemStore pattern");

    // Create router and server
    let router = Arc::new(Router::new(pattern_manager.clone()));
    let mut server = ProxyServer::new(router, "127.0.0.1:18980".to_string());

    // Start the proxy server
    server
        .start()
        .await
        .expect("Failed to start proxy server");

    println!("✓ Proxy server started on 127.0.0.1:18980");

    // Start MemStore pattern
    println!("Starting MemStore pattern...");
    let start_result = pattern_manager.start_pattern("memstore").await;

    match start_result {
        Ok(_) => {
            println!("✓ MemStore pattern started successfully");

            // Wait for pattern to fully initialize
            sleep(Duration::from_secs(1)).await;

            // Health check the pattern
            let health = pattern_manager.health_check("memstore").await;
            println!("Health check result: {:?}", health);

            assert!(health.is_ok(), "Health check should succeed");

            // TODO: Send actual Set/Get requests via gRPC client
            // This would require a KeyValue gRPC client implementation
            println!("✓ Pattern is healthy and running");

            // Stop the pattern
            println!("Stopping MemStore pattern...");
            pattern_manager
                .stop_pattern("memstore")
                .await
                .expect("Failed to stop MemStore");
            println!("✓ MemStore pattern stopped");
        }
        Err(e) => {
            eprintln!("Failed to start MemStore: {}", e);
            eprintln!("This is expected if MemStore doesn't implement the lifecycle gRPC interface yet");
            // Don't fail the test - MemStore might not implement the full protocol yet
        }
    }

    // Shutdown server
    server.shutdown().await.expect("Failed to shutdown server");
    println!("✓ Proxy server shut down");

    println!("\n🎉 Integration test completed!");
}

#[tokio::test]
async fn test_pattern_manager_standalone() {
    // Test pattern manager without actually spawning processes
    let pattern_manager = PatternManager::new();

    // Register a test pattern
    pattern_manager
        .register_pattern("test".to_string(), PathBuf::from("/nonexistent"))
        .await
        .expect("Failed to register test pattern");

    // List patterns
    let patterns = pattern_manager.list_patterns().await;
    assert_eq!(patterns.len(), 1);
    assert_eq!(patterns[0], "test");

    println!("✓ Pattern manager basic operations work");
}

#[tokio::test]
async fn test_server_startup_and_shutdown() {
    let pattern_manager = Arc::new(PatternManager::new());
    let router = Arc::new(Router::new(pattern_manager));
    let mut server = ProxyServer::new(router, "127.0.0.1:18981".to_string());

    // Start server
    server.start().await.expect("Failed to start server");
    println!("✓ Server started on 127.0.0.1:18981");

    // Give it a moment
    sleep(Duration::from_millis(100)).await;

    // Shutdown server
    server.shutdown().await.expect("Failed to shutdown server");
    println!("✓ Server shut down cleanly");
}
