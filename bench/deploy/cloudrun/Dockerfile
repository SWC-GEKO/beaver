# Generic image for evaluation/gcf-single and evaluation/gcf-batch (the original function
# code is compiled unchanged; only cmd/main.go is swapped for a Cloud Run compatible one).
FROM golang:1.26-alpine AS build
ENV CGO_ENABLED=0
WORKDIR /src
COPY . .
RUN go mod tidy && go build -o /out/server ./cmd

FROM alpine:3.20
COPY --from=build /out/server /server
CMD ["/server"]
