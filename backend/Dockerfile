FROM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api . \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/retention ./cmd/retention \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/catalog-import ./cmd/catalog-import \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/nginx-import ./cmd/nginx-import

FROM gcr.io/distroless/base-debian12:nonroot AS api
COPY --from=build --chown=nonroot:nonroot /out/api /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

FROM gcr.io/distroless/base-debian12:nonroot AS worker
COPY --from=build --chown=nonroot:nonroot /out/worker /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

FROM gcr.io/distroless/base-debian12:nonroot AS migrate
COPY --from=build --chown=nonroot:nonroot /out/migrate /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

FROM gcr.io/distroless/base-debian12:nonroot AS retention
COPY --from=build --chown=nonroot:nonroot /out/retention /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

FROM gcr.io/distroless/base-debian12:nonroot AS catalog-import
COPY --from=build --chown=nonroot:nonroot /out/catalog-import /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

FROM gcr.io/distroless/base-debian12:nonroot AS nginx-import
COPY --from=build --chown=nonroot:nonroot /out/nginx-import /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
