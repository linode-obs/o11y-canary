FROM --platform=$TARGETPLATFORM golang:1.24
COPY o11y-canary /app/
WORKDIR /app

EXPOSE      8080
USER        nobody
ENTRYPOINT  [ "./o11y-canary" ]
