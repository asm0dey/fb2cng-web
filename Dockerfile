# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# --- Stage 1: fetch the fbc binary for the TARGET arch (runs on the build host) ---
FROM --platform=$BUILDPLATFORM bellsoft/alpaquita-linux-base@sha256:9c31d60aa6d12a472039d9486c6f7123f7d49a80265bcce0140b4609b0020813 AS fbc
ARG FBC_VERSION=v1.4.5
ARG TARGETARCH
RUN apk add --no-cache ca-certificates curl unzip \
 && curl -fsSL -o /tmp/fbc.zip \
      "https://github.com/rupor-github/fb2cng/releases/download/${FBC_VERSION}/fbc-linux-${TARGETARCH}.zip" \
 && unzip -o /tmp/fbc.zip -d /opt \
 && chmod +x /opt/fbc

# --- Stage 2: cross-compile the Go server (runs on the build host) ---
FROM --platform=$BUILDPLATFORM bellsoft/alpaquita-linux-go@sha256:f130131d2302b0b08c989ba1e8a43e115a5a98fa6f2339b10676564b0db17e69 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" go build -o /out/fb2cng-web .

# --- Stage 3: hardened runtime (per-arch image, COPY only) ---
FROM bellsoft/hardened-base@sha256:c34bb0c10884c48c79d76b2618eaccc27c66d3bc341c7b1d919c0e8975d596b5
COPY --from=fbc /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080 TMPDIR=/tmp \
    PRESETS_DIR=/data/presets JOBS_DIR=/tmp/fb2cng-jobs
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
