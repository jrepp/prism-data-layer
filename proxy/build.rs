use std::env;
use std::path::PathBuf;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Get the manifest directory (proxy/)
    let manifest_dir = env::var("CARGO_MANIFEST_DIR")?;
    let manifest_path = PathBuf::from(&manifest_dir);

    // Path to proto files (relative to manifest directory)
    let proto_root = manifest_path.parent().unwrap().join("proto");

    // Proto files to compile
    // Note: Proto files reorganized to match MEMO-006 three-layer architecture
    let proto_files = vec![
        proto_root.join("prism/interfaces/lifecycle.proto"),
        proto_root.join("prism/interfaces/keyvalue/keyvalue_basic.proto"),
    ];

    // Include paths for imports
    let include_paths = vec![proto_root.clone()];

    // Configure tonic build - use default OUT_DIR (target/build/.../out/)
    // We'll include the generated code via a module in src/proto.rs
    //
    // Generate both client and server for all services:
    // - Pattern lifecycle: use client (proxy connects TO patterns)
    // - KeyValue data: use server (clients connect TO proxy)
    tonic_build::configure()
        .build_server(true)  // Generate server stubs for KeyValue service
        .build_client(true)  // Generate client stubs for Pattern lifecycle
        .compile_protos(&proto_files, &include_paths)?;

    // Tell Cargo to rerun build.rs if proto files change
    for proto_file in &proto_files {
        println!("cargo:rerun-if-changed={}", proto_file.display());
    }

    // Also watch proto directory
    println!("cargo:rerun-if-changed=../proto");

    Ok(())
}
