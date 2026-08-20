<p align="center">
  <img src="assets/icon.png" alt="Silo logo" width="112" height="112">
</p>

<h1 align="center">Silo</h1>

<p align="center">
  <strong>Your media. Your server. Your way.</strong>
</p>

<p align="center">
  A modern, self-hosted media server for films, series, audiobooks, ebooks, podcasts, and manga.
</p>

<p align="center">
  <a href="https://github.com/Silo-Server/silo-server/releases"><img alt="Latest GitHub release" src="https://img.shields.io/github/v/release/Silo-Server/silo-server?include_prereleases&amp;sort=semver&amp;display_name=tag&amp;style=flat-square&amp;label=release"></a>
  <a href="https://github.com/orgs/Silo-Server/packages/container/package/silo-server"><img alt="Container image on GHCR" src="https://img.shields.io/badge/container-GHCR-2496ED?style=flat-square&amp;logo=docker&amp;logoColor=white"></a>
  <a href="https://github.com/Silo-Server/silo-server/actions/workflows/ci.yml"><img alt="Continuous integration" src="https://img.shields.io/github/actions/workflow/status/Silo-Server/silo-server/ci.yml?branch=main&amp;style=flat-square&amp;label=CI"></a>
  <img alt="Go 1.26" src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&amp;logo=go&amp;logoColor=white">
  <img alt="React 19" src="https://img.shields.io/badge/React-19-149ECA?style=flat-square&amp;logo=react&amp;logoColor=white">
  <a href="LICENSE"><img alt="AGPL-3.0-or-later license" src="https://img.shields.io/badge/license-AGPL--3.0--or--later-555555?style=flat-square"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick start</a>
  · <a href="docs/wiki/index.md">Documentation</a>
  · <a href="docs/release-versioning.md">Builds &amp; releases</a>
  · <a href="https://discord.com/invite/4RxuUQAEnW">Discord</a>
  · <a href="#supporting-silo">Support Silo</a>
  · <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

> [!WARNING]
> Silo is in active pre-release development. APIs, configuration, and database
> migrations may change before the first stable release. Review the build history
> and back up your deployment before updating.

## A media server built around ownership

Silo keeps your media, metadata, and household experience under your control.
Run it on one host, reach it at home or away, and choose how each device receives
the best version of your media.

<table>
  <tr>
    <td width="33%" valign="top">
      <strong>Play</strong><br><br>
      Direct play when possible, remux when needed, or transcode automatically,
      with hardware acceleration including VA-API, Quick Sync, and NVENC.
    </td>
    <td width="33%" valign="top">
      <strong>Organize</strong><br><br>
      Bring films, series, audiobooks, ebooks, podcasts, and manga into one
      catalog, with plugin-driven matching and providers such as TMDB and TVDB
      where supported.
    </td>
    <td width="33%" valign="top">
      <strong>Connect</strong><br><br>
      Use the included web app or Silo's Jellyfin/Emby compatibility surface
      with clients such as <a href="https://vidhub.okaapps.com/what-does-vidhub-do/">VidHub</a>,
      <a href="https://github.com/jarnedemeulemeester/findroid">Findroid</a>, and
      <a href="https://support.firecore.com/hc/en-us/articles/360006462093-Streaming-from-Plex-Emby-and-Jellyfin">Infuse</a>.
      Client coverage varies.
    </td>
  </tr>
  <tr>
    <td width="33%" valign="top">
      <strong>Share</strong><br><br>
      Give household members their own profiles, watch state, library access,
      and parental controls.
    </td>
    <td width="33%" valign="top">
      <strong>Manage</strong><br><br>
      Configure libraries, users, providers, storage, search, and playback from
      a dedicated administration interface.
    </td>
    <td width="33%" valign="top">
      <strong>Scale</strong><br><br>
      Start with one integrated server, then separate proxy and transcode roles
      across shared PostgreSQL and Redis infrastructure when needed.
    </td>
  </tr>
</table>

## Quick start

The recommended installation uses Docker Compose 2.24 or newer. The default
stack includes Silo, PostgreSQL with pgvector, Redis, and FFmpeg.

1. **Clone the repository and create your configuration.**

   ```sh
   git clone https://github.com/Silo-Server/silo-server.git
   cd silo-server
   cp .env.example .env
   chmod 600 .env
   printf '\nPOSTGRES_PASSWORD=%s\nSECRET_KEY=%s\n' \
     "$(openssl rand -hex 24)" "$(openssl rand -base64 48)" >> .env
   ```

