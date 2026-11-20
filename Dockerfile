# syntax=docker/dockerfile:1@sha256:572cebbc2b15e6f84c077742952674889cb63cd5e9b80abdffcbe587b6f19183

# --- Stage 1: fetch the fbc binary for the TARGET arch (runs on the build host) ---
FROM --platform=$BUILDPLATFORM bellsoft/alpaquita-linux-base@sha256:04e4e54b5e11a6499e8e217217e562dc13516a5c94eda867e6d9abfec75e5156 AS fbc
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
FROM bellsoft/hardened-base@sha256:e7455a0c78c95528013596d7fe36d4782777f44599d5d9e120e172e97be34bec
COPY --from=fbc /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080 TMPDIR=/tmp \
    PRESETS_DIR=/data/presets JOBS_DIR=/tmp/fb2cng-jobs
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
