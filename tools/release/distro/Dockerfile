# Root filesystem of a hub release distribution (exported into <NAME>.wsl).
# Debian + systemd + Docker Engine + the hub in /opt/hub, run as user "hub".
FROM debian:trixie-slim
ARG DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl gnupg systemd systemd-sysv dbus dbus-user-session libpam-systemd \
      iproute2 iptables procps less \
 && install -m 0755 -d /etc/apt/keyrings \
 && curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc \
 && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/debian trixie stable" \
      >/etc/apt/sources.list.d/docker.list \
 && apt-get update \
 && apt-get install -y --no-install-recommends docker-ce docker-ce-cli containerd.io docker-compose-plugin \
 && rm -rf /var/lib/apt/lists/* \
 && systemctl enable docker.service containerd.service \
 && useradd --create-home --uid 1000 --groups docker --shell /bin/bash hub
COPY distro/wsl.conf /etc/wsl.conf
COPY distro/wsl-distribution.conf /etc/wsl-distribution.conf
COPY distro/daemon.json /etc/docker/daemon.json
COPY --chmod=0755 distro/net.sh /usr/local/lib/hub/net.sh
COPY distro/docker-net.conf /etc/systemd/system/docker.service.d/hub-net.conf
COPY --chown=1000:1000 opt/hub /opt/hub
# A clean copy: nothing from the build host or from a previous run. Every
# install generates its own machine-id on first boot (empty file = first boot).
RUN rm -rf /var/lib/apt/lists/* /var/cache/apt/* /var/cache/debconf/*-old /var/log/apt/* /tmp/* /var/tmp/* \
 && find /var/log -type f -exec truncate -s 0 {} + \
 && rm -rf /var/lib/dbus/machine-id /etc/ssh/ssh_host_* /root/.ssh \
 && : >/etc/machine-id
