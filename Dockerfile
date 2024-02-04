FROM golang:1.21-bullseye as builder

ENV CGO_ENABLED=0

WORKDIR /src
RUN --mount=target=/src,type=bind,source=. --mount=type=cache,target=/root/.cache/go-build go build -o /sshpiper-gh -buildvcs=false -tags timetzdata

FROM farmer1992/sshpiperd:v1.2.6

ENV PLUGIN=sshpiper-gh

COPY --from=builder /sshpiper-gh /sshpiperd/plugins/
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ADD web.tmpl /sshpiperd/plugins/

WORKDIR /sshpiperd/plugins/

EXPOSE 2222 3000

