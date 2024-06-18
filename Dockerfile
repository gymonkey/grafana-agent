# syntax=docker/dockerfile:1.4

# NOTE: This Dockerfile can only be built using BuildKit. BuildKit is used by
# default when running `docker buildx build` or when DOCKER_BUILDKIT=1 is set
# in environment variables.

FROM  grafana/agent-build-image:0.40.2 as build
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG RELEASE_BUILD=1
ARG VERSION
ARG GOEXPERIMENT

COPY . /src/agent
WORKDIR /src/agent

# Build the UI before building the agent, which will then bake the final UI
# into the binary.
#   make generate-ui

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    RELEASE_BUILD=${RELEASE_BUILD} VERSION=${VERSION} \
    GO_TAGS="netgo builtinassets promtail_journal_enabled" \
    GOEXPERIMENT=${GOEXPERIMENT} \
    make agent

FROM public.ecr.aws/ubuntu/ubuntu:mantic

#Username and uid for grafana-agent user
ARG UID=473
ARG USERNAME="grafana-agent"

LABEL org.opencontainers.image.source="https://github.com/grafana/agent"

# Install dependencies needed at runtime.
RUN apt-get update && apt-get install -qy libsystemd-dev tzdata ca-certificates libcap2-bin && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY --from=build /src/agent/build/grafana-agent /bin/grafana-agent
COPY cmd/grafana-agent/agent-local-config.yaml /etc/agent/agent.yaml

# Create grafana-agent user in container, but do not set it as default
RUN groupadd --gid $UID $USERNAME
RUN useradd -m -u $UID -g $UID $USERNAME
RUN chown -R $USERNAME:$USERNAME /etc/agent
RUN chown -R $USERNAME:$USERNAME /bin/grafana-agent
RUN setcap 'cap_net_bind_service=+ep' /bin/grafana-agent

ENTRYPOINT ["/bin/grafana-agent"]
ENV AGENT_DEPLOY_MODE=docker
CMD ["--config.file=/etc/agent/agent.yaml", "--metrics.wal-directory=/etc/agent/data"]
