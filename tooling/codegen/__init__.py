"""Code generation from protobuf definitions."""

import click


@click.group(name="codegen")
def codegen():
    """Generate code from protobuf definitions."""
    pass


@codegen.command()
@click.option("--proto-path", default="proto", help="Path to proto files")
@click.option("--out-rust", default="proxy/src/generated", help="Rust output directory")
@click.option("--out-python", default="tooling/generated", help="Python output directory")
@click.option("--out-typescript", default="admin/app/models/generated", help="TypeScript output")
def generate(proto_path: str, out_rust: str, out_python: str, out_typescript: str):
    """Generate code from all proto files."""
    click.echo(f"Generating code from {proto_path}...")
    click.echo(f"  Rust: {out_rust}")
    click.echo(f"  Python: {out_python}")
    click.echo(f"  TypeScript: {out_typescript}")

    # TODO: Implement actual code generation
    click.echo("✗ Code generation not yet implemented")
    click.echo("  See ADR-003 for implementation plan")
