FROM golang:1.21-bullseye as build

WORKDIR /nkml
COPY go.mod go.sum .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o nkml

FROM scratch
COPY --from=build /nkml/nkml .
ENTRYPOINT ["./nkml"]
