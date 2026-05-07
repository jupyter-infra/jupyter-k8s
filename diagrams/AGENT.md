# Diagrams

Architecture diagrams written in [d2](https://d2lang.com/), compiled to SVG.

## Structure

- `diagrams/` — d2 source files
- `diagrams/icons/` — SVG icons shared across diagrams

Compiled SVGs are output to `docs/source/_static/img/diagrams/`.

## Compiling diagrams

From the repo root: `make docs-diagrams`

This compiles all `.d2` files in `diagrams/` and writes SVGs to the docs static directory.

## Referencing diagrams in docs

Use relative paths from the doc page:

```markdown
![Architecture overview](/_static/img/diagrams/architecture-overview.svg)
```

## Adding icons

1. Place the SVG in `diagrams/icons/`
2. Add an attribution row in `diagrams/icons/ATTRIBUTION.md`
3. Reference the icon in d2 with `icon: icons/<name>.svg`

Most icons come from [simple-icons](https://github.com/simple-icons/simple-icons) (CC0 1.0).
Ensure each icon has an open license (MIT, CC0, etc), or ask the user to confirm.

## Adding a new diagram

1. Create a `.d2` file in `diagrams/`
2. Run `make docs-diagrams` to compile
3. Reference the SVG in the appropriate docs page
