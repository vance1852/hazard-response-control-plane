FROM --platform=$BUILDPLATFORM golang:1.22.5-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/hazardd ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/hazardd /app/hazardd
COPY migrations /app/migrations
VOLUME ["/tmp"]
EXPOSE 8080
ENV HAZARD_HTTP_ADDR=:8080
ENV HAZARD_DATABASE_PATH=/tmp/hazard.db
ENTRYPOINT ["/app/hazardd"]
