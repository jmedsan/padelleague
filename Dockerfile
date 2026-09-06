FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-X main.Version=$(date -u +%Y%m%d-%H%M%S)" -o /padelleague .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates rclone
COPY --from=build /padelleague /app/padelleague
COPY entrypoint.sh /app/entrypoint.sh
WORKDIR /app
EXPOSE 8090
RUN adduser -D -H app && chown -R app:app /app
USER app
ENTRYPOINT ["./entrypoint.sh"]
