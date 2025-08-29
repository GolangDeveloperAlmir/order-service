FROM golang:1.25-alpine as build

WORKDIR /src

RUN apk add --no-cache git build-base

COPY go.mod ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/order-service ./cmd/order-service

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app

COPY --from=build /out/order-service /app/order-service

COPY openapi.yaml /app/openapi.yaml

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT [ "/app/order-service" ]
