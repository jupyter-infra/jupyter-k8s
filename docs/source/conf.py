import time

project = "jupyter-k8s"
copyright = f"2025–{time.localtime().tm_year}, Amazon Web Services"
author = "Amazon Web Services"
html_title = "jupyter-k8s"

extensions = [
    "myst_parser",
    "sphinx_design",
    "sphinx_copybutton",
]
myst_enable_extensions = ["colon_fence"]

templates_path = ["_templates"]
exclude_patterns = []

html_theme = "shibuya"
html_static_path = ["_static"]
html_logo = "_static/img/jupyter_logo.png"

html_css_files = [
    "css/custom.css",
]

html_theme_options = {
    "accent_color": "orange",
    "github_url": "https://github.com/jupyter-infra/jupyter-k8s",
    "globaltoc_expand_depth": 1,
    "nav_links": [
        {
            "title": "Getting Started",
            "url": "getting-started/index",
        },
        {
            "title": "Core Concepts",
            "url": "core-concepts/index",
        },
        {
            "title": "Applications",
            "url": "applications/index",
        },
        {
            "title": "Dive Deeper",
            "url": "dive-deeper/index",
        },
        {
            "title": "Integrations",
            "url": "integrations/index",
        },
        {
            "title": "Contributor Guide",
            "url": "contributor-guide/index",
        },
    ],
}
