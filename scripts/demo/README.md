# README demo recording

Scripts and notes for recording the asciinema showcase that fronts the project README.

## Prerequisites

```sh
brew install asciinema starship    # recording + prompt
brew install agg                   # asciinema → GIF (recommended, actively maintained)
```

`zsh` is already on macOS by default. `starship` renders the prompt the demo uses, including the `kcm` custom module.

SVG conversion is deliberately not listed: the two options in circulation (`svg-term-cli`, `termtosvg`) are both poorly maintained, and GitHub renders GIFs inline just fine. Stick with `agg`.

## Record

```sh
# 1. Seed the sandbox.
./scripts/demo/setup.sh

# 2. Source the env. This points HOME at .temp/demo/home/ so every kcm
#    command uses the sandbox kubeconfigs by default — no --dir flags needed.
#    Your real ~/.zshrc and ~/.config/starship.toml are backed up to
#    .temp/backups/ by setup.sh, just in case.
. .temp/demo/env.sh

# 3. Start recording into a clean zsh. The sandbox .zshrc loads starship
#    with the kcm custom module and installs the kcm shell hook so plain
#    `kubectl`/`helm` are aliased through the guard for the demo.
asciinema rec docs/images/demo.cast \
  --title 'kubeconfig-manager — prod/test guard in action' \
  --command zsh \
  --idle-time-limit 2 \
  --overwrite

# 4. Follow the beats in scripts/demo/walkthrough.md.
# 5. Exit the recording zsh (ctrl+d) when done.
```

`--overwrite` is important — re-doing a take otherwise fails with "file exists". `--idle-time-limit 2` collapses long pauses in the recorded cast so viewers don't wait on your typing.

## Convert

```sh
agg --theme monokai --font-size 18 docs/images/demo.cast docs/images/demo.gif
```

Tune `--font-size` and `--theme` to taste. `agg --help` lists the bundled themes. Typical README demos run 200–800 KB at font size 18; bump lower for smaller files.

## Embed

Add near the top of `README.md`, above the feature list:

```markdown
![kubeconfig-manager demo](docs/images/demo.gif)
```

Optionally upload the raw cast so readers can copy-paste commands from it:

```sh
asciinema upload docs/images/demo.cast
```

And link to the resulting URL:

```markdown
> [View the interactive cast ↗](https://asciinema.org/a/<id>)
```

## Re-recording

Re-run `./scripts/demo/setup.sh` for a fresh sandbox whenever the flow changes (new feature, default-value shift, etc.) and update `walkthrough.md` to match the state the setup script now seeds. The setup script wipes `.temp/demo/` but preserves `.temp/backups/` across runs.
