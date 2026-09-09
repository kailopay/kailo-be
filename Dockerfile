FROM golang:1.25-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags='-s -w' -mod=readonly -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags='-s -w' -mod=readonly -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags='-s -w' -mod=readonly -o /out/worker ./cmd/worker

FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget \
    && addgroup -S -g 10001 kailopay \
    && adduser -S -D -H -u 10001 -G kailopay kailopay

WORKDIR /app

COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate
COPY --from=build /out/worker /app/worker
COPY --from=build /src/migrations /app/migrations

RUN chmod 0555 /app/api /app/migrate /app/worker \
    && chown -R kailopay:kailopay /app

USER kailopay

EXPOSE 8080

CMD ["/app/api"]
