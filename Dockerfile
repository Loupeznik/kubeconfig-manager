FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w \
      -X github.com/loupeznik/kubeconfig-manager/internal/cli.Version=${VERSION} \
      -X github.com/loupeznik/kubeconfig-manager/internal/cli.Commit=${COMMIT} \
      -X github.com/loupeznik/kubeconfig-manager/internal/cli.Date=${DATE}" \
    -o /out/kcm ./cmd/kubeconfig-manager

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/kcm /usr/local/bin/kcm
COPY LICENSE NOTICE /
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/kcm"]
