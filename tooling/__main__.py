"""Main entry point for Prism tooling CLI."""

import click

from tooling.codegen import codegen
from tooling.test.local_stack import local_stack


@click.group()
@click.version_option()
def cli():
    """Prism development tooling."""
    pass


cli.add_command(codegen)
cli.add_command(local_stack)


if __name__ == "__main__":
    cli()
