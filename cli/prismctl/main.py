"""Main CLI entry point for prismctl."""

import sys

import click

from .auth import OIDCAuthenticator, TokenManager
from .client import PrismClient
from .config import Config, ensure_prism_dir


@click.group()
@click.version_option()
@click.pass_context
def cli(ctx: click.Context) -> None:
    """Prism CLI - Manage Prism data access gateway."""
    ctx.ensure_object(dict)

    # Load configuration
    ctx.obj["config"] = Config.load()


@cli.command()
@click.option(
    "--device-code/--no-device-code", default=True, help="Use device code flow (recommended)",
)
@click.option("--username", help="Username for password flow (testing only)")
@click.option("--password", help="Password for password flow (testing only)")
@click.pass_context
def login(
    ctx: click.Context, device_code: bool, username: str | None, password: str | None
) -> None:
    """Authenticate with Prism using OIDC.

    By default, uses device code flow which is secure for CLI applications.
    For local testing, you can use --username and --password.
    """
    config = ctx.obj["config"]
    ensure_prism_dir()

    authenticator = OIDCAuthenticator(config.oidc)
    token_manager = TokenManager(config.token_path)

    try:
        if device_code:
            click.echo("Starting device code authentication...")
            token = authenticator.login_device_code(open_browser=True)
        elif username and password:
            click.echo("⚠️  Using password flow (testing only)")
            token = authenticator.login_password(username, password)
        else:
            click.echo(
                "Error: Either use device code (default) or provide --username and --password",
                err=True,
            )
            sys.exit(1)

        # Save token
        token_manager.save(token)

        # Get user info
        userinfo = authenticator.get_userinfo(token)
        click.echo("\n✅ Authenticated successfully!")
        click.echo(f"   User: {userinfo.get('name', userinfo.get('email', 'Unknown'))}")
        click.echo(f"   Token expires: {token.expires_at.strftime('%Y-%m-%d %H:%M:%S')}")
        click.echo(f"   Token saved to: {config.token_path}")

    except Exception as e:
        click.echo(f"❌ Authentication failed: {e}", err=True)
        sys.exit(1)


@cli.command()
@click.pass_context
def logout(ctx: click.Context) -> None:
    """Remove stored authentication token."""
    config = ctx.obj["config"]
    token_manager = TokenManager(config.token_path)

    if token_manager.token_path.exists():
        token_manager.delete()
        click.echo(f"✅ Token removed from {config.token_path}")
    else:
        click.echo("(i) No token found (already logged out)")


@cli.command()
@click.pass_context
def whoami(ctx: click.Context) -> None:
    """Show current authentication status."""
    config = ctx.obj["config"]
    token_manager = TokenManager(config.token_path)

    token = token_manager.load()
    if not token:
        click.echo("❌ Not authenticated. Run 'prism login' first.", err=True)
        sys.exit(1)

    if token.is_expired():
        click.echo("⚠️  Token expired. Run 'prism login' again.", err=True)
        sys.exit(1)

    authenticator = OIDCAuthenticator(config.oidc)
    try:
        userinfo = authenticator.get_userinfo(token)
        click.echo(f"✅ Authenticated as: {userinfo.get('name', userinfo.get('email', 'Unknown'))}")
        click.echo(f"   Email: {userinfo.get('email', 'N/A')}")
        click.echo(f"   Token expires: {token.expires_at.strftime('%Y-%m-%d %H:%M:%S')}")

        if token.needs_refresh():
            click.echo("   ⚠️  Token expires soon, consider refreshing")

    except Exception as e:
        click.echo(f"❌ Failed to get user info: {e}", err=True)
        sys.exit(1)


@cli.command()
@click.pass_context
def health(ctx: click.Context) -> None:
    """Check Prism proxy health."""
    config = ctx.obj["config"]
    client = PrismClient(config.proxy)

    try:
        health_data = client.health()
        click.echo("✅ Proxy is healthy")
        click.echo(f"   Status: {health_data.get('status', 'unknown')}")
    except Exception as e:
        click.echo(f"❌ Health check failed: {e}", err=True)
        sys.exit(1)


@cli.group()
def namespace() -> None:
    """Manage namespaces."""


@namespace.command("list")
@click.pass_context
def namespace_list(ctx: click.Context) -> None:
    """List all namespaces."""
    config = ctx.obj["config"]
    token_manager = TokenManager(config.token_path)

    token = token_manager.load()
    if not token:
        click.echo("❌ Not authenticated. Run 'prism login' first.", err=True)
        sys.exit(1)

    client = PrismClient(config.proxy, token)

    try:
        namespaces = client.list_namespaces()
        if not namespaces:
            click.echo("No namespaces found")
            return

        click.echo(f"Found {len(namespaces)} namespace(s):\n")
        for ns in namespaces:
            click.echo(f"  • {ns['name']}")
            if desc := ns.get("description"):
                click.echo(f"    {desc}")

    except Exception as e:
        click.echo(f"❌ Failed to list namespaces: {e}", err=True)
        sys.exit(1)


@namespace.command("show")
@click.argument("name")
@click.pass_context
def namespace_show(ctx: click.Context, name: str) -> None:
    """Show namespace details."""
    config = ctx.obj["config"]
    token_manager = TokenManager(config.token_path)

    token = token_manager.load()
    if not token:
        click.echo("❌ Not authenticated. Run 'prism login' first.", err=True)
        sys.exit(1)

    client = PrismClient(config.proxy, token)

    try:
        ns = client.get_namespace(name)
        click.echo(f"Namespace: {ns['name']}")
        click.echo(f"Description: {ns.get('description', 'N/A')}")
        click.echo(f"Created: {ns.get('created_at', 'N/A')}")
        click.echo(f"Backends: {', '.join(ns.get('backends', []))}")

    except Exception as e:
        click.echo(f"❌ Failed to get namespace: {e}", err=True)
        sys.exit(1)


@cli.group()
def session() -> None:
    """Manage sessions."""


@session.command("list")
@click.option("--namespace", help="Filter by namespace")
@click.pass_context
def session_list(ctx: click.Context, namespace: str | None) -> None:
    """List active sessions."""
    config = ctx.obj["config"]
    token_manager = TokenManager(config.token_path)

    token = token_manager.load()
    if not token:
        click.echo("❌ Not authenticated. Run 'prism login' first.", err=True)
        sys.exit(1)

    client = PrismClient(config.proxy, token)

    try:
        sessions = client.list_sessions(namespace)
        if not sessions:
            click.echo("No active sessions")
            return

        click.echo(f"Found {len(sessions)} active session(s):\n")
        click.echo(f"{'Session ID':<36} {'Principal':<30} {'Namespace':<20} {'Started'}")
        click.echo("-" * 100)

        for s in sessions:
            session_id = s.get("id", "N/A")[:35]
            principal = s.get("principal", "N/A")[:29]
            ns = s.get("namespace", "N/A")[:19]
            started = s.get("started_at", "N/A")[:19]
            click.echo(f"{session_id} {principal} {ns} {started}")

    except Exception as e:
        click.echo(f"❌ Failed to list sessions: {e}", err=True)
        sys.exit(1)


def main() -> None:
    """Entry point for prism command."""
    cli(obj={})


if __name__ == "__main__":
    main()
