FROM golang:1.25-alpine

COPY go.mod go.sum ./
RUN apk add --no-cache git
RUN go mod download

COPY *.go ./
RUN go build -o /home/app .

FROM chromedp/headless-shell:latest

COPY --from=0 /home/app /home/app

ENTRYPOINT ["/home/app"]