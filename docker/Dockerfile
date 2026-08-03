FROM ubuntu:24.04 AS strongswan-builder

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y build-essential autoconf automake libtool pkg-config \
    libssl-dev gettext bison flex gperf curl patch && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN curl -L --fail https://github.com/strongswan/strongswan/archive/refs/tags/5.9.13.tar.gz \
    | tar -xz --strip-components=1
COPY xauth-generic-otp.patch /tmp/xauth-generic-otp.patch
RUN patch -p1 < /tmp/xauth-generic-otp.patch && ./autogen.sh && \
    ./configure --disable-defaults --enable-xauth-generic && \
    make -C src/libstrongswan -j2 && make -C src/libcharon -j2 && \
    make -C src/libcharon/plugins/xauth_generic -j2

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    openvpn \
    openconnect \
    openfortivpn \
    strongswan-charon \
    strongswan-swanctl \
    libcharon-extra-plugins \
    libstrongswan-extra-plugins \
    dante-server \
    dnsmasq \
    dnsutils \
    iproute2 \
    iptables \
    curl \
    gettext-base \
    && rm -rf /var/lib/apt/lists/*

COPY --from=strongswan-builder /src/src/libcharon/plugins/xauth_generic/.libs/libstrongswan-xauth-generic.so /usr/lib/ipsec/plugins/libstrongswan-xauth-generic.so

COPY entrypoint.sh /entrypoint.sh
COPY danted.conf.template /etc/danted.conf.template
RUN chmod +x /entrypoint.sh

EXPOSE 1080

HEALTHCHECK --interval=2s --timeout=1s --start-period=5s --retries=3 \
    CMD bash -c '</dev/tcp/127.0.0.1/1080' || exit 1

ENTRYPOINT ["/entrypoint.sh"]
