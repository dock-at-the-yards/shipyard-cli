# Shipyard CLI runtime image.
#
# This image does NOT compile Go. The evaluation engine (shipyard-core) is a
# private repository, so the build context cannot resolve the CLI's module
# dependencies. Instead the release pipeline cross-compiles the binary and
# passes it in, and this image only assembles a runtime around it.
#
# Runtime requirements, all of them real:
#   python3  - pkg/engine/rubric.go runs `python3 <script>` for every rubric
#   docker   - pkg/environment/docker.go shells out to the `docker` CLI and
#              talks to the daemon, normally via a mounted socket
#   git      - evalsets may reference remote agent repositories

FROM alpine:3.20

LABEL org.opencontainers.image.title="shipyard" \
      org.opencontainers.image.description="Evaluation harness and execution engine for autonomous AI agents" \
      org.opencontainers.image.licenses="LicenseRef-Shipyard" \
      org.opencontainers.image.source="https://github.com/dock-at-the-yards/shipyard-cli"

RUN apk add --no-cache \
      bash \
      ca-certificates \
      docker-cli \
      git \
      python3 \
      py3-pip \
 && rm -rf /var/cache/apk/*

# TARGETARCH is supplied automatically by buildx for linux/amd64 and
# linux/arm64, which is how the correct prebuilt binary is selected.
ARG TARGETARCH
COPY dist/shipyard-linux-${TARGETARCH} /usr/local/bin/shipyard

RUN chmod +x /usr/local/bin/shipyard

# Fail the build if the binary cannot report its own version, so a broken
# image can never be published.
RUN shipyard version

WORKDIR /src
VOLUME [ "/src" ]

ENTRYPOINT [ "shipyard" ]
CMD [ "--help" ]
