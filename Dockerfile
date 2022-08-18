# syntax=docker/dockerfile:1.4

FROM golang:1.18 as builder

COPY . /src/
WORKDIR /src

RUN  --mount=type=cache,target=/root/.local/share/golang \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

ENV LD_FLAGS="-s -w -extldflags -static"
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.local/share/golang \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "$LD_FLAGS" -o kube-vip-cloud-provider

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /src/kube-vip-cloud-provider /
USER nonroot:nonroot

CMD ["/kube-vip-cloud-provider"]
