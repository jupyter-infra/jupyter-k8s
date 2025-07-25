# /// script
# dependencies = []
# ///

# This script verifies that the user ran `uv run ruff format`, and fails if running formatting
# would lead to any diff.
# This is useful to ensure we only merge commits that have been properly formatted.
# Ideally, `uv run ruff check` would fail if formatting hadn't been run, but that is not the case.

import argparse
import os
import subprocess
import sys


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify code formatting with ruff")
    parser.add_argument(
        "directory",
        nargs="?",
        default=".",
        help="Directory to check (default: current directory)",
    )
    args = parser.parse_args()

    directory = args.directory

    if not os.path.isdir(directory):
        print(f"Error: Directory '{directory}' does not exist")
        print()
        return 1

    format_result = subprocess.run(["uv", "run", "ruff", "format", "--diff", directory], capture_output=True)

    if format_result.stdout:
        print(
            f"Ruff found formatting issues in '{directory}'. Please run `uv run ruff format {directory}` to fix them.",
        )
        print(format_result.stdout.decode())
        print()
        return 1

    print(
        f"All good, confirmed that `uv run ruff format` would not make any change in '{directory}'.",
    )
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
