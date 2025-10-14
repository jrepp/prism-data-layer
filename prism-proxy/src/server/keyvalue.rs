//! KeyValue gRPC server implementation

use crate::proto::interfaces::keyvalue::key_value_basic_interface_server::KeyValueBasicInterface;
use crate::proto::interfaces::keyvalue::{
    DeleteRequest, DeleteResponse, ExistsRequest, ExistsResponse, GetRequest, GetResponse,
    SetRequest, SetResponse,
};
use crate::router::Router;
use std::sync::Arc;
use tonic::{Request, Response, Status};

/// KeyValue gRPC service implementation
pub struct KeyValueService {
    _router: Arc<Router>,
}

impl KeyValueService {
    pub fn new(router: Arc<Router>) -> Self {
        Self { _router: router }
    }
}

#[tonic::async_trait]
impl KeyValueBasicInterface for KeyValueService {
    async fn set(&self, request: Request<SetRequest>) -> Result<Response<SetResponse>, Status> {
        let _req = request.into_inner();

        // TODO: Route request to appropriate pattern
        // For now, return success
        Ok(Response::new(SetResponse {
            success: true,
            error: String::new(),
        }))
    }

    async fn get(&self, request: Request<GetRequest>) -> Result<Response<GetResponse>, Status> {
        let _req = request.into_inner();

        // TODO: Route request to appropriate pattern
        // For now, return not found
        Ok(Response::new(GetResponse {
            found: false,
            value: vec![],
            error: String::new(),
        }))
    }

    async fn delete(
        &self,
        request: Request<DeleteRequest>,
    ) -> Result<Response<DeleteResponse>, Status> {
        let _req = request.into_inner();

        // TODO: Route request to appropriate pattern
        Ok(Response::new(DeleteResponse {
            success: true,
            error: String::new(),
        }))
    }

    async fn exists(
        &self,
        request: Request<ExistsRequest>,
    ) -> Result<Response<ExistsResponse>, Status> {
        let _req = request.into_inner();

        // TODO: Route request to appropriate pattern
        Ok(Response::new(ExistsResponse {
            exists: false,
            error: String::new(),
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pattern::PatternManager;

    #[tokio::test]
    async fn test_keyvalue_service_creation() {
        let pattern_manager = Arc::new(PatternManager::new());
        let router = Arc::new(Router::new(pattern_manager));
        let _service = KeyValueService::new(router);
        // Service created successfully
    }

    #[tokio::test]
    async fn test_set_request() {
        let pattern_manager = Arc::new(PatternManager::new());
        let router = Arc::new(Router::new(pattern_manager));
        let service = KeyValueService::new(router);

        let request = Request::new(SetRequest {
            key: "test-key".to_string(),
            value: b"test-value".to_vec(),
            tags: None,
        });

        let response = service.set(request).await;
        assert!(response.is_ok(), "Set request should succeed");

        let set_response = response.unwrap().into_inner();
        assert!(set_response.success, "Set should be successful");
    }

    #[tokio::test]
    async fn test_get_request() {
        let pattern_manager = Arc::new(PatternManager::new());
        let router = Arc::new(Router::new(pattern_manager));
        let service = KeyValueService::new(router);

        let request = Request::new(GetRequest {
            key: "test-key".to_string(),
        });

        let response = service.get(request).await;
        assert!(response.is_ok(), "Get request should succeed");

        let get_response = response.unwrap().into_inner();
        // For now, should return not found
        assert!(!get_response.found, "Key should not be found");
    }

    #[tokio::test]
    async fn test_delete_request() {
        let pattern_manager = Arc::new(PatternManager::new());
        let router = Arc::new(Router::new(pattern_manager));
        let service = KeyValueService::new(router);

        let request = Request::new(DeleteRequest {
            key: "test-key".to_string(),
        });

        let response = service.delete(request).await;
        assert!(response.is_ok(), "Delete request should succeed");

        let delete_response = response.unwrap().into_inner();
        assert!(delete_response.success, "Delete should be successful");
    }
}
