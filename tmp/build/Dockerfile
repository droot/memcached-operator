FROM alpine:3.6

ADD tmp/_output/bin/memcached-operator /usr/local/bin/memcached-operator

RUN adduser -D memcached-operator
USER memcached-operator
