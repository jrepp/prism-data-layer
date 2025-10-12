//! Generated protobuf code for Prism proxy

// Module structure matches protobuf package structure
pub mod common {
    include!(concat!(env!("OUT_DIR"), "/prism.common.rs"));
}

pub mod interfaces {
    include!(concat!(env!("OUT_DIR"), "/prism.interfaces.rs"));

    pub mod keyvalue {
        include!(concat!(env!("OUT_DIR"), "/prism.interfaces.keyvalue.rs"));
    }
}
