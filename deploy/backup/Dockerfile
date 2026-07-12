FROM alpine:3.22

RUN apk add --no-cache aws-cli bash ca-certificates coreutils postgresql16-client python3

COPY scripts/backup-to-object-storage.sh /usr/local/bin/backup-to-object-storage
COPY scripts/verify-backup-manifest.sh /usr/local/bin/verify-backup-manifest
COPY scripts/restore-drill.sh /usr/local/bin/restore-drill

RUN chmod 0555 /usr/local/bin/backup-to-object-storage /usr/local/bin/verify-backup-manifest /usr/local/bin/restore-drill \
    && addgroup -S backup -g 10002 \
    && adduser -S backup -u 10002 -G backup

USER backup
WORKDIR /work
ENTRYPOINT ["/usr/local/bin/backup-to-object-storage"]
