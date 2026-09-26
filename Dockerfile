# syntax=docker/dockerfile:1@sha256:ecfaec9ed6d810b56388c508f4121597bfbba70d41a6dfeee4d8cad5f295fc32

# --- Stage 1: fetch the fbc binary for the TARGET arch (runs on the build host) ---
FROM --platform=$BUILDPLATFORM bellsoft/alpaquita-linux-base@sha256:b2a795bdfbf97bc2bef7bc32f1076440e0c9a4ee46ffc7c7ec3832483a3cd0fc AS fbc
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
FROM bellsoft/hardened-base@sha256:1f03f94eb77af9ccadaaaf0c800f969b49cfd2cee7dc6517ed84e4f63d4a960c
COPY --from=fbc /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080 TMPDIR=/tmp \
    PRESETS_DIR=/data/presets JOBS_DIR=/tmp/fb2cng-jobs
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
