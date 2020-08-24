FROM golang:1.15.0-alpine3.12 as builder
RUN mkdir /go-src
WORKDIR /go-src

# Install dependencies first.
COPY ./go-src/go.mod ./go-src/go.sum ./
RUN go mod download

# Copy and build src.
COPY ./go-src/*.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-extldflags "-static"' -o /sidecar

FROM scratch
COPY --from=builder /sidecar /sidecar
CMD ["/sidecar"]