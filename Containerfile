FROM docker.io/library/node:24.13.1-bookworm-slim@sha256:85a395c77b811fa7f5b5e4aa69cd6eb4c3b80c7f1a8e34704dc0ce061e5b404e AS web-builder
WORKDIR /src/web
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM docker.io/library/node:24.13.1-bookworm-slim@sha256:85a395c77b811fa7f5b5e4aa69cd6eb4c3b80c7f1a8e34704dc0ce061e5b404e AS worker-deps
WORKDIR /opt/roaminal/terminal-worker
COPY terminal-worker/package.json terminal-worker/package-lock.json ./
RUN npm ci --omit=dev
COPY terminal-worker/src ./src

FROM docker.io/library/golang:1.26.5-bookworm@sha256:db25d241820546be7b96953eea8d3e6bd15d413d59d00a75b68b74dfb5e2ecd2 AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=web-builder /src/web/dist /src/web/dist
RUN rm -rf internal/webassets/dist && cp -a /src/web/dist internal/webassets/dist
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/roaminal ./cmd/roaminal

FROM docker.io/library/node:24.13.1-bookworm-slim@sha256:85a395c77b811fa7f5b5e4aa69cd6eb4c3b80c7f1a8e34704dc0ce061e5b404e
RUN apt-get update \
    && apt-get install -y --no-install-recommends bash ca-certificates tini \
    && rm -rf /var/lib/apt/lists/* \
    && groupmod --new-name roaminal node \
    && usermod --login roaminal --home /home/roaminal --move-home node \
    && rm -rf /usr/local/lib/node_modules/npm /usr/local/lib/node_modules/corepack /usr/local/lib/node_modules/yarn \
    && rm -f /usr/local/bin/npm /usr/local/bin/npx /usr/local/bin/corepack /usr/local/bin/yarn /usr/local/bin/yarnpkg \
    && mkdir -p /opt/roaminal/terminal-worker /home/roaminal/.roaminal /workspace \
    && chown -R roaminal:roaminal /opt/roaminal /home/roaminal /workspace
COPY --from=go-builder /out/roaminal /usr/local/bin/roaminal
COPY --from=worker-deps --chown=roaminal:roaminal /opt/roaminal/terminal-worker /opt/roaminal/terminal-worker
COPY --chown=roaminal:roaminal shell /opt/roaminal/shell
RUN chmod 0755 /usr/local/bin/roaminal && chmod 0755 /opt/roaminal/shell/roaminal-bashrc
ENV HOME=/home/roaminal \
    ROAMINAL_HOST=0.0.0.0 \
    ROAMINAL_PORT=9846 \
    ROAMINAL_CWD=/workspace \
    ROAMINAL_ACCEPT_TERMS=true \
    ROAMINAL_WORKER_PATH=/opt/roaminal/terminal-worker/src/index.mjs \
    ROAMINAL_SHELL_RC=/opt/roaminal/shell/roaminal-bashrc
VOLUME ["/home/roaminal/.roaminal", "/workspace"]
EXPOSE 9846
USER 1000:1000
WORKDIR /home/roaminal
HEALTHCHECK --interval=10s --timeout=2s --start-period=10s --retries=3 CMD ["node", "-e", "fetch('http://127.0.0.1:9846/healthz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"]
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/roaminal"]
