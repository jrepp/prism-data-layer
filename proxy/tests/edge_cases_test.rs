//! Edge case and failure scenario tests for Prism proxy
//!
//! These tests validate proxy behavior under adverse conditions:
//! - Process crashes
//! - Connection failures and timeouts
//! - Concurrent operations
//! - Resource exhaustion
//! - Invalid inputs

use prism_proxy::{PatternManager, PatternStatus};
use std::path::PathBuf;
use std::sync::Arc;
use std::time::Duration;
use tokio::time::{sleep, timeout};

// Helper to create a non-existent binary path
fn nonexistent_binary() -> PathBuf {
    PathBuf::from("/tmp/nonexistent-pattern-binary-12345")
}

#[tokio::test]
async fn test_pattern_spawn_failure_updates_status() {
    let manager = PatternManager::new();

    // Register pattern with invalid binary
    manager
        .register_pattern("failing-pattern".to_string(), nonexistent_binary())
        .await
        .expect("Should register pattern");

    // Try to start - should fail
    let result = manager.start_pattern("failing-pattern").await;
    assert!(result.is_err(), "Should fail to start invalid binary");

    // Verify status is Failed
    let status = manager.health_check("failing-pattern").await.unwrap();
    assert!(
        matches!(status, PatternStatus::Failed(_)),
        "Status should be Failed, got: {:?}",
        status
    );
}

#[tokio::test]
async fn test_health_check_on_uninitialized_pattern() {
    let manager = PatternManager::new();

    manager
        .register_pattern("uninitialized".to_string(), nonexistent_binary())
        .await
        .unwrap();

    // Health check should succeed but return Uninitialized
    let status = manager.health_check("uninitialized").await.unwrap();
    assert_eq!(status, PatternStatus::Uninitialized);
}

#[tokio::test]
async fn test_concurrent_pattern_registration() {
    let manager = Arc::new(PatternManager::new());

    let mut handles = vec![];

    // Register 10 patterns concurrently
    for i in 0..10 {
        let manager_clone = manager.clone();
        let handle = tokio::spawn(async move {
            let name = format!("pattern-{}", i);
            let path = PathBuf::from(format!("/tmp/pattern-{}", i));
            manager_clone.register_pattern(name, path).await
        });
        handles.push(handle);
    }

    // All should succeed
    for handle in handles {
        let result = handle.await.unwrap();
        assert!(result.is_ok(), "Concurrent registration should succeed");
    }

    // Verify all registered
    let patterns = manager.list_patterns().await;
    assert_eq!(patterns.len(), 10, "Should have 10 patterns registered");
}

#[tokio::test]
async fn test_concurrent_health_checks() {
    let manager = Arc::new(PatternManager::new());

    manager
        .register_pattern("test".to_string(), nonexistent_binary())
        .await
        .unwrap();

    let mut handles = vec![];

    // Run 20 concurrent health checks
    for _ in 0..20 {
        let manager_clone = manager.clone();
        let handle = tokio::spawn(async move {
            manager_clone.health_check("test").await
        });
        handles.push(handle);
    }

    // All should succeed
    for handle in handles {
        let result = handle.await.unwrap();
        assert!(result.is_ok(), "Concurrent health checks should succeed");
    }
}

#[tokio::test]
async fn test_stop_pattern_that_never_started() {
    let manager = PatternManager::new();

    manager
        .register_pattern("never-started".to_string(), nonexistent_binary())
        .await
        .unwrap();

    // Stop should succeed even though pattern never started
    let result = manager.stop_pattern("never-started").await;
    assert!(result.is_ok(), "Should gracefully handle stopping unstarted pattern");

    let status = manager.health_check("never-started").await.unwrap();
    assert_eq!(status, PatternStatus::Stopped);
}

#[tokio::test]
async fn test_multiple_start_attempts() {
    let manager = PatternManager::new();

    manager
        .register_pattern("multi-start".to_string(), nonexistent_binary())
        .await
        .unwrap();

    // First start attempt
    let result1 = manager.start_pattern("multi-start").await;
    assert!(result1.is_err(), "First start should fail (invalid binary)");

    // Second start attempt - should also fail gracefully
    let result2 = manager.start_pattern("multi-start").await;
    assert!(result2.is_err(), "Second start should also fail");

    // Status should still be Failed
    let status = manager.health_check("multi-start").await.unwrap();
    assert!(matches!(status, PatternStatus::Failed(_)));
}

#[tokio::test]
async fn test_pattern_not_found_operations() {
    let manager = PatternManager::new();

    // Try operations on non-existent pattern
    let start_result = manager.start_pattern("ghost").await;
    assert!(start_result.is_err(), "Start should fail for non-existent pattern");

    let stop_result = manager.stop_pattern("ghost").await;
    assert!(stop_result.is_err(), "Stop should fail for non-existent pattern");

    let health_result = manager.health_check("ghost").await;
    assert!(health_result.is_err(), "Health check should fail for non-existent pattern");
}

#[tokio::test]
async fn test_empty_pattern_name() {
    let manager = PatternManager::new();

    // Register pattern with empty name
    manager
        .register_pattern("".to_string(), nonexistent_binary())
        .await
        .expect("Should allow empty name registration");

    // Should be able to query it
    let result = manager.get_pattern("").await;
    assert!(result.is_some(), "Should find pattern with empty name");
}

