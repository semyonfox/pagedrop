FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/semyonfox/pagedrop/internal/app.version=${VERSION}" -o /pagedrop ./cmd/pagedrop

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S pagedrop && adduser -S -G pagedrop pagedrop
COPY --from=build /pagedrop /usr/local/bin/pagedrop
RUN mkdir /data && chown pagedrop:pagedrop /data
USER pagedrop
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["pagedrop"]
CMD ["serve"]
