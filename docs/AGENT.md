# Documentation

Docs are built with Sphinx + MyST Markdown and the Shibuya theme.
Source files live in `docs/source/`.

## Building

From the repo root:
- `make docs` — build HTML to `docs/build/`
- `make docs-serve` — live-reload dev server

## Formatting rules

### Product names in body text

Use bold for product names in running text:
- **Jupyter K8s**, **Extension API**, **Auth middleware**, **JupyterLab**, **Traefik**

Use **Jupyter K8s** (not "the operator") when referring to the project as subject.
Use **Extension API** and **Auth middleware** (not "the extension API server" or "the auth middleware component").

Use plain text (no bold, no backticks) in headings and `{toctree}` entries:
- `## JupyterLab` — correct
- `## **JupyterLab**` — wrong

### CLI and resource references

Use backticks for Kubernetes resource kinds (`Workspace`, `WorkspaceTemplate`),
field names (`spec.template`), and CLI commands (`kubectl`, `helm`).

### Voice

Prefer active voice over passive. For example:
- "The operator creates a Pod for each workspace" — correct
- "A Pod is created for each workspace by the operator" — avoid

### Headings

Never embed bold (`**`), backticks (`` ` ``), or other inline formatting in
markdown headings (`#`, `##`, `###`, etc.).

### Naming and ordering consistency

Directory names, page titles, and `nav_links` entries must match each other
(e.g. `concepts/` dir, "Concepts" title).
The order of `nav_links` in `conf.py` must match the `{toctree}` order in `index.md`.

## Diagrams

Refer to `diagrams/AGENT.md` for compiling diagrams, adding icons,
and the conventions for referencing SVGs in docs.

## Site structure

The site has two top-level nav tabs:
- **Documentation** — narrative content (Getting Started through Contributor Guide)
- **Reference** — auto-generated CRD specs, Connections API, and Helm chart values

These are implemented as separate `{toctree}` groups rendered by Shibuya as tabs.
