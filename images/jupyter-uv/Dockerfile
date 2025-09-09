FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim

USER root

# 1/ Create a non-root user "jovyan" with a home dir
RUN useradd -m -s /bin/bash jovyan

# 2/ create jupyter default config dirs
RUN mkdir -p /etc/jupyter
COPY jupyter_server_config.py /etc/jupyter/

# 3/ create a dir for UV, copy jupyter toml file, run uv
RUN mkdir -p /opt/uv/jupyter
COPY pyproject.jupyter.toml /opt/uv/jupyter/pyproject.toml
RUN uv sync --directory /opt/uv/jupyter

# 4/ create the start-up script
COPY jupyter-start.sh /usr/local/bin/
COPY jupyter-reset.sh /usr/local/bin/

# 5/ Change dirs ownership and visibility
RUN chmod -R a+r /opt/uv
RUN chmod -R a+r /etc/jupyter
RUN chmod +x /usr/local/bin/jupyter-start.sh
RUN chmod +x /usr/local/bin/jupyter-reset.sh
RUN chown -R jovyan:jovyan /home/jovyan

# 6/ Switch user and workdir
WORKDIR /home/jovyan
USER jovyan

ENTRYPOINT ["/usr/local/bin/jupyter-start.sh"]