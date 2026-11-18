FROM golang:1.26-alpine AS build

WORKDIR /app
COPY go.mod ./
# No external dependencies yet; copy the full source and build the binary.
COPY . .
RUN go build -o spok ./cmd/spok

FROM alpine:3.22
RUN apk add --no-cache ffmpeg
COPY --from=build /app/spok /usr/local/bin/spok

# Don't run the ffmpeg-wrapping service as root.
USER nobody

EXPOSE 9090
ENTRYPOINT ["spok"]
