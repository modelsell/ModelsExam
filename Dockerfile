FROM oven/bun:1 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock* ./
RUN bun install
COPY web ./
RUN bun run build

FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -o /modelsexam ./cmd/model-check

FROM gcr.io/distroless/static-debian12
COPY --from=build /modelsexam /modelsexam
ENV LISTEN_ADDR=:8080 SQL_DSN=/data/modelsexam.db
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/modelsexam"]
