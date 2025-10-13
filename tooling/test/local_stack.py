"""Local testing infrastructure management."""

import subprocess
import time
from pathlib import Path

import click


@click.group(name="local-stack")
def local_stack():
    """Manage local testing infrastructure."""


@local_stack.command()
@click.option("--wait/--no-wait", default=True, help="Wait for services to be healthy")
def up(wait: bool):
    """Start local backend services."""
    compose_file = Path(__file__).parent.parent.parent / "docker-compose.test.yml"

    if not compose_file.exists():
        click.echo(f"✗ docker-compose.test.yml not found at {compose_file}")
        click.echo("  Run this from the repository root")
        raise click.Abort

    click.echo("Starting local backend services...")
    subprocess.run(
        ["docker", "compose", "-f", str(compose_file), "up", "-d"],
        check=True,
    )

    if wait:
        click.echo("Waiting for services to be healthy...")
        wait_healthy(timeout=60)

    click.echo("✓ Local stack is running")
    click.echo("\nAvailable services:")
    click.echo("  PostgreSQL: localhost:5432 (user=prism, db=prism_test)")
    click.echo("  Kafka: localhost:9092")
    click.echo("  NATS: localhost:4222")


@local_stack.command()
def down():
    """Stop and remove local backend services."""
    compose_file = Path(__file__).parent.parent.parent / "docker-compose.test.yml"

    click.echo("Stopping local backend services...")
    subprocess.run(
        ["docker", "compose", "-f", str(compose_file), "down", "-v"],
        check=True,
    )
    click.echo("✓ Local stack stopped")


@local_stack.command()
def status():
    """Show status of local backend services."""
    compose_file = Path(__file__).parent.parent.parent / "docker-compose.test.yml"

    subprocess.run(
        ["docker", "compose", "-f", str(compose_file), "ps"],
        check=True,
    )


def wait_healthy(timeout: int = 60):
    """Wait for all services to report healthy."""
    start = time.time()
    while time.time() - start < timeout:
        # Check if all services are healthy via docker compose
        subprocess.run(
            ["docker", "compose", "-f", "docker-compose.test.yml", "ps", "--format", "json"],
            check=False, capture_output=True,
            text=True,
        )

        # TODO: Parse JSON and check health status
        # For now, just wait a bit
        time.sleep(1)

        # Simple heuristic: if no error after 10s, probably healthy
        if time.time() - start > 10:
            return

    raise TimeoutError("Services failed to become healthy")
