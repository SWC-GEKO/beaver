FROM golang:1.26-alpine

WORKDIR /usr/src/app

COPY . .

RUN go mod tidy
RUN go mod download
RUN go build -o router .

EXPOSE 4222

CMD ["./router"]