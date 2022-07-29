FROM golang:1.18.4-bullseye as build
COPY . /nkml
WORKDIR /nkml
RUN CGO_ENABLED=0 go build --mod=vendor -o nkml

FROM scratch
COPY --from=build /nkml/nkml .
ENTRYPOINT ["./nkml"]
