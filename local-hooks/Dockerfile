# Build the cpu tool
FROM golang as builder
ADD . /go/src/hooks
WORKDIR /go/src/hooks
RUN make install INSTALLDIR=hooks-bin

# Create a container for hosting the binary
FROM busybox
COPY --from=builder /go/src/hooks/hooks-bin /hooks-bin
