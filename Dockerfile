FROM golang:1.16
WORKDIR /app
ENTRYPOINT ["/app/snat-race-conn-test"]
ADD ./go.mod go.sum /app/
RUN go mod download
ADD ./ /app/
RUN go build
