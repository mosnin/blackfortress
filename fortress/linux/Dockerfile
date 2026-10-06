# Black Fortress runtime image. Build from an extracted Linux release:
#   docker build -t blackfortress -f fortress/linux/Dockerfile blackfortress-linux-amd64/
FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates git passwd \
 && rm -rf /var/lib/apt/lists/*

COPY . /tmp/blackfortress-release
RUN /tmp/blackfortress-release/install.sh && rm -rf /tmp/blackfortress-release

# Data lives here; mount a volume to keep it across containers.
VOLUME /var/lib/blackfortress

CMD ["bfd", "run"]
