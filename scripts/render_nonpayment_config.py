"""Render the current block-style service config with payments explicitly disabled.

This is a narrow transformation of the canonical layout, not a YAML parser.
Unsupported root syntax is rejected instead of interpreting another layout.
"""

from __future__ import annotations

import argparse
from pathlib import Path
import re

ROOT_BLOCK = re.compile(r"([a-z][a-z0-9_]*):[ ]*(?:#[^\n]*)?\n?")
SERVER_PORT = re.compile(r"  port: ([0-9]+)([ ]*(?:#[^\n]*)?)(\n?)")


def render_nonpayment_config(source: str, port: int | None) -> str:
    """Validate the canonical block boundaries before transforming the text."""
    if "\r" in source or "\t" in source:
        raise ValueError("config requires LF lines and space indentation")
    lines = source.splitlines(keepends=True)
    blocks: dict[str, int] = {}
    for index, line in enumerate(lines):
        if not line.strip() or line.startswith("#"):
            continue
        if line.startswith(" "):
            if not blocks:
                raise ValueError("config content precedes its root block")
            continue
        match = ROOT_BLOCK.fullmatch(line)
        if match is None:
            raise ValueError("config has unsupported root syntax")
        name = match.group(1)
        if name in blocks:
            raise ValueError(f"config has duplicate root block: {name}")
        blocks[name] = index

    if "payments" not in blocks:
        raise ValueError("config requires exactly one root payments block")
    payment_start = blocks["payments"]
    payment_end = next(
        (index for index in blocks.values() if index > payment_start), len(lines)
    )
    # Preserve the blank lines and root comments that separate the next section.
    while payment_end > payment_start + 1 and (
        not lines[payment_end - 1].strip() or lines[payment_end - 1].startswith("#")
    ):
        payment_end -= 1
    if not any(
        line.strip() and not line.lstrip().startswith("#")
        for line in lines[payment_start + 1 : payment_end]
    ):
        raise ValueError("payments requires the canonical nonempty block")

    if port is not None:
        if not 1 <= port <= 65535:
            raise ValueError("port must be between 1 and 65535")
        if "server" not in blocks:
            raise ValueError("config requires exactly one root server block")
        server_start = blocks["server"]
        server_end = next(
            (index for index in blocks.values() if index > server_start), len(lines)
        )
        port_lines = [
            index
            for index in range(server_start + 1, server_end)
            if lines[index].startswith("  port:")
        ]
        if len(port_lines) != 1:
            raise ValueError("config requires exactly one server.port")
        port_index = port_lines[0]
        match = SERVER_PORT.fullmatch(lines[port_index])
        if match is None:
            raise ValueError("server.port requires the canonical integer syntax")
        line = lines[port_index]
        lines[port_index] = line[: match.start(1)] + str(port) + line[match.end(1) :]

    newline = "\n" if lines[payment_start].endswith("\n") else ""
    return "".join(lines[:payment_start] + ["payments: null" + newline] + lines[payment_end:])


def main() -> None:
    """Read one explicit source and write only a fully validated result."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--port", type=int)
    arguments = parser.parse_args()
    try:
        if arguments.source.resolve() == arguments.output.resolve() or (
            arguments.output.exists() and arguments.source.samefile(arguments.output)
        ):
            raise ValueError("source and output must be different files")
        source = arguments.source.read_bytes().decode("utf-8")
        rendered = render_nonpayment_config(source, arguments.port)
        arguments.output.write_bytes(rendered.encode("utf-8"))
    except (OSError, UnicodeError, ValueError) as error:
        parser.exit(1, f"error: render nonpayment config: {error}\n")


if __name__ == "__main__":
    main()
