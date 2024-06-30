FROM golang:1.22.4 as builder

WORKDIR /work

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY main.go .

RUN CGO_ENABLED=0 go build -trimpath -o server .

FROM scratch

COPY --from=builder /work/server .

USER 10000

ENTRYPOINT [ "/server" ]
