"""Code generation from protobuf definitions."""

import subprocess
from pathlib import Path

import click


@click.group(name="codegen")
def codegen():
    """Generate code from protobuf definitions."""


@codegen.command()
@click.option("--proto-path", default="proto", help="Path to proto files")
@click.option("--check-only", is_flag=True, help="Only check buf installation and lint")
def generate(proto_path: str, check_only: bool):
    """Generate code from all proto files using buf."""
    proto_dir = Path(proto_path)

    if not proto_dir.exists():
        click.echo(f"✗ Proto directory not found: {proto_path}", err=True)
        return 1

    click.echo(f"📦 Generating code from {proto_path}...")

    # Check if buf is installed
    try:
        result = subprocess.run(["buf", "--version"], capture_output=True, text=True, check=True)
        click.echo(f"✓ buf version: {result.stdout.strip()}")
    except FileNotFoundError:
        click.echo("✗ buf not found. Please install buf:", err=True)
        click.echo("  macOS: brew install bufbuild/buf/buf")
        click.echo("  Linux: See https://docs.buf.build/installation")
        return 1
    except subprocess.CalledProcessError as e:
        click.echo(f"✗ buf check failed: {e}", err=True)
        return 1

    # Run buf lint
    click.echo("\n🔍 Running buf lint...")
    try:
        subprocess.run(["buf", "lint"], cwd=proto_dir, check=True)
        click.echo("✓ Lint passed")
    except subprocess.CalledProcessError as e:
        click.echo(f"✗ Lint failed with exit code {e.returncode}", err=True)
        return 1

    if check_only:
        click.echo("\n✓ Check complete (--check-only mode)")
        return 0

    # Generate Rust code
    click.echo("\n🦀 Generating Rust code...")
    try:
        subprocess.run(["buf", "generate", "--template", "buf.gen.rust.yaml"], cwd=proto_dir, check=True)
        click.echo("✓ Rust code generated")
    except subprocess.CalledProcessError as e:
        click.echo(f"⚠ Rust generation skipped (exit code {e.returncode})")
        click.echo("  This is expected if proxy/ directory doesn't exist yet")

    # Generate Python code
    click.echo("\n🐍 Generating Python code...")
    try:
        subprocess.run(["buf", "generate", "--template", "buf.gen.python.yaml"], cwd=proto_dir, check=True)
        click.echo("✓ Python code generated")
    except subprocess.CalledProcessError as e:
        click.echo(f"⚠ Python generation skipped (exit code {e.returncode})")
        click.echo("  This is expected if tooling/generated/ doesn't exist yet")

    click.echo("\n✓ Code generation complete!")
    return 0


@codegen.command()
@click.option("--proto-path", default="proto", help="Path to proto files")
def lint(proto_path: str):
    """Lint protobuf files."""
    proto_dir = Path(proto_path)

    if not proto_dir.exists():
        click.echo(f"✗ Proto directory not found: {proto_path}", err=True)
        return 1

    click.echo(f"🔍 Linting {proto_path}...")

    try:
        subprocess.run(["buf", "lint"], cwd=proto_dir, check=True)
        click.echo("✓ Lint passed")
        return 0
    except FileNotFoundError:
        click.echo("✗ buf not found. Please install buf:", err=True)
        click.echo("  macOS: brew install bufbuild/buf/buf")
        return 1
    except subprocess.CalledProcessError as e:
        click.echo(f"✗ Lint failed with exit code {e.returncode}", err=True)
        return 1


@codegen.command()
@click.option("--proto-path", default="proto", help="Path to proto files")
def format_check(proto_path: str):
    """Check protobuf formatting."""
    proto_dir = Path(proto_path)

    if not proto_dir.exists():
        click.echo(f"✗ Proto directory not found: {proto_path}", err=True)
        return 1

    click.echo(f"📝 Checking format for {proto_path}...")

    try:
        subprocess.run(["buf", "format", "--diff", "--exit-code"], cwd=proto_dir, check=True)
        click.echo("✓ Format check passed")
        return 0
    except FileNotFoundError:
        click.echo("✗ buf not found. Please install buf:", err=True)
        return 1
    except subprocess.CalledProcessError:
        click.echo("✗ Format check failed. Run 'buf format -w' to fix.", err=True)
        return 1
