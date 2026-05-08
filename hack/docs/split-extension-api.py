#!/usr/bin/env python3
"""Split extension API reference into per-operation pages.

Groups WorkspaceConnectionRequest + WorkspaceConnectionResponse into one
"Connection" page, and creates separate pages for BearerTokenReview and
ConnectionAccessReview.

Usage: split-extension-api.py <input.md> <output-dir>
"""
import re
import sys
from pathlib import Path


# Map section names to output pages
PAGE_MAP = {
    "BearerTokenReview": "bearer-token-review",
    "BearerTokenReviewSpec": "bearer-token-review",
    "BearerTokenReviewStatus": "bearer-token-review",
    "BearerTokenReviewUser": "bearer-token-review",
    "ConnectionAccessReview": "connection-access-review",
    "ConnectionAccessReviewSpec": "connection-access-review",
    "ConnectionAccessReviewStatus": "connection-access-review",
    "WorkspaceConnectionRequest": "connection",
    "WorkspaceConnectionRequestSpec": "connection",
    "WorkspaceConnectionResponse": "connection",
    "WorkspaceConnectionResponseStatus": "connection",
}

PAGE_TITLES = {
    "bearer-token-review": "BearerTokenReview",
    "connection-access-review": "ConnectionAccessReview",
    "connection": "Connection",
}


def main():
    input_path = Path(sys.argv[1])
    output_dir = Path(sys.argv[2])
    output_dir.mkdir(parents=True, exist_ok=True)

    content = input_path.read_text()

    # Parse into sections at ### boundaries
    sections: dict[str, list[str]] = {}
    current_name = None
    current_lines: list[str] = []

    for line in content.split("\n"):
        if line.startswith("### ") and "Resource Types" not in line:
            if current_name:
                sections[current_name] = current_lines
            current_name = line[4:].strip()
            current_lines = [line]
        elif current_name:
            current_lines.append(line)

    if current_name:
        sections[current_name] = current_lines

    # Group sections into pages
    pages: dict[str, list[tuple[str, list[str]]]] = {
        slug: [] for slug in PAGE_TITLES
    }
    for name, lines in sections.items():
        slug = PAGE_MAP.get(name)
        if slug and slug in pages:
            pages[slug].append((name, lines))

    # Write pages
    for slug, page_sections in pages.items():
        title = PAGE_TITLES[slug]
        out_path = output_dir / f"{slug}.md"

        out_lines = [f"# {title}\n"]
        for name, lines in page_sections:
            # Demote ### to ##
            section_lines = list(lines)
            if section_lines and section_lines[0].startswith("### "):
                section_lines[0] = "##" + section_lines[0][3:]
            out_lines.append("\n".join(section_lines))
            out_lines.append("")

        out_path.write_text("\n".join(out_lines))
        print(f"{slug}.md")


if __name__ == "__main__":
    main()