#[tokio::test]
async fn test_duplicate_pattern_registration() {
    let manager = PatternManager::new();
    let path = nonexistent_binary();

    // Register same pattern twice
    manager
        .register_pattern("duplicate".to_string(), path.clone())
        .await
        .unwrap();

    // Second registration should succeed (overwrites)
    manager
        .register_pattern("duplicate".to_string(), path)
        .await
        .unwrap();

    let patterns = manager.list_patterns().await;
    assert_eq!(patterns.len(), 1, "Should have only one pattern (overwritten)");
}

#[tokio::test]
async fn test_very_long_pattern_name() {
    let manager = PatternManager::new();

    // Create a very long name (1000 characters)
    let long_name = "a".repeat(1000);

    manager
        .register_pattern(long_name.clone(), nonexistent_binary())
        .await
        .expect("Should handle long pattern names");

    let result = manager.get_pattern(&long_name).await;
    assert!(result.is_some(), "Should find pattern with long name");
}

#[tokio::test]
async fn test_special_characters_in_pattern_name() {
    let manager = PatternManager::new();

    // Test various special characters
    let names = vec![
        "pattern-with-dashes",
        "pattern_with_underscores",
        "pattern.with.dots",
        "pattern:with:colons",
        "pattern/with/slashes", // This might be problematic but should not crash
        "pattern with spaces",
        "pattern\nwith\nnewlines",
        "pattern\twith\ttabs",
    ];

    for name in names {
        let result = manager
            .register_pattern(name.to_string(), nonexistent_binary())
            .await;
        assert!(
            result.is_ok(),
            "Should handle special characters in name: {}",
            name
        );
    }

    let patterns = manager.list_patterns().await;
    assert_eq!(patterns.len(), 8, "Should register all patterns");
}

#[tokio::test]
async fn test_concurrent_start_attempts_on_same_pattern() {
    let manager = Arc::new(PatternManager::new());

    manager
        .register_pattern("concurrent-start".to_string(), nonexistent_binary())
        .await
        .unwrap();

    let mut handles = vec![];

    // Try to start the same pattern concurrently (all should fail since binary doesn't exist)
    for _ in 0..5 {
        let manager_clone = manager.clone();
        let handle = tokio::spawn(async move {
            manager_clone.start_pattern("concurrent-start").await
        });
        handles.push(handle);
    }

    // All should complete (though they'll all fail)
    for handle in handles {
        let result = handle.await.unwrap();
        // Each attempt should fail with spawn error
        assert!(result.is_err());
    }
}

#[tokio::test]
async fn test_pattern_list_is_consistent() {
    let manager = PatternManager::new();

    // Register patterns
    for i in 0..5 {
        manager
            .register_pattern(format!("pattern-{}", i), nonexistent_binary())
            .await
            .unwrap();
    }

    // Get list multiple times - should be consistent
    let list1 = manager.list_patterns().await;
    let list2 = manager.list_patterns().await;
    let list3 = manager.list_patterns().await;

    assert_eq!(list1, list2);
    assert_eq!(list2, list3);
    assert_eq!(list1.len(), 5);
}

#[tokio::test]
async fn test_health_check_timeout_handling() {
    let manager = PatternManager::new();

    manager
        .register_pattern("timeout-test".to_string(), nonexistent_binary())
        .await
        .unwrap();

    // Health check with short timeout
    let result = timeout(
        Duration::from_millis(100),
        manager.health_check("timeout-test")
    ).await;

    assert!(
        result.is_ok(),
        "Health check should complete within timeout"
    );
}

#[tokio::test]
async fn test_get_pattern_returns_correct_metadata() {
    let manager = PatternManager::new();
    let path = nonexistent_binary();

    manager
        .register_pattern("metadata-test".to_string(), path.clone())
        .await
        .unwrap();

    let result = manager.get_pattern("metadata-test").await;
    assert!(result.is_some());

    let (name, status, endpoint) = result.unwrap();
    assert_eq!(name, "metadata-test");
    assert_eq!(status, PatternStatus::Uninitialized);
    assert_eq!(endpoint, None);
}

#[tokio::test]
async fn test_pattern_manager_is_send_and_sync() {
    // This test ensures PatternManager can be safely shared across threads
    fn assert_send<T: Send>() {}
    fn assert_sync<T: Sync>() {}

    assert_send::<PatternManager>();
    assert_sync::<PatternManager>();
}

// NOTE: The following tests require actual pattern binaries and are marked as ignored
// Run with: cargo test -- --ignored

#[tokio::test]
#[ignore]
async fn test_pattern_crash_detection() {
    // This would require a pattern binary that crashes after starting
    // TODO: Implement with a test binary that exits with error code
    todo!("Requires test pattern binary that crashes")
}

#[tokio::test]
#[ignore]
async fn test_pattern_graceful_restart() {
    // This would test stopping and restarting a running pattern
    // TODO: Implement with real pattern binary
    todo!("Requires real pattern binary")
}

#[tokio::test]
#[ignore]
async fn test_port_conflict_handling() {
    // This would test what happens when allocated port is already in use
    // TODO: Implement by manually binding the port first
    todo!("Requires port binding setup")
}

#[tokio::test]
#[ignore]
async fn test_slow_pattern_startup() {
    // Test pattern that takes >5 seconds to start
    // TODO: Implement with test binary that delays startup
    todo!("Requires slow-starting test binary")
}

#[tokio::test]
#[ignore]
async fn test_pattern_memory_leak_detection() {
    // Test for memory leaks in pattern lifecycle
    // TODO: Implement with memory monitoring
    todo!("Requires memory monitoring")
}
