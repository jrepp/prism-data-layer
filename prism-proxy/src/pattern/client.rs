//! gRPC client for pattern lifecycle communication

use crate::proto::interfaces::{
    lifecycle_interface_client::LifecycleInterfaceClient, DrainRequest, HealthCheckRequest,
    InitializeRequest, StartRequest, StopRequest,
};
use tonic::transport::Channel;

/// Convert serde_json::Value to prost_types::Struct
/// TODO: Implement proper JSON to protobuf Struct conversion
/// For now, return an empty struct as this is not critical for POC
fn json_value_to_prost_struct(_value: serde_json::Value) -> crate::Result<prost_types::Struct> {
    // Return empty struct for now (prost uses BTreeMap)
    Ok(prost_types::Struct {
        fields: std::collections::BTreeMap::new(),
    })
}

/// Pattern gRPC client wrapper
pub struct PatternClient {
    client: LifecycleInterfaceClient<Channel>,
}

impl PatternClient {
    /// Connect to a pattern's gRPC endpoint
    pub async fn connect(endpoint: String) -> crate::Result<Self> {
        let client = LifecycleInterfaceClient::connect(endpoint).await?;
        Ok(Self { client })
    }

    /// Initialize the pattern
    /// Returns pattern metadata including interface declarations
    pub async fn initialize(
        &mut self,
        name: String,
        version: String,
        config: serde_json::Value,
    ) -> crate::Result<Option<crate::proto::interfaces::PatternMetadata>> {
        // Convert serde_json::Value to prost_types::Struct
        let config_struct = json_value_to_prost_struct(config)?;

        let request = tonic::Request::new(InitializeRequest {
            name,
            version,
            config: Some(config_struct),
        });

        let response = self.client.initialize(request).await?;
        let init_response = response.into_inner();

        if !init_response.success {
            anyhow::bail!("Initialize failed: {}", init_response.error);
        }

        Ok(init_response.metadata)
    }

    /// Start the pattern
    pub async fn start(&mut self) -> crate::Result<String> {
        let request = tonic::Request::new(StartRequest {});

        let response = self.client.start(request).await?;
        let start_response = response.into_inner();

        if !start_response.success {
            anyhow::bail!("Start failed: {}", start_response.error);
        }

        Ok(start_response.data_endpoint)
    }

    /// Drain the pattern (prepare for shutdown)
    pub async fn drain(&mut self, timeout_seconds: i32, reason: String) -> crate::Result<()> {
        let request = tonic::Request::new(DrainRequest {
            timeout_seconds,
            reason,
        });

        let response = self.client.drain(request).await?;
        let drain_response = response.into_inner();

        if !drain_response.success {
            anyhow::bail!("Drain failed: {}", drain_response.error);
        }

        Ok(())
    }

    /// Stop the pattern
    pub async fn stop(&mut self, timeout_seconds: i32) -> crate::Result<()> {
        let request = tonic::Request::new(StopRequest { timeout_seconds });

        let response = self.client.stop(request).await?;
        let stop_response = response.into_inner();

        if !stop_response.success {
            anyhow::bail!("Stop failed: {}", stop_response.error);
        }

        Ok(())
    }

    /// Health check the pattern
    pub async fn health_check(&mut self) -> crate::Result<crate::pattern::PatternStatus> {
        let request = tonic::Request::new(HealthCheckRequest {});

        let response = self.client.health_check(request).await?;
        let health_response = response.into_inner();

        use crate::proto::interfaces::HealthStatus;
        let status = match HealthStatus::try_from(health_response.status) {
            Ok(HealthStatus::Healthy) => crate::pattern::PatternStatus::Running,
            Ok(HealthStatus::Degraded) => crate::pattern::PatternStatus::Degraded,
            Ok(HealthStatus::Unhealthy) => {
                crate::pattern::PatternStatus::Failed(health_response.message)
            }
            _ => crate::pattern::PatternStatus::Failed("Unknown health status".to_string()),
        };

        Ok(status)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_client_connection_failure() {
        // Test connecting to invalid endpoint
        let result = PatternClient::connect("http://localhost:9999".to_string()).await;
        assert!(
            result.is_err(),
            "Should fail to connect to non-existent endpoint"
        );
    }

    // Note: More comprehensive tests require a mock gRPC server
    // We'll test the full integration in integration tests
}
