# Optional: the VPS setup in docs/SELF-HOSTING.md runs the binary under systemd.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /notehop ./cmd/notehop && mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /notehop /notehop
# Owned by the image's nonroot user so a fresh volume is writable.
COPY --from=build --chown=65532:65532 /data /data
ENV NOTEHOP_ADDR=:8080 NOTEHOP_DB=/data/notehop.db
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/notehop"]
