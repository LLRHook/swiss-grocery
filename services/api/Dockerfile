FROM rust:1.93.1-alpine3.23 AS optimizer

WORKDIR /optimizer
COPY optimizer/Cargo.toml optimizer/Cargo.lock ./
COPY optimizer/src ./src
RUN cargo build --release --locked --offline

# Match the go.mod minimum with the current patch of the same Go minor line.
FROM golang:1.26.10-alpine3.23 AS builder

ENV GOTOOLCHAIN=local

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o server .

FROM alpine:3.23

RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /server
COPY --from=optimizer /optimizer/target/release/swiss-optimizer /swiss-optimizer

EXPOSE 8080
CMD ["/server"]
