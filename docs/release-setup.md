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

- **`HOMEBREW_TAP_TOKEN` is required.** If unset when you tag, the `homebrew_casks:` stage errors at the push step. The rest of the release (GitHub release, ghcr, AUR) still completes because goreleaser runs them in parallel.
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

## Snap Store — postponed

Snap publishing is deliberately **not** wired in the current `.goreleaser.yaml`. The `kubeconfig-manager` name was blocked during store registration and needs a dispute / clarification round with the snap store team before we can proceed. Until then, we don't generate `.snap` artifacts on release.

When we come back to it, the work is:

1. Resolve the name collision (either clear `kubeconfig-manager` via <https://forum.snapcraft.io/c/store-requests/19> or fall back to `kcm` — which may also be taken; check with `snapcraft register --dry-run`).
2. Re-add the `snapcrafts:` block to `.goreleaser.yaml` — `id: kcm`, `ids: [kcm]`, `confinement: classic`, `grade: stable`, `license: Apache-2.0`, with a single `apps.kcm` entry. Reference: <https://goreleaser.com/customization/package/snapcraft/>.
3. Re-add the `snapcraft` install step + `SNAPCRAFT_STORE_CREDENTIALS` env to `release.yml`.
4. Request classic-confinement approval via the forum link above before the first published release — classic confinement is needed because kcm reads `~/.kube`, exec's `kubectl` / `helm`, and writes to the user's shell rc.

## Secrets quick-reference

| Secret | Used by | Created via |
| --- | --- | --- |
| `HOMEBREW_TAP_TOKEN` | `brews:` | Classic PAT with `repo` scope |
| `AUR_KEY` | `aurs:` | `ssh-keygen`, public key on AUR |
| `GITHUB_TOKEN` | ghcr, release notes | Built-in (no setup) |

## Before the next tag

- Confirm every secret above is present and not expired.
- `goreleaser check` locally (or rely on the `goreleaser config check` CI job) — it validates all four package manager blocks.
- Dry-run end-to-end: `goreleaser release --snapshot --clean --skip=publish`.
