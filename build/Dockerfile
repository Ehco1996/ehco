FROM debian:12-slim

RUN apt update && apt install -y --no-install-recommends ca-certificates curl glibc-source libc6

WORKDIR /bin/

# Copy the pre-built binary file from the previous stage
COPY ehco .
RUN chmod +x ehco

ENTRYPOINT ["ehco"]