2. **Set the host path to your media.**

   Edit `.env` and replace `MEDIA_ROOT` with an absolute path:

   ```dotenv
   MEDIA_ROOT=/path/to/your/media
   ```

3. **Start Silo.**

   ```sh
   docker compose up -d
   ```

4. **Open the web app.**

   Visit <http://localhost:8090>, complete onboarding, then add libraries,
   users, metadata providers, and playback settings from the admin interface.

> [!CAUTION]
> Keep `SECRET_KEY` secret and back it up separately from PostgreSQL. Silo uses
> it to encrypt stored credentials; losing it makes those credentials
> unrecoverable.

The default deployment is CPU-only and stores application data under
`/opt/silo`. The [Docker deployment guide](docs/wiki/deployment/docker.md)
covers custom storage paths, VA-API/Quick Sync, NVIDIA NVENC, Meilisearch,
external PostgreSQL and Redis, distributed roles, backups, and PostgreSQL
auto-tuning.

Migrating an existing Continuum installation? Follow the
[Continuum-to-Silo cutover guide](docs/continuum-to-silo-docker-migration.md).

## Builds and releases

> [!IMPORTANT]
> Until Silo's first release is selected and published, default-branch
> containers are identified by an ordered `build-N` and their commit SHA.
> Build numbers make published images comparable; they are not release versions.

Silo's release contract follows [Semantic Versioning](https://semver.org/) with
prerelease and build metadata support. The
[release versioning guide](docs/release-versioning.md) explains the source of
truth and the meaning of each container tag:

| Image reference | Use |
| --- | --- |
| `build-N` | Select an ordered published build. |
| Short commit SHA | Select the image built from an exact source revision. |
| Image digest | Pin an immutable deployment or rollback target. |
| `latest` | Follow the newest successful default-branch publication. |

Review configuration, compatibility, and migration impact before every update.

## Documentation

| Start here | What it covers |
| --- | --- |
| [Documentation index](docs/wiki/index.md) | User and operator guides currently available in the repository. |
| [Docker deployment](docs/wiki/deployment/docker.md) | Storage, acceleration, search, topology, external services, tuning, and updates. |
| [Media naming](docs/wiki/admin/media-folder-and-naming.md) | Supported library folder structures and filenames. |
| [Development guide](DEVELOPMENT.md) | Source setup, builds, tests, migrations, and repository structure. |
| [Settings API](docs/settings-api.md) | Client settings contracts, contextual scopes, and effective reads. |
| [Downloads API](docs/downloads-api.md) | Offline sync, download delivery, and distributed client behavior. |
| [Release versioning](docs/release-versioning.md) | SemVer, container identifiers, release notes, and publishing. |

## Community and contributions

Questions and project discussion are welcome in the
[Silo Discord community](https://discord.com/invite/4RxuUQAEnW).

For a bug, installation problem, or performance issue, use the
[GitHub issue forms](https://github.com/Silo-Server/silo-server/issues/new/choose)
and include the workflow you followed, exact reproduction steps, expected and
actual behavior, the affected version/build, deployment details, and raw logs
where relevant.

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before
starting, and use [DEVELOPMENT.md](DEVELOPMENT.md) for the local workflow.
Non-trivial features, API changes, migrations, behavior changes, and refactors
should be coordinated in an issue before implementation.

## Supporting Silo

Silo is a free and open-source project developed in spare time and funded out of
pocket. It will remain free and open source.
[GitHub Sponsors](https://github.com/sponsors/quick104) helps cover AI-assisted
development tooling, including Claude and Codex, push-notification relay
infrastructure, and future project costs.

Sponsoring is optional. Bug reports, contributions, documentation, and feedback
are equally valuable ways to support the project.

<p align="center">
  <a href="https://github.com/sponsors/quick104"><strong>Sponsor Silo</strong></a>
  · <a href="https://discord.com/invite/4RxuUQAEnW"><strong>Join the community</strong></a>
</p>

## License and trademarks

Silo's source code is licensed under the
**GNU Affero General Public License v3.0 or later** (`AGPL-3.0-or-later`). See
[LICENSE](LICENSE).

The **Silo name, logo, and wordmark are trademarks of Silo Media L.L.C.** and
are not covered by the AGPL. Forks and redistributions may use the code but must
not use the Silo brand as their identity and must remove or replace the brand
assets. Publishing a Silo-branded app to an app store requires written
permission. See [TRADEMARK.md](TRADEMARK.md) for permitted referential use,
including phrases such as "compatible with Silo."
