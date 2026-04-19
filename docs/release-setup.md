# Release-time distribution setup

Goreleaser publishes to several channels on every `v*` tag push. Most require a one-time external account + a GitHub Actions secret. This page is the checklist.

All secrets live under **Settings → Secrets and variables → Actions** on the repo; the `release` workflow also pins `environment: production` so you can scope these to the production environment if you prefer.

## GitHub Container Registry (ghcr.io)

Works out of the box — uses the built-in `GITHUB_TOKEN`. No extra setup needed.

Package will appear at <https://github.com/Loupeznik/kubeconfig-manager/pkgs/container/kubeconfig-manager> after the first tagged release.

Images are signed keyless via cosign + GitHub OIDC. Verify:

```sh
cosign verify ghcr.io/loupeznik/kubeconfig-manager:<tag> \
  --certificate-identity-regexp='https://github.com/Loupeznik/kubeconfig-manager/.github/workflows/release.yml@.*' \
  --certificate-oidc-issuer='https://token.actions.githubusercontent.com'
```

## Homebrew tap

The short answer: an **empty public repo + a PAT** is the whole setup. Homebrew doesn't require any central registration for third-party taps — any repo named `homebrew-<suffix>` works.

1. **Repo name must be `Loupeznik/homebrew-tap`** (the `homebrew-` prefix is mandatory — `brew tap loupeznik/tap` drops the prefix and looks for `homebrew-tap` under the `loupeznik` org). Make it **public**. Leave it empty; goreleaser creates the first commit.
2. **Personal access token**. Create a **classic** PAT (fine-grained tokens don't always cover the push-to-another-repo case goreleaser needs). Scope: `repo`. Expiry ≥ 1 year. Add it as the `HOMEBREW_TAP_TOKEN` Actions secret on the kubeconfig-manager repo.
3. (Optional but recommended) Add a short `README.md` in the tap repo so visitors understand what it is. goreleaser doesn't touch the README after creating it, so a one-paragraph description is a one-time effort.

On release, goreleaser:

- Generates `Casks/kcm.rb` under the `Casks/` directory of the tap repo.
- Commits + pushes as `goreleaserbot`.
- Attaches the archive URL + SHA-256 of the tarball goreleaser just built for each target arch/OS.
- Includes a `post_install` hook that strips the macOS Gatekeeper quarantine xattr so the unsigned binary runs without a Gatekeeper prompt on first launch.

Users install with:

```sh
brew install --cask loupeznik/tap/kcm          # explicit cask form
# or
brew install loupeznik/tap/kcm                  # auto-detects the cask
```

### Formula → Cask migration (v0.10.1 → v0.10.2)

v0.10.1 shipped as a brew **formula** (`Formula/kcm.rb`). Starting with v0.10.2 we ship as a **cask** (`Casks/kcm.rb`) because goreleaser deprecated the formula generator in favour of the cask one (see [goreleaser.com/deprecations#brews](https://goreleaser.com/deprecations#brews)).

One-time tap cleanup to keep existing users on a smooth upgrade path:

1. Delete `Formula/kcm.rb` from the `Loupeznik/homebrew-tap` repo. (goreleaser writes the new cask to `Casks/`; leaving the old formula around causes brew to prefer it.)
2. Add a `tap_migrations.json` at the tap repo root so `brew update` auto-redirects users from the removed formula to the new cask:

    ```json
    {
      "kcm": "loupeznik/tap"
    }
    ```

    Commit both changes to `homebrew-tap/master` before tagging v0.10.2.
3. Existing users run `brew update && brew upgrade` and brew re-installs as a cask under the hood.

Gotchas to know about:

- **`HOMEBREW_TAP_TOKEN` is required.** If unset when you tag, the `homebrew_casks:` stage errors at the push step. The rest of the release (GitHub release, ghcr, AUR, snap) still completes because goreleaser runs them in parallel.
- **`brew audit` is not enforced.** That's only for homebrew-core submissions. Third-party taps can ship anything that parses as a Ruby cask/formula.
- **The tap repo doesn't need to exist before the first release** — goreleaser will push the first commit regardless. But creating it yourself means you can set up branch protection, a README, and a topic (`homebrew-tap`) upfront.

## AUR (Arch User Repository)

One-time:

1. Create an AUR account at <https://aur.archlinux.org/register>.
2. Register the package name by pushing an initial empty commit with a `.SRCINFO` — or let goreleaser create it on the first release (the AUR creates the repo on first SSH push).
3. Generate a dedicated SSH keypair (do **not** reuse your regular SSH key):

    ```sh
    ssh-keygen -t ed25519 -N '' -f aur_key -C 'goreleaser-aur'
    ```

4. Add the **public** key (`aur_key.pub`) under <https://aur.archlinux.org/account/> → *My Account* → *SSH Public Key*.
5. Add the **private** key (`aur_key` contents) as the `AUR_KEY` Actions secret on this repo.

On release:

- Goreleaser pushes a `PKGBUILD` + `.SRCINFO` to `ssh://aur@aur.archlinux.org/kubeconfig-manager-bin.git`.
- Users install via `yay -S kubeconfig-manager-bin` (or any other AUR helper).
- Package installs the binary, man pages, and bash/zsh/fish completions.

## Snap Store

One-time:

1. Create a Snapcraft account at <https://snapcraft.io/account>.
2. Register the snap name:

    ```sh
    sudo snap install snapcraft --classic
    snapcraft login
    snapcraft register kubeconfig-manager
    ```

3. Export a store credential for CI. Run this on your **host** machine (not inside a container) — snapcraft uses a browser-based SSO flow for 2FA, which the old email+password path in `snapcore/snapcraft:*` docker images can't complete.

    Note the scoping trade-off with `--snaps=`:

    - A **just-registered** snap name (present in your dashboard but never uploaded) can't be used with `--snaps=<name>` — snapcraft errors with `Snap not found for the given snap name`. The scoping requires the snap to have at least one published revision.
    - Omitting `--snaps` gives you an **account-scoped** credential instead: it can push/release any snap the account owns. For a single-snap account this is fine, and the first CI release will itself be the "publish" step that activates the name.

    Recommended for a fresh account:

    ```sh
    snapcraft export-login \
      --acls=package_access,package_push,package_update,package_release \
      --expires=2030-01-01 snap-credentials.txt
    ```

    If you later want to tighten the credential to a specific snap, first publish one revision (any channel), then re-export with `--snaps=kubeconfig-manager` and swap the secret.

    Omit `--channels` too — we currently publish `devel` revisions to `edge`/`beta` and will flip to `stable` at v1.0.0 without re-issuing the secret. If you prefer an explicit channel allowlist, pass `--channels=stable,candidate,beta,edge`; the narrow `--channels=stable` form rejects uploads while grade is `devel`.

    If you must export from a container, run `snapcraft login` interactively first (it prompts for the OTP separately), then run `snapcraft export-login` in the same session.

4. Add the contents of `snap-credentials.txt` as the `SNAPCRAFT_STORE_CREDENTIALS` Actions secret on this repo.

On release, goreleaser:

- Builds the `.snap` under `confinement: classic` for linux/amd64 + arm64.
- Sets `grade: devel` while we're on the v0.x line, and `grade: stable` from v1.0.0 onward — the template `{{ if eq .Major 0 }}devel{{ else }}stable{{ end }}` flips automatically on the v1 tag. Snap store restricts `devel` revisions to the `edge` and `beta` channels; `candidate` and `stable` are blocked until grade flips at v1.0.0. Grade is per-revision (not per-snap-name), so the v1.0.0 revision can publish straight to `stable` regardless of what v0.x shipped.

Users install with:

```sh
sudo snap install kubeconfig-manager --classic
```

**Why classic confinement.** kcm reads `~/.kube`, exec's `kubectl` / `helm` from the user's PATH, and writes to the user's shell rc during `install-shell-hook`. None of those work under strict confinement without hand-wiring interfaces — classic is simpler and what users of this class of tool expect. Classic confinement requires a one-time review from the snap store team (request via <https://forum.snapcraft.io/c/store-requests/19>) before the first publish; the upload itself fails with an approval message until that's granted.

## Secrets quick-reference

| Secret | Used by | Created via |
| --- | --- | --- |
| `HOMEBREW_TAP_TOKEN` | `homebrew_casks:` | Classic PAT with `repo` scope |
| `AUR_KEY` | `aurs:` | `ssh-keygen`, public key on AUR |
| `SNAPCRAFT_STORE_CREDENTIALS` | `snapcrafts:` | `snapcraft export-login` |
| `GITHUB_TOKEN` | ghcr, release notes | Built-in (no setup) |

## Before the next tag

- Confirm every secret above is present and not expired.
- `goreleaser check` locally (or rely on the `goreleaser config check` CI job) — it validates all four package manager blocks.
- Dry-run end-to-end: `goreleaser release --snapshot --clean --skip=publish`.
