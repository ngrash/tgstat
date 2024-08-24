FROM golang:1.23

WORKDIR /tgstat

# Add and download dependencies first, so that they are cached.
ADD go.mod go.sum ./
RUN go mod download -x

# Add the rest of the code and build.
ADD . .
RUN go build -v -o tgstat

# Remove everything. We only care for the cache.
RUN rm -rf /tgstat

# Code is mounted via volume in docker-compose.yml
ENTRYPOINT ["go", "run", "."]