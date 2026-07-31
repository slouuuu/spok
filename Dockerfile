FROM golang:1.26-alpine AS build

WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN go build -o spok .

FROM alpine:3.22
RUN apk add --no-cache ffmpeg
COPY --from=build /app/spok /usr/local/bin/spok

EXPOSE 9090
ENTRYPOINT ["spok"]
