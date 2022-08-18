# syntax=docker/dockerfile:experimental

FROM golang:1.18 as builder

USER nobody

COPY . /src/
WORKDIR /src

ENV GO111MODULE=on
RUN --mount=type=cache,sharing=locked,id=gomod,target=/go/pkg/mod/cache \
    --mount=type=cache,sharing=locked,id=goroot,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags '-s -w -extldflags -static' -o kube-vip-cloud-provider

FROM scratch
COPY --from=builder /src/kube-vip-cloud-provider /
CMD ["/kube-vip-cloud-provider"]
