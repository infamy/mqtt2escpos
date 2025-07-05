ARG BUILD_FROM
FROM $BUILD_FROM

# Install build dependencies
RUN apk add --no-cache go git

WORKDIR /build
COPY ../go.mod ../go.sum ./
RUN go mod download
COPY ../ .
RUN go build -o mqtt2escpos main.go

FROM $BUILD_FROM
RUN apk add --no-cache ca-certificates
COPY --from=0 /build/mqtt2escpos /usr/local/bin/mqtt2escpos
COPY run.sh /run.sh
RUN chmod a+x /run.sh
WORKDIR /data
CMD [ "/run.sh" ] 