FROM golang:1.25
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -v -o app cmd/server/main.go
CMD ["./app"]
