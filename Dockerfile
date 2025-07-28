ARG ARCH="amd64"
ARG OS="linux"
FROM golang:1.24 AS builderimage
LABEL maintainer="Akamai SRE Observability Team support@linode.com"
WORKDIR /go/src/o11y-canary
COPY . .
RUN go build -o o11y-canary cmd/main.go

###################################################################

FROM golang:1.24
COPY --from=builderimage /go/src/o11y-canary/o11y-canary /app/
WORKDIR /app

EXPOSE      8080
USER        nobody
ENTRYPOINT  [ "./o11y-canary" ]
