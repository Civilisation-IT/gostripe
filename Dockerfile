FROM golang:1.20-alpine as build
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ENV GOOS=linux

RUN apk add --no-cache make git

WORKDIR /go/src/github.com/open-urbex-map/gostripe

# Pulling dependencies
COPY go.* ./
RUN go mod download

# Building the binary
COPY . .
RUN go build -ldflags "-X github.com/open-urbex-map/gostripe/cmd.Version=`git rev-parse HEAD || echo 'unknown'`" -o gostripe

# Final stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app/
COPY --from=build /go/src/github.com/open-urbex-map/gostripe/gostripe .
COPY --from=build /go/src/github.com/open-urbex-map/gostripe/migrations ./migrations

ENTRYPOINT ["/app/gostripe"]
CMD ["migrate", "&&", "serve"]
