FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /padelleague .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /padelleague /app/padelleague
COPY entrypoint.sh /app/entrypoint.sh
WORKDIR /app
EXPOSE 8090
ENTRYPOINT ["./entrypoint.sh"]
