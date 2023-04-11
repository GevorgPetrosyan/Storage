FROM golang:1.20-alpine

WORKDIR /storage

COPY go.mod go.sum ./
COPY promotions.csv ./
RUN go mod download

COPY *.go ./

RUN go build -o /storage-app

EXPOSE 1321

CMD [ "/storage-app" ]
