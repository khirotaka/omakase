FROM golang:1.26 AS build

WORKDIR /build
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o bin/omakase ./agent/...

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /build/bin/omakase /

ENTRYPOINT [ "/omakase" ]
