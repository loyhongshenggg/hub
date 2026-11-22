# ADR-0011: Windows releases as a WSL distribution file

- Status: Accepted (2026-11-21)

## Context

Other people should be able to download a release from GitHub and run it on their own Windows laptop without a Linux setup, Docker Desktop or a build toolchain. The first release, named **finance**, ships only the Finance app. Releases are Windows-only for now.

Constraints:

- Unsigned programs can be blocked on Windows (ADR-0009). Code signing for individuals isn't available: Microsoft's Artifact Signing ($9.99/month) has paused individual onboarding ([update](https://techcommunity.microsoft.com/blog/microsoft-security-blog/trusted-signing-public-preview-update/4399713), [Q&A](https://learn.microsoft.com/answers/a/2028104)).
- Docker Desktop is free only for personal use and companies under 250 employees and $10M revenue ([Docker FAQ](https://www.docker.com/pricing/faq)).
- WSL installs a distribution from a `.wsl` file (a tar of the root filesystem) on double-click or with `wsl --install --from-file`. `/etc/wsl-distribution.conf` sets the name and a first-run command (WSL 2.4.8+; [Microsoft](https://learn.microsoft.com/windows/wsl/build-custom-distro), [Ubuntu](https://ubuntu.com/blog/ubuntu-wsl-new-format-available)).

## Decision

- **One file to install.** A release is `<name>.wsl`, which contains:
  - Debian, systemd and Docker Engine;
  - the hub core in `/opt/hub/bin`;
  - the listed apps (manifest, icon, a compose file that runs a prebuilt image, and the image itself as a tarball);
  - the launcher (ADR-0009).
- **First run.** `first-run.sh` (the OOBE command) writes the launcher config and puts a Desktop icon named after the release. It runs only signed Windows tools: conhost, wsl.exe, and Chrome or Edge. On first start, `launch.sh` loads the app images into Docker. Shipped apps are set to auto-start.
- **What ships.** `releases/<name>.env` lists the apps, the name, the title and the port (finance: 7100, so it can run next to a dev hub on 7000). Apps not listed never ship.
- **Build and publish.** `tools/release/build.sh <name> <version>` builds everything. A tag `<name>-v<version>` runs `.github/workflows/release.yml`:
  1. checks;
  2. build;
  3. [build provenance attestations](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations);
  4. a GitHub release with `<name>.wsl`, `<name>-update.tar.gz` and `SHA256SUMS`.
- **Every download is a clean copy.**
  - Releases are built only by GitHub Actions, from a fresh checkout of the tag. The workflow refuses a modified tree.
  - The rootfs drops apt lists, logs, temp files, SSH keys and `/root/.ssh`.
  - `/etc/machine-id` is left empty, so each install generates its own identity on first boot. The first build baked one in, which every install would have shared.
  - `tools/release/audit.sh` fails the build on:
    - secrets or personal files anywhere: `.env`, keys, `desktop.env`, databases, PDFs;
    - any file in home folders, `/root`, Docker's data, `/tmp`, or a non-empty log;
    - a set `/etc/machine-id`;
    - files in `/opt/hub` beyond the expected set;
    - data, home or temp files inside each app image.
  - Downloads carry `SHA256SUMS` and a GitHub build attestation. `update.sh` checks the SHA-256 before installing.
- **Updates keep data.** `/opt/hub/update.sh` finds the newest `<name>-v*` release, checks its SHA-256, stops the app and swaps in the new files. Reinstalling the `.wsl` would erase the distribution's data.
- **Docker that coexists with other Dockers.** All WSL distributions share one network namespace, so this Docker:
  - **uses its own bridge (`hubbr0`).** `dockerd` deletes the shared `docker0` when it runs with `--bridge=none`, or with `default-address-pools` and no `--bridge` ([moby `initNetworkController`](https://github.com/moby/moby/blob/master/daemon/daemon_unix.go)). Found the hard way: the first test build used `bridge: none` and removed the dev Docker's `docker0`.
  - **runs with iptables off**, so it can't rewrite another Docker's chains.
  - **gets outbound NAT from `net.sh`,** scoped to `10.231.0.0/16` and never flushing anything.

  Tested next to the dev Docker: the app's containers resolved DNS and reached HTTPS, and the dev hub's Radar fetched all 21 sources without errors.

### Not chosen

- **Images on GHCR.** A registry needs package visibility settings and network at install time. Bundling images makes the install self-contained.
- **A native Windows core with Docker Desktop.** That means porting the core and taking on Docker Desktop's licence, and it still needs signing.
- **macOS and Linux.** Out of scope for now.

## Consequences

- Install is: turn on WSL (once), double-click the file, click the icon. No admin rights are needed after WSL is on.
- A release is about 230 MB (Debian, Docker and the app image) and uses about 1–1.5 GB once installed.
- No code signing is needed. The tar is installed by `wsl.exe`.
- The release repository has to be public for downloads and `update.sh` to work without credentials.
