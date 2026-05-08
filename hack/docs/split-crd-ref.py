#!/usr/bin/env python3
"""Split a crd-ref-docs markdown file into per-resource pages.

Usage: split-crd-ref.py <input.md> <output-dir>

Each top-level resource (identified by having apiVersion/kind rows in its
field table) gets its own file. Sub-types are placed on the page of the
root resource they belong to, determined by "Appears in" references.
"""
import re
import sys
from pathlib import Path


def parse_sections(content: str) -> list[dict]:
    """Parse markdown into sections split at ### headings."""
    sections = []
    current = None

    for line in content.split("\n"):
        if line.startswith("### ") and "Resource Types" not in line:
            if current:
                sections.append(current)
            name = line[4:].strip()
            current = {"name": name, "lines": [line], "is_root": False}
        elif current:
            current["lines"].append(line)
            # Root resources have a GVK apiVersion with a backtick-quoted group/version value
            if re.match(r'^\| `apiVersion` _string_ \| `.+/.+`', line):
                current["is_root"] = True
        # Skip header content before first ### section

    if current:
        sections.append(current)

    return sections


def find_owner(section: dict, roots: list[str]) -> str | None:
    """Find which root resource a sub-type belongs to via Appears in links."""
    text = "\n".join(section["lines"])
    for root in roots:
        # Check direct reference, Spec, or Status
        patterns = [
            rf'\[{re.escape(root)}\]',
            rf'\[{re.escape(root)}Spec\]',
            rf'\[{re.escape(root)}Status\]',
        ]
        for pat in patterns:
            if re.search(pat, text):
                return root
    return None


def main():
    input_path = Path(sys.argv[1])
    output_dir = Path(sys.argv[2])
    output_dir.mkdir(parents=True, exist_ok=True)

    content = input_path.read_text()
    sections = parse_sections(content)

    # Identify root resources
    roots = [s["name"] for s in sections if s["is_root"]]

    # Build ownership map: sub-type -> root resource
    ownership: dict[str, str] = {}
    for s in sections:
        if s["is_root"]:
            ownership[s["name"]] = s["name"]
        else:
            owner = find_owner(s, roots)
            if owner:
                ownership[s["name"]] = owner

    # Second pass: assign unowned types by following reference chains
    changed = True
    while changed:
        changed = False
        for s in sections:
            if s["name"] in ownership:
                continue
            owner = find_owner(s, list(ownership.keys()))
            if owner and owner in ownership:
                ownership[s["name"]] = ownership[owner]
                changed = True

    # Group sections by owner
    groups: dict[str, list[dict]] = {root: [] for root in roots}
    for s in sections:
        owner = ownership.get(s["name"], roots[0] if roots else None)
        if owner is None or owner not in groups:
            print(f"WARNING: type '{s['name']}' could not be assigned to a root resource, defaulting to '{roots[0]}'", file=sys.stderr)
            owner = roots[0] if roots else None
        if owner and owner in groups:
            groups[owner].append(s)

    # Write per-resource files
    for root, group_sections in groups.items():
        filename = re.sub(r'[^a-z0-9]+', '-', root.lower()).strip('-') + ".md"
        out_path = output_dir / filename

        # Put root resource first, then sub-types alphabetically
        root_section = None
        sub_sections = []
        for s in group_sections:
            if s["name"] == root:
                root_section = s
            else:
                sub_sections.append(s)
        sub_sections.sort(key=lambda s: s["name"])

        ordered = []
        if root_section:
            ordered.append(root_section)
        ordered.extend(sub_sections)

        lines = [f"# {root}\n"]
        for s in ordered:
            section_lines = list(s["lines"])
            if section_lines and section_lines[0].startswith("### "):
                section_lines[0] = "##" + section_lines[0][3:]
            lines.append("\n".join(section_lines))
            lines.append("")

        out_path.write_text("\n".join(lines))

    # Print generated filenames for Makefile use
    for root in roots:
        filename = re.sub(r'[^a-z0-9]+', '-', root.lower()).strip('-') + ".md"
        print(filename)


if __name__ == "__main__":
    main()
