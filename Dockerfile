# Build the controller binary.
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build

WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# TARGETOS/TARGETARCH are provided by buildx for cross-compilation.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /out/tdarr-operator .

# Minimal runtime image.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=build /out/tdarr-operator /tdarr-operator

USER 65532:65532

ENTRYPOINT ["/tdarr-operator"]
