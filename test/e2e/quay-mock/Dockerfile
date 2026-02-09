FROM golang:1.25 AS builder
WORKDIR /app
COPY main.go .
RUN CGO_ENABLED=0 go build -o quay-mock main.go

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/quay-mock /quay-mock
USER 65532:65532
ENTRYPOINT ["/quay-mock"]
