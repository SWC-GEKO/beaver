FROM golang:1.26-alpine

WORKDIR /usr/src/app

COPY . .

RUN go mod tidy
RUN go mod download
RUN go build -o handler .

EXPOSE 8000

CMD ["./handler"]


