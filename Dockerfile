FROM golang:1.24-bookworm AS build

WORKDIR /nkml
COPY go.mod go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o nkml

FROM gcr.io/distroless/base
COPY --from=build /nkml/nkml .
ENTRYPOINT ["./nkml"]
