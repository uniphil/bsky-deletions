ARG GO_VERSION=1
FROM golang:${GO_VERSION}-alpine as builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
ENV GOCACHE=/srv/go-build
RUN --mount=type=cache,id=go-build,sharing=locked,target=/srv/go-build \
    go build -v -o /run-app .


FROM alpine:latest

COPY --from=builder /run-app /usr/local/bin/
CMD ["run-app"]
