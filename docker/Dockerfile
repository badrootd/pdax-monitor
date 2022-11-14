# This image aggregates Docker layers that change infrequently so that they can be cached and re-build only occasionally.
FROM alpine:3.14 AS base


RUN apk add --no-cache \
            make==4.3-r0 \
            gcc==10.2.1_pre1-r3 \
            libc-dev==0.7.2-r3 \
            git==2.30.3-r0
WORKDIR /opt/service/
ARG CI_BRANCH
COPY . .
RUN make build

# Copy to fresh image to keep final image small and clean.
FROM scratch
USER nobody
COPY --from=base /etc/passwd /etc/passwd
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=base /opt/service/bin /bin
